package postgres

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentsdomain "ideaboard/api/internal/payments/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const userCheckoutLifecycleLockSeed int64 = 412456873631731953

type Repository struct {
	db *pgxpool.Pool
}

type pgxTransactionStarter interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type paymentContextDB struct {
	ctx                context.Context
	executor           dbsqlc.DBTX
	transactionStarter pgxTransactionStarter
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) dbWithContext(ctx context.Context) paymentContextDB {
	if connection, ok := AdvisoryConnection(ctx); ok {
		return paymentContextDB{ctx: ctx, executor: connection, transactionStarter: connection}
	}
	return paymentContextDB{ctx: ctx, executor: r.db, transactionStarter: r.db}
}

func (r *Repository) WithUserCheckoutLock(ctx context.Context, userID int64, run func(context.Context) error) (resultErr error) {
	if r == nil || r.db == nil || userID <= 0 || run == nil {
		return fmt.Errorf("payments repository: invalid user Checkout lock")
	}
	lockKey := fmt.Sprintf("payment-user-checkout:%d", userID)
	return WithAdvisoryLock(ctx, r.db, lockKey, userCheckoutLifecycleLockSeed, run)
}

func (db paymentContextDB) queries() *dbsqlc.Queries {
	return dbsqlc.New(db.executor)
}

func (db paymentContextDB) withTx(run func(*dbsqlc.Queries, pgx.Tx) error) (resultErr error) {
	if db.transactionStarter == nil {
		return fmt.Errorf("payments repository: database unavailable")
	}
	tx, err := db.transactionStarter.BeginTx(db.ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin payments transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(db.ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && resultErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("rollback payments transaction: %w", rollbackErr))
		}
	}()
	if err := run(dbsqlc.New(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(db.ctx); err != nil {
		return fmt.Errorf("commit payments transaction: %w", err)
	}
	return nil
}

func (r *Repository) ConvergePaidPaymentOrder(ctx context.Context, facts paymentsdomain.PaidCheckoutFacts, creditedAt time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("payments repository: database unavailable")
	}
	order, _, err := r.RecordPaidPaymentOrder(ctx, facts)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, err
	}
	// Replay + already-credited short-circuit lives inside
	// creditLockedPaidOrderInTx; the two phases stay separate tx so a
	// credit failure leaves paid_pending_credit for replay/reconcile.
	return r.CreditPaidPaymentOrder(ctx, order.ID, creditedAt)
}

func (r *Repository) RecordPaidPaymentOrder(ctx context.Context, facts paymentsdomain.PaidCheckoutFacts) (paymentsdomain.PaymentOrder, bool, error) {
	id, err := ParseUUID(facts.PaymentOrderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	var result paymentsdomain.PaymentOrder
	var replayed bool
	err = r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, _ pgx.Tx) error {
		row, err := q.GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
		if err != nil {
			return fmt.Errorf("lock payment order to record paid fact: %w", err)
		}
		recorded, isReplay, err := recordPaidOrderInTx(ctx, q, row, facts)
		if err != nil {
			return err
		}
		result, replayed = recorded, isReplay
		return nil
	})
	return result, replayed, err
}

func recordPaidOrderInTx(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) (paymentsdomain.PaymentOrder, bool, error) {
	order := PaymentOrder(row)
	if err := validatePaidCheckoutFacts(order, facts); err != nil {
		return paymentsdomain.PaymentOrder{}, false, err
	}
	if order.Status == paymentsdomain.PaymentOrderPaidPendingCredit || order.Status == paymentsdomain.PaymentOrderCredited {
		if err := validateRecordedPaidCheckoutFacts(order, facts); err != nil {
			return paymentsdomain.PaymentOrder{}, false, err
		}
		return order, true, nil
	}
	updated, err := q.RecordPaymentOrderPaid(ctx, dbsqlc.RecordPaymentOrderPaidParams{
		ID: row.ID, StripeCustomerID: facts.StripeCustomerID,
		StripeCheckoutSessionID: pgtype.Text{String: facts.StripeSessionID, Valid: true},
		ExpiresAt:               dbutil.TimestamptzFromTime(facts.ExpiresAt),
		StripePaymentIntentID:   facts.StripePaymentIntentID, StripeChargeID: facts.StripeChargeID,
		PaidSubtotalMinor: pgtype.Int8{Int64: facts.PaidSubtotalMinor, Valid: true},
		PaidTaxMinor:      pgtype.Int8{Int64: facts.PaidTaxMinor, Valid: true},
		PaidTotalMinor:    pgtype.Int8{Int64: facts.PaidTotalMinor, Valid: true},
		PaidCurrency:      strings.ToLower(strings.TrimSpace(facts.PaidCurrency)), ReceiptUrl: facts.ReceiptURL,
		PaidAt: pgtype.Timestamptz{Time: facts.PaidAt, Valid: true},
	})
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("record paid payment order: %w", err)
	}
	return PaymentOrder(updated), false, nil
}

func (r *Repository) CreditPaidPaymentOrder(ctx context.Context, orderID string, creditedAt time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	id, err := ParseUUID(orderID)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	var result paymentsdomain.PaymentConvergenceResult
	err = r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx pgx.Tx) error {
		row, err := q.GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
		if err != nil {
			return fmt.Errorf("lock paid payment order for credit: %w", err)
		}
		credited, err := creditLockedPaidOrderInTx(ctx, q, tx, PaymentOrder(row), creditedAt)
		if err != nil {
			return err
		}
		result = credited
		return nil
	})
	return result, err
}

func creditLockedPaidOrderInTx(ctx context.Context, q *dbsqlc.Queries, tx pgx.Tx, order paymentsdomain.PaymentOrder, creditedAt time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	if order.Status == paymentsdomain.PaymentOrderCredited {
		var available int64
		if err := tx.QueryRow(ctx, `SELECT available_balance_after FROM credit_transactions WHERE transaction_id = $1`, order.CreditTransactionID).Scan(&available); err != nil {
			return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("read replayed purchase balance: %w", err)
		}
		return paymentsdomain.PaymentConvergenceResult{Order: order, AvailableIdeas: available, Replayed: true}, nil
	}
	if order.Status != paymentsdomain.PaymentOrderPaidPendingCredit {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("payment order is not ready for credit from status %q", order.Status)
	}
	amount, err := domain.NewCreditAmount(order.IdeaQuantity)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("build purchase credit amount: %w", err)
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, err
	}
	creditResult, err := commands.Purchase(ctx, ports.PurchaseCommand{
		UserID: order.UserID, Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "payment-purchase:" + order.ID,
			Actor:          domain.Actor{Type: domain.ActorService, ID: "payments"},
			Source:         "stripe_purchase", Reason: "Stripe payment credited",
			BusinessReference: domain.BusinessReference{Type: "payment_order", ID: order.ID},
		},
	})
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("credit paid payment order: %w", err)
	}
	id, err := ParseUUID(order.ID)
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	updated, err := q.MarkPaymentOrderCredited(ctx, dbsqlc.MarkPaymentOrderCreditedParams{
		ID: pgUUIDToPgType(id), CreditTransactionID: pgtype.Text{String: creditResult.TransactionID, Valid: true},
		CreditedAt: pgtype.Timestamptz{Time: creditedAt, Valid: true},
	})
	if err != nil {
		return paymentsdomain.PaymentConvergenceResult{}, fmt.Errorf("mark payment order credited: %w", err)
	}
	return paymentsdomain.PaymentConvergenceResult{
		Order: PaymentOrder(updated), AvailableIdeas: creditResult.Wallet.Available, Replayed: creditResult.Replayed,
	}, nil
}

func validatePaidCheckoutFacts(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) error {
	if paidCheckoutIdentityConflict(order, facts) {
		return paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	if paidCheckoutBindingConflict(order, facts) {
		return paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	if invalidPaidCheckoutPayload(facts) {
		return fmt.Errorf("invalid paid Checkout facts")
	}
	return nil
}

func paidCheckoutIdentityConflict(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) bool {
	if facts.StripeAccountID != order.StripeAccountID {
		return true
	}
	if facts.PublicOrderNumber != order.PublicOrderNumber {
		return true
	}
	if facts.PaymentOrderID != order.ID {
		return true
	}
	if facts.RequestHash != order.RequestHash {
		return true
	}
	return facts.IdeaQuantity != order.IdeaQuantity
}

func paidCheckoutBindingConflict(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) bool {
	if order.StripeCheckoutSessionID != "" && facts.StripeSessionID != order.StripeCheckoutSessionID {
		return true
	}
	if order.StripeCustomerID != "" && facts.StripeCustomerID != order.StripeCustomerID {
		return true
	}
	return order.StripeCheckoutSessionID != "" && !facts.ExpiresAt.Equal(order.ExpiresAt)
}

func invalidPaidCheckoutPayload(facts paymentsdomain.PaidCheckoutFacts) bool {
	currency := strings.ToLower(strings.TrimSpace(facts.PaidCurrency))
	if strings.TrimSpace(facts.StripePaymentIntentID) == "" {
		return true
	}
	if facts.PaidSubtotalMinor < 0 || facts.PaidTaxMinor < 0 {
		return true
	}
	if facts.PaidTotalMinor <= 0 || facts.PaidSubtotalMinor+facts.PaidTaxMinor != facts.PaidTotalMinor {
		return true
	}
	if len(currency) != 3 || facts.PaidAt.IsZero() || facts.ExpiresAt.IsZero() {
		return true
	}
	return strings.TrimSpace(facts.StripeCustomerID) == "" || strings.TrimSpace(facts.StripeSessionID) == ""
}

func validateRecordedPaidCheckoutFacts(order paymentsdomain.PaymentOrder, facts paymentsdomain.PaidCheckoutFacts) error {
	if order.StripePaymentIntentID != facts.StripePaymentIntentID || order.StripeChargeID != facts.StripeChargeID ||
		order.PaidSubtotalMinor == nil || *order.PaidSubtotalMinor != facts.PaidSubtotalMinor ||
		order.PaidTaxMinor == nil || *order.PaidTaxMinor != facts.PaidTaxMinor ||
		order.PaidTotalMinor == nil || *order.PaidTotalMinor != facts.PaidTotalMinor ||
		order.PaidCurrency != strings.ToLower(strings.TrimSpace(facts.PaidCurrency)) || order.ReceiptURL != facts.ReceiptURL ||
		!order.ExpiresAt.Equal(facts.ExpiresAt) {
		return paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	return nil
}
