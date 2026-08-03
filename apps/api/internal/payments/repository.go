package payments

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

const stripeConfigurationLifecycleLockID int64 = 839036782198407216
const userCheckoutLifecycleLockSeed int64 = 412456873631731953

type Repository struct {
	db *sql.DB
}

type stripeLifecycleConnectionContextKey struct{}

type sqlTransactionStarter interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

type paymentContextDB struct {
	ctx                context.Context
	executor           dbsqlc.DBTX
	transactionStarter sqlTransactionStarter
}

type StoredStripeConfiguration struct {
	AccountID               string
	Mode                    StripeMode
	ChargesEnabled          bool
	ProductTaxCode          string
	PurchasesEnabled        bool
	StripeProductID         string
	StripeWebhookEndpointID string
	APIKeyCiphertext        string
	WebhookSecretCiphertext string
	SecretMaterialKind      string
	CreatedBy               int64
	UpdatedBy               int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type ConfigurationAudit struct {
	ActorID      int64
	Outcome      string
	ErrorCode    string
	ErrorSummary string
	Metadata     json.RawMessage
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) dbWithContext(ctx context.Context) paymentContextDB {
	if connection, ok := ctx.Value(stripeLifecycleConnectionContextKey{}).(*sql.Conn); ok {
		return paymentContextDB{ctx: ctx, executor: connection, transactionStarter: connection}
	}
	return paymentContextDB{ctx: ctx, executor: r.db, transactionStarter: r.db}
}

func (r *Repository) WithStripeConfigurationSharedLock(ctx context.Context, run func(context.Context) error) error {
	return r.withStripeConfigurationLifecycleLock(ctx, false, run)
}

func (r *Repository) WithStripeConfigurationExclusiveLock(ctx context.Context, run func(context.Context) error) error {
	return r.withStripeConfigurationLifecycleLock(ctx, true, run)
}

func (r *Repository) withStripeConfigurationLifecycleLock(ctx context.Context, exclusive bool, run func(context.Context) error) (resultErr error) {
	if r == nil || r.db == nil || run == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	connection, err := r.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open Stripe configuration lifecycle lock connection: %w", err)
	}
	lockFunction, unlockFunction := "pg_advisory_lock_shared", "pg_advisory_unlock_shared"
	if exclusive {
		lockFunction, unlockFunction = "pg_advisory_lock", "pg_advisory_unlock"
	}
	if _, err := connection.ExecContext(ctx, "SELECT "+lockFunction+"($1)", stripeConfigurationLifecycleLockID); err != nil {
		_ = connection.Close()
		return fmt.Errorf("acquire Stripe configuration lifecycle lock: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		unlockErr := connection.QueryRowContext(unlockContext, "SELECT "+unlockFunction+"($1)", stripeConfigurationLifecycleLockID).Scan(&unlocked)
		if unlockErr != nil || !unlocked {
			_ = connection.Raw(func(any) error { return driver.ErrBadConn })
			if unlockErr == nil {
				unlockErr = fmt.Errorf("Stripe configuration lifecycle lock was not held")
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("release Stripe configuration lifecycle lock: %w", unlockErr))
		}
		if closeErr := connection.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close Stripe configuration lifecycle lock connection: %w", closeErr))
		}
	}()
	lockedContext := context.WithValue(ctx, stripeLifecycleConnectionContextKey{}, connection)
	return run(lockedContext)
}

func (r *Repository) WithUserCheckoutLock(ctx context.Context, userID int64, run func(context.Context) error) (resultErr error) {
	if r == nil || r.db == nil || userID <= 0 || run == nil {
		return fmt.Errorf("payments repository: invalid user Checkout lock")
	}
	connection, ok := ctx.Value(stripeLifecycleConnectionContextKey{}).(*sql.Conn)
	if !ok {
		return fmt.Errorf("payments repository: user Checkout lock requires Stripe lifecycle session")
	}
	lockKey := fmt.Sprintf("payment-user-checkout:%d", userID)
	if _, err := connection.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, $2))`, lockKey, userCheckoutLifecycleLockSeed); err != nil {
		return fmt.Errorf("acquire user Checkout lifecycle lock: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		unlockErr := connection.QueryRowContext(unlockContext, `SELECT pg_advisory_unlock(hashtextextended($1, $2))`, lockKey, userCheckoutLifecycleLockSeed).Scan(&unlocked)
		if unlockErr != nil || !unlocked {
			_ = connection.Raw(func(any) error { return driver.ErrBadConn })
			if unlockErr == nil {
				unlockErr = fmt.Errorf("user Checkout lifecycle lock was not held")
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("release user Checkout lifecycle lock: %w", unlockErr))
		}
	}()
	return run(ctx)
}

func (db paymentContextDB) queries() *dbsqlc.Queries {
	return dbsqlc.New(db.executor)
}

func (db paymentContextDB) withTx(run func(*dbsqlc.Queries, *sql.Tx) error) (resultErr error) {
	if db.transactionStarter == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	tx, err := db.transactionStarter.BeginTx(db.ctx, nil)
	if err != nil {
		return fmt.Errorf("begin payments transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && resultErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("rollback payments transaction: %w", rollbackErr))
		}
	}()
	if err := run(dbsqlc.New(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit payments transaction: %w", err)
	}
	return nil
}

func (r *Repository) GetStripeConfiguration(ctx context.Context) (StoredStripeConfiguration, error) {
	if r == nil || r.db == nil {
		return StoredStripeConfiguration{}, fmt.Errorf("payments repository: database unavailable")
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentStripeConfiguration(ctx)
	if err != nil {
		return StoredStripeConfiguration{}, err
	}
	return storedStripeConfiguration(row), nil
}

func (r *Repository) ReplaceStripeConfiguration(ctx context.Context, value StoredStripeConfiguration, audit ConfigurationAudit) (StoredStripeConfiguration, error) {
	if r == nil || r.db == nil {
		return StoredStripeConfiguration{}, fmt.Errorf("payments repository: database unavailable")
	}
	var stored StoredStripeConfiguration
	err := r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ *sql.Tx) error {
		row, err := q.ReplacePaymentStripeConfiguration(ctx, dbsqlc.ReplacePaymentStripeConfigurationParams{
			AccountID: value.AccountID, Mode: string(value.Mode), ChargesEnabled: value.ChargesEnabled,
			ProductTaxCode: value.ProductTaxCode, PurchasesEnabled: value.PurchasesEnabled,
			StripeProductID: value.StripeProductID, StripeWebhookEndpointID: value.StripeWebhookEndpointID,
			ApiKeyCiphertext: value.APIKeyCiphertext, WebhookSecretCiphertext: value.WebhookSecretCiphertext,
			SecretMaterialKind: value.SecretMaterialKind, CreatedBy: value.UpdatedBy,
		})
		if err != nil {
			return fmt.Errorf("replace Stripe configuration: %w", err)
		}
		if err := insertConfigurationAudit(ctx, q, audit); err != nil {
			return err
		}
		stored = storedStripeConfiguration(row)
		return nil
	})
	return stored, err
}

func (r *Repository) RecordConfigurationAudit(ctx context.Context, audit ConfigurationAudit) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	return insertConfigurationAudit(ctx, r.dbWithContext(ctx).queries(), audit)
}

func (r *Repository) InsertStripeWebhookEvent(ctx context.Context, event StripeWebhookEvent, result string, processedAt *time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().InsertPaymentWebhookEvent(ctx, dbsqlc.InsertPaymentWebhookEventParams{
		StripeEventID: event.ID, EventType: event.Type, StripeObjectID: event.ObjectID,
		Livemode: event.Livemode, StripeCreatedAt: event.CreatedAt, ProcessingResult: result,
		ProcessedAt: dbutil.NullTime(processedAt),
	})
	if err != nil {
		return false, fmt.Errorf("insert Stripe webhook event: %w", err)
	}
	return rows == 0, nil
}

func (r *Repository) MarkStripeWebhookEventProcessed(ctx context.Context, eventID, result, errorSummary string, processedAt time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().MarkPaymentWebhookEventProcessed(ctx, dbsqlc.MarkPaymentWebhookEventProcessedParams{
		StripeEventID: eventID, ProcessingResult: result, ErrorSummary: boundedText(errorSummary, 500),
		ProcessedAt: sql.NullTime{Time: processedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("mark Stripe webhook event processed: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("mark Stripe webhook event processed: event not found")
	}
	return nil
}

func (r *Repository) CreatePaymentOrderIntent(ctx context.Context, input CreatePaymentOrderIntentInput) (PaymentOrder, bool, error) {
	if r == nil || r.db == nil {
		return PaymentOrder{}, false, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := parseOrderUUID(input.ID)
	if err != nil {
		return PaymentOrder{}, false, fmt.Errorf("parse payment order ID: %w", err)
	}
	publicToken, err := parseOrderUUID(input.PublicToken)
	if err != nil {
		return PaymentOrder{}, false, fmt.Errorf("parse payment order public token: %w", err)
	}
	var order PaymentOrder
	var reused bool
	err = r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx *sql.Tx) error {
		lockKey := fmt.Sprintf("%d:%s", input.UserID, input.ClientIdempotencyKey)
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 8188403870220423781))`, lockKey); err != nil {
			return fmt.Errorf("lock payment order idempotency: %w", err)
		}
		existing, err := q.GetPaymentOrderByIdempotency(ctx, dbsqlc.GetPaymentOrderByIdempotencyParams{
			UserID: input.UserID, ClientIdempotencyKey: input.ClientIdempotencyKey,
		})
		if err == nil {
			if existing.RequestHash != input.RequestHash {
				return ErrCheckoutIdempotencyConflict
			}
			order, reused = paymentOrder(existing), true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read payment order intent: %w", err)
		}
		created, err := q.InsertPaymentOrderIntent(ctx, dbsqlc.InsertPaymentOrderIntentParams{
			ID: id, PublicOrderNumber: input.PublicOrderNumber, PublicToken: publicToken,
			UserID: input.UserID, ClientIdempotencyKey: input.ClientIdempotencyKey, RequestHash: input.RequestHash,
			StripeAccountID: input.StripeAccountID, StripeProductID: input.StripeProductID,
			IdeaQuantity: input.IdeaQuantity, TermsVersion: input.TermsVersion,
			RefundPolicyVersion: input.RefundPolicyVersion, ReturnBaseUrl: input.ReturnBaseURL, ExpiresAt: input.ExpiresAt,
		})
		if err != nil {
			return fmt.Errorf("insert payment order intent: %w", err)
		}
		order = paymentOrder(created)
		return nil
	})
	return order, reused, err
}

func (r *Repository) GetPaymentCustomer(ctx context.Context, userID int64) (PaymentCustomer, error) {
	if r == nil || r.db == nil {
		return PaymentCustomer{}, fmt.Errorf("payments repository: database unavailable")
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentCustomer(ctx, userID)
	if err != nil {
		return PaymentCustomer{}, err
	}
	return PaymentCustomer{
		UserID: row.UserID, StripeAccountID: row.StripeAccountID, StripeCustomerID: row.StripeCustomerID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *Repository) ReplacePaymentCustomer(ctx context.Context, customer PaymentCustomer) (PaymentCustomer, error) {
	if r == nil || r.db == nil {
		return PaymentCustomer{}, fmt.Errorf("payments repository: database unavailable")
	}
	row, err := r.dbWithContext(ctx).queries().ReplacePaymentCustomer(ctx, dbsqlc.ReplacePaymentCustomerParams{
		UserID: customer.UserID, StripeAccountID: customer.StripeAccountID, StripeCustomerID: customer.StripeCustomerID,
	})
	if err != nil {
		return PaymentCustomer{}, fmt.Errorf("replace payment customer: %w", err)
	}
	return PaymentCustomer{
		UserID: row.UserID, StripeAccountID: row.StripeAccountID, StripeCustomerID: row.StripeCustomerID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *Repository) BindPaymentOrderCheckout(ctx context.Context, input BindPaymentOrderCheckoutInput) (PaymentOrder, error) {
	if r == nil || r.db == nil {
		return PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := parseOrderUUID(input.OrderID)
	if err != nil {
		return PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.dbWithContext(ctx).queries().BindPaymentOrderCheckout(ctx, dbsqlc.BindPaymentOrderCheckoutParams{
		ID: id, StripeCustomerID: input.StripeCustomerID,
		StripeCheckoutSessionID: sql.NullString{String: input.StripeSessionID, Valid: true},
		CheckoutUrl:             input.CheckoutURL, ExpiresAt: input.ExpiresAt,
	})
	if err == nil {
		return paymentOrder(row), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PaymentOrder{}, fmt.Errorf("bind payment Checkout Session: %w", err)
	}
	existing, readErr := r.dbWithContext(ctx).queries().GetPaymentOrderByID(ctx, id)
	if readErr != nil {
		return PaymentOrder{}, fmt.Errorf("read payment order after bind conflict: %w", readErr)
	}
	if existing.StripeCustomerID != input.StripeCustomerID || existing.StripeCheckoutSessionID.String != input.StripeSessionID ||
		existing.CheckoutUrl != input.CheckoutURL || !existing.ExpiresAt.Equal(input.ExpiresAt) {
		return PaymentOrder{}, ErrCheckoutIdempotencyConflict
	}
	return paymentOrder(existing), nil
}

func (r *Repository) GetPaymentOrderByPublicToken(ctx context.Context, userID int64, token string) (PaymentOrder, error) {
	if r == nil || r.db == nil {
		return PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	publicToken, err := parseOrderUUID(token)
	if err != nil {
		return PaymentOrder{}, sql.ErrNoRows
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentOrderByPublicToken(ctx, dbsqlc.GetPaymentOrderByPublicTokenParams{
		PublicToken: publicToken, UserID: userID,
	})
	if err != nil {
		return PaymentOrder{}, err
	}
	return paymentOrder(row), nil
}

func (r *Repository) GetPaymentOrderByCheckoutSession(ctx context.Context, accountID, sessionID string) (PaymentOrder, error) {
	if r == nil || r.db == nil || strings.TrimSpace(accountID) == "" || strings.TrimSpace(sessionID) == "" {
		return PaymentOrder{}, sql.ErrNoRows
	}
	row, err := r.dbWithContext(ctx).queries().GetPaymentOrderByCheckoutSession(ctx, dbsqlc.GetPaymentOrderByCheckoutSessionParams{
		StripeAccountID:         accountID,
		StripeCheckoutSessionID: sql.NullString{String: sessionID, Valid: true},
	})
	if err != nil {
		return PaymentOrder{}, err
	}
	return paymentOrder(row), nil
}

func (r *Repository) ListActivePaymentOrders(ctx context.Context, userID int64) ([]PaymentOrder, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, fmt.Errorf("payments repository: invalid active order query")
	}
	rows, err := r.dbWithContext(ctx).queries().ListActivePaymentOrders(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active payment orders: %w", err)
	}
	orders := make([]PaymentOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, paymentOrder(row))
	}
	return orders, nil
}

func (r *Repository) ListUserPendingPaymentOrders(ctx context.Context, userID int64) ([]PaymentOrder, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, fmt.Errorf("payments repository: invalid pending order query")
	}
	rows, err := r.dbWithContext(ctx).queries().ListUserPendingPaymentOrders(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user pending payment orders: %w", err)
	}
	orders := make([]PaymentOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, paymentOrder(row))
	}
	return orders, nil
}

func (r *Repository) MarkPaymentOrderAsyncPending(ctx context.Context, orderID string) (PaymentOrder, error) {
	if r == nil || r.db == nil {
		return PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := parseOrderUUID(orderID)
	if err != nil {
		return PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.dbWithContext(ctx).queries().MarkPaymentOrderAsyncPending(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		row, err = r.dbWithContext(ctx).queries().GetPaymentOrderByID(ctx, id)
	}
	if err != nil {
		return PaymentOrder{}, fmt.Errorf("mark payment order async pending: %w", err)
	}
	return paymentOrder(row), nil
}

func (r *Repository) MarkPaymentOrderFailed(ctx context.Context, orderID string) (PaymentOrder, error) {
	if r == nil || r.db == nil {
		return PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := parseOrderUUID(orderID)
	if err != nil {
		return PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.dbWithContext(ctx).queries().MarkPaymentOrderFailed(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		row, err = r.dbWithContext(ctx).queries().GetPaymentOrderByID(ctx, id)
	}
	if err != nil {
		return PaymentOrder{}, fmt.Errorf("mark payment order failed: %w", err)
	}
	return paymentOrder(row), nil
}

func (r *Repository) MarkPaymentOrderExpired(ctx context.Context, orderID string) (PaymentOrder, bool, error) {
	id, err := parseOrderUUID(orderID)
	if err != nil {
		return PaymentOrder{}, false, ErrPaymentOrderNotFound
	}
	queries := r.dbWithContext(ctx).queries()
	row, err := queries.MarkPaymentOrderExpired(ctx, id)
	if err == nil {
		return paymentOrder(row), true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PaymentOrder{}, false, fmt.Errorf("mark payment order expired: %w", err)
	}
	existing, err := queries.GetPaymentOrderByID(ctx, id)
	if err != nil {
		return PaymentOrder{}, false, fmt.Errorf("read payment order after expiry replay: %w", err)
	}
	return paymentOrder(existing), false, nil
}

func (r *Repository) MarkPaymentOrderCancelled(ctx context.Context, userID int64, orderID string) (PaymentOrder, bool, error) {
	id, err := parseOrderUUID(orderID)
	if err != nil {
		return PaymentOrder{}, false, ErrPaymentOrderNotFound
	}
	queries := r.dbWithContext(ctx).queries()
	row, err := queries.MarkPaymentOrderCancelled(ctx, dbsqlc.MarkPaymentOrderCancelledParams{ID: id, UserID: userID})
	if err == nil {
		return paymentOrder(row), true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PaymentOrder{}, false, fmt.Errorf("mark payment order cancelled: %w", err)
	}
	existing, err := queries.GetPaymentOrderByID(ctx, id)
	if err != nil || existing.UserID != userID {
		return PaymentOrder{}, false, ErrPaymentOrderNotFound
	}
	return paymentOrder(existing), false, nil
}

func (r *Repository) ListUnsettledPaymentOrders(ctx context.Context) ([]PaymentOrder, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).queries().ListUnsettledPaymentOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsettled payment orders: %w", err)
	}
	orders := make([]PaymentOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, paymentOrder(row))
	}
	return orders, nil
}

func (r *Repository) ConvergePaidPaymentOrder(ctx context.Context, facts PaidCheckoutFacts, creditedAt time.Time) (PaymentConvergenceResult, error) {
	if r == nil || r.db == nil {
		return PaymentConvergenceResult{}, fmt.Errorf("payments repository: database unavailable")
	}
	order, replayed, err := r.RecordPaidPaymentOrder(ctx, facts)
	if err != nil {
		return PaymentConvergenceResult{}, err
	}
	if replayed && order.Status == PaymentOrderCredited {
		return r.replayedPaymentConvergence(ctx, order)
	}
	return r.CreditPaidPaymentOrder(ctx, order.ID, creditedAt)
}

func (r *Repository) RecordPaidPaymentOrder(ctx context.Context, facts PaidCheckoutFacts) (PaymentOrder, bool, error) {
	id, err := parseOrderUUID(facts.PaymentOrderID)
	if err != nil {
		return PaymentOrder{}, false, ErrCheckoutIdempotencyConflict
	}
	var result PaymentOrder
	var replayed bool
	err = r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ *sql.Tx) error {
		row, err := q.GetPaymentOrderByIDForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("lock payment order to record paid fact: %w", err)
		}
		order := paymentOrder(row)
		if err := validatePaidCheckoutFacts(order, facts); err != nil {
			return err
		}
		if order.Status == PaymentOrderPaidPendingCredit || order.Status == PaymentOrderCredited {
			if err := validateRecordedPaidCheckoutFacts(order, facts); err != nil {
				return err
			}
			result, replayed = order, true
			return nil
		}
		updated, err := q.RecordPaymentOrderPaid(ctx, dbsqlc.RecordPaymentOrderPaidParams{
			ID: row.ID, StripeCustomerID: facts.StripeCustomerID,
			StripeCheckoutSessionID: sql.NullString{String: facts.StripeSessionID, Valid: true},
			ExpiresAt:               facts.ExpiresAt,
			StripePaymentIntentID:   facts.StripePaymentIntentID, StripeChargeID: facts.StripeChargeID,
			PaidSubtotalMinor: sql.NullInt64{Int64: facts.PaidSubtotalMinor, Valid: true},
			PaidTaxMinor:      sql.NullInt64{Int64: facts.PaidTaxMinor, Valid: true},
			PaidTotalMinor:    sql.NullInt64{Int64: facts.PaidTotalMinor, Valid: true},
			PaidCurrency:      strings.ToLower(strings.TrimSpace(facts.PaidCurrency)), ReceiptUrl: facts.ReceiptURL,
			PaidAt: sql.NullTime{Time: facts.PaidAt, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("record paid payment order: %w", err)
		}
		result = paymentOrder(updated)
		return nil
	})
	return result, replayed, err
}

func (r *Repository) CreditPaidPaymentOrder(ctx context.Context, orderID string, creditedAt time.Time) (PaymentConvergenceResult, error) {
	id, err := parseOrderUUID(orderID)
	if err != nil {
		return PaymentConvergenceResult{}, ErrCheckoutIdempotencyConflict
	}
	var result PaymentConvergenceResult
	err = r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx *sql.Tx) error {
		row, err := q.GetPaymentOrderByIDForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("lock paid payment order for credit: %w", err)
		}
		order := paymentOrder(row)
		if order.Status == PaymentOrderCredited {
			var available int64
			if err := tx.QueryRowContext(ctx, `SELECT available_balance_after FROM credit_transactions WHERE transaction_id = $1`, order.CreditTransactionID).Scan(&available); err != nil {
				return fmt.Errorf("read replayed purchase balance: %w", err)
			}
			result = PaymentConvergenceResult{Order: order, AvailableIdeas: available, Replayed: true}
			return nil
		}
		if order.Status != PaymentOrderPaidPendingCredit {
			return fmt.Errorf("payment order is not ready for credit from status %q", order.Status)
		}
		amount, err := credits.NewCreditAmount(order.IdeaQuantity)
		if err != nil {
			return fmt.Errorf("build purchase credit amount: %w", err)
		}
		commands, err := credits.CommandsInSQLTx(tx)
		if err != nil {
			return err
		}
		creditResult, err := commands.Purchase(ctx, credits.PurchaseCommand{
			UserID: order.UserID, Amount: amount,
			Metadata: credits.CommandMetadata{
				IdempotencyKey: "payment-purchase:" + order.ID,
				Actor:          credits.Actor{Type: credits.ActorService, ID: "payments"},
				Source:         "stripe_purchase", Reason: "Stripe payment credited",
				BusinessReference: credits.BusinessReference{Type: "payment_order", ID: order.ID},
			},
		})
		if err != nil {
			return fmt.Errorf("credit paid payment order: %w", err)
		}
		updated, err := q.MarkPaymentOrderCredited(ctx, dbsqlc.MarkPaymentOrderCreditedParams{
			ID: row.ID, CreditTransactionID: sql.NullString{String: creditResult.TransactionID, Valid: true},
			CreditedAt: sql.NullTime{Time: creditedAt, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("mark payment order credited: %w", err)
		}
		result = PaymentConvergenceResult{
			Order: paymentOrder(updated), AvailableIdeas: creditResult.Wallet.Available, Replayed: creditResult.Replayed,
		}
		return nil
	})
	return result, err
}

func (r *Repository) replayedPaymentConvergence(ctx context.Context, order PaymentOrder) (PaymentConvergenceResult, error) {
	var available int64
	if err := r.dbWithContext(ctx).executor.QueryRowContext(ctx, `SELECT available_balance_after FROM credit_transactions WHERE transaction_id = $1`, order.CreditTransactionID).Scan(&available); err != nil {
		return PaymentConvergenceResult{}, fmt.Errorf("read replayed purchase balance: %w", err)
	}
	return PaymentConvergenceResult{Order: order, AvailableIdeas: available, Replayed: true}, nil
}

func validatePaidCheckoutFacts(order PaymentOrder, facts PaidCheckoutFacts) error {
	currency := strings.ToLower(strings.TrimSpace(facts.PaidCurrency))
	if facts.StripeAccountID != order.StripeAccountID ||
		facts.PublicOrderNumber != order.PublicOrderNumber || facts.PaymentOrderID != order.ID ||
		facts.RequestHash != order.RequestHash ||
		facts.IdeaQuantity != order.IdeaQuantity {
		return ErrCheckoutIdempotencyConflict
	}
	if order.StripeCheckoutSessionID != "" && facts.StripeSessionID != order.StripeCheckoutSessionID ||
		order.StripeCustomerID != "" && facts.StripeCustomerID != order.StripeCustomerID ||
		order.StripeCheckoutSessionID != "" && !facts.ExpiresAt.Equal(order.ExpiresAt) {
		return ErrCheckoutIdempotencyConflict
	}
	if strings.TrimSpace(facts.StripePaymentIntentID) == "" || facts.PaidSubtotalMinor < 0 || facts.PaidTaxMinor < 0 ||
		facts.PaidTotalMinor <= 0 || facts.PaidSubtotalMinor+facts.PaidTaxMinor != facts.PaidTotalMinor ||
		len(currency) != 3 || facts.PaidAt.IsZero() || facts.ExpiresAt.IsZero() || strings.TrimSpace(facts.StripeCustomerID) == "" ||
		strings.TrimSpace(facts.StripeSessionID) == "" {
		return fmt.Errorf("invalid paid Checkout facts")
	}
	return nil
}

func validateRecordedPaidCheckoutFacts(order PaymentOrder, facts PaidCheckoutFacts) error {
	if order.StripePaymentIntentID != facts.StripePaymentIntentID || order.StripeChargeID != facts.StripeChargeID ||
		order.PaidSubtotalMinor == nil || *order.PaidSubtotalMinor != facts.PaidSubtotalMinor ||
		order.PaidTaxMinor == nil || *order.PaidTaxMinor != facts.PaidTaxMinor ||
		order.PaidTotalMinor == nil || *order.PaidTotalMinor != facts.PaidTotalMinor ||
		order.PaidCurrency != strings.ToLower(strings.TrimSpace(facts.PaidCurrency)) || order.ReceiptURL != facts.ReceiptURL ||
		!order.ExpiresAt.Equal(facts.ExpiresAt) {
		return ErrCheckoutIdempotencyConflict
	}
	return nil
}

func insertConfigurationAudit(ctx context.Context, q *dbsqlc.Queries, audit ConfigurationAudit) error {
	metadata := audit.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if err := q.InsertPaymentAdminAction(ctx, dbsqlc.InsertPaymentAdminActionParams{
		ActionType: "configure_stripe", ActorID: audit.ActorID, Outcome: audit.Outcome,
		ErrorCode: audit.ErrorCode, ErrorSummary: audit.ErrorSummary, Metadata: metadata,
	}); err != nil {
		return fmt.Errorf("insert payment admin action: %w", err)
	}
	return nil
}

func storedStripeConfiguration(row dbsqlc.PaymentStripeConfiguration) StoredStripeConfiguration {
	return StoredStripeConfiguration{
		AccountID: row.AccountID, Mode: StripeMode(row.Mode), ChargesEnabled: row.ChargesEnabled,
		ProductTaxCode: row.ProductTaxCode, PurchasesEnabled: row.PurchasesEnabled,
		StripeProductID: row.StripeProductID, StripeWebhookEndpointID: row.StripeWebhookEndpointID,
		APIKeyCiphertext: row.ApiKeyCiphertext, WebhookSecretCiphertext: row.WebhookSecretCiphertext,
		SecretMaterialKind: row.SecretMaterialKind, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func isConfigurationNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
