package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// PaymentOrderFacts carries the order identity and paid facts shared by the
// refunds and disputes subpackages when they resolve an order by its Stripe
// charge ID. It lives on the order aggregate (root) because both subpackages
// load it the same way. BillingAccountID is the fixed payer funds identity
// (#920): refund/dispute settlement converges on this row-inline account,
// never re-deriving from the operator.
type PaymentOrderFacts struct {
	ID               string
	PublicNumber     string
	UserID           int64
	BillingAccountID int64
	Status           paymentsdomain.PaymentOrderStatus
	IdeaQuantity     int64
	PaidSubtotal     int64
	PaidTotal        int64
	PaidCurrency     string
	PaidAt           time.Time
}

// LoadPaymentOrderFactsForCharge locks the order behind a Stripe charge so
// refund and dispute intake serialize on the same row.
func LoadPaymentOrderFactsForCharge(ctx context.Context, q Queryer, chargeID string) (PaymentOrderFacts, error) {
	var order PaymentOrderFacts
	var paidAt pgtype.Timestamptz
	var billingAccountID pgtype.Int8
	err := q.QueryRow(ctx, `SELECT id::text, public_order_number, user_id, billing_account_id, status, idea_quantity, paid_subtotal_minor, paid_total_minor, paid_currency, paid_at FROM payment_orders WHERE stripe_charge_id = $1 FOR UPDATE`, chargeID).Scan(
		&order.ID, &order.PublicNumber, &order.UserID, &billingAccountID, &order.Status, &order.IdeaQuantity, &order.PaidSubtotal, &order.PaidTotal, &order.PaidCurrency, &paidAt,
	)
	if err != nil {
		return PaymentOrderFacts{}, err
	}
	order.BillingAccountID = pgBillingAccountID(billingAccountID)
	if paidAt.Valid {
		order.PaidAt = paidAt.Time
	}
	return order, nil
}

// CreatePaymentOrderIntent persists an idempotent checkout intent while
// serializing concurrent requests for the same payer account/client key.
// The durable replay scope is the payer Billing Account (#919): same key +
// same hash replays, same key + different hash conflicts, and cross-account
// key reuse never replays another account's order.
func (r *Repository) CreatePaymentOrderIntent(ctx context.Context, input paymentsdomain.CreatePaymentOrderIntentInput) (paymentsdomain.PaymentOrder, bool, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("payments repository: database unavailable")
	}
	if input.BillingAccountID <= 0 || input.UserID <= 0 {
		return paymentsdomain.PaymentOrder{}, false, paymentsdomain.ErrCheckoutIdempotencyKey
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("parse payment order ID: %w", err)
	}
	publicToken, err := uuid.Parse(input.PublicToken)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("parse payment order public token: %w", err)
	}
	stripeCredentialID, err := uuid.Parse(input.StripeCredentialID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("parse Stripe credential ID: %w", err)
	}
	var order paymentsdomain.PaymentOrder
	var reused bool
	err = r.DBWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx pgx.Tx) error {
		lockKey := fmt.Sprintf("%d:%s", input.BillingAccountID, input.ClientIdempotencyKey)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 8188403870220423781))`, lockKey); err != nil {
			return fmt.Errorf("lock payment order idempotency: %w", err)
		}
		existing, err := q.GetPaymentOrderByAccountIdempotency(ctx, dbsqlc.GetPaymentOrderByAccountIdempotencyParams{
			BillingAccountID: pgtype.Int8{Int64: input.BillingAccountID, Valid: true}, ClientIdempotencyKey: input.ClientIdempotencyKey,
		})
		if err == nil {
			if existing.RequestHash != input.RequestHash {
				return paymentsdomain.ErrCheckoutIdempotencyConflict
			}
			order, reused = PaymentOrder(existing), true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read payment order intent: %w", err)
		}
		created, err := q.InsertPaymentOrderIntent(ctx, dbsqlc.InsertPaymentOrderIntentParams{
			ID: pgUUIDToPgType(id), PublicOrderNumber: input.PublicOrderNumber, PublicToken: pgUUIDToPgType(publicToken),
			UserID: input.UserID, BillingAccountID: pgtype.Int8{Int64: input.BillingAccountID, Valid: true},
			ClientIdempotencyKey: input.ClientIdempotencyKey, RequestHash: input.RequestHash,
			StripeCredentialID: pgUUIDToPgType(stripeCredentialID), StripeAccountID: input.StripeAccountID, StripeProductID: input.StripeProductID,
			IdeaQuantity: input.IdeaQuantity, TermsVersion: input.TermsVersion,
			RefundPolicyVersion: input.RefundPolicyVersion, ReturnBaseUrl: input.ReturnBaseURL, ExpiresAt: dbutil.TimestamptzFromTime(input.ExpiresAt),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return paymentsdomain.ErrPurchasesUnavailable
			}
			if database.IsDuplicateConstraintError(err) {
				// A uniqueness race must not replay an unrelated order.
				// Durable replay is resolved above within the payer scope.
				return paymentsdomain.ErrCheckoutIdempotencyConflict
			}
			return fmt.Errorf("insert payment order intent: %w", err)
		}
		order = PaymentOrder(created)
		return nil
	})
	return order, reused, err
}

func (r *Repository) UpsertPaymentCustomer(ctx context.Context, customer paymentcheckout.PaymentCustomer) (paymentcheckout.PaymentCustomer, error) {
	if r == nil || r.db == nil {
		return paymentcheckout.PaymentCustomer{}, fmt.Errorf("payments repository: database unavailable")
	}
	if customer.BillingAccountID <= 0 {
		return paymentcheckout.PaymentCustomer{}, fmt.Errorf("payments repository: invalid payer Billing Account")
	}
	row, err := r.DBWithContext(ctx).Queries().UpsertPaymentCustomerByAccount(ctx, dbsqlc.UpsertPaymentCustomerByAccountParams{
		UserID: customer.UserID, BillingAccountID: customer.BillingAccountID,
		StripeAccountID: customer.StripeAccountID,
		StripeMode:      string(customer.StripeMode), StripeCustomerID: customer.StripeCustomerID,
	})
	if err != nil {
		return paymentcheckout.PaymentCustomer{}, fmt.Errorf("upsert payment customer: %w", err)
	}
	return paymentcheckout.PaymentCustomer{
		UserID: row.UserID, BillingAccountID: row.BillingAccountID,
		StripeAccountID: row.StripeAccountID, StripeMode: paymentports.StripeMode(row.StripeMode), StripeCustomerID: row.StripeCustomerID,
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}, nil
}

func (r *Repository) BindPaymentOrderCheckout(ctx context.Context, input paymentsdomain.BindPaymentOrderCheckoutInput) (paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := uuid.Parse(input.OrderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.DBWithContext(ctx).Queries().BindPaymentOrderCheckout(ctx, dbsqlc.BindPaymentOrderCheckoutParams{
		ID: pgUUIDToPgType(id), StripeCustomerID: input.StripeCustomerID,
		StripeCheckoutSessionID: pgtype.Text{String: input.StripeSessionID, Valid: true},
		CheckoutUrl:             input.CheckoutURL, ExpiresAt: dbutil.TimestamptzFromTime(input.ExpiresAt),
	})
	if err == nil {
		return PaymentOrder(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("bind payment Checkout Session: %w", err)
	}
	existing, readErr := r.DBWithContext(ctx).Queries().GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
	if readErr != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("read payment order after bind conflict: %w", readErr)
	}
	if existing.StripeCustomerID != input.StripeCustomerID || PgTextString(existing.StripeCheckoutSessionID) != input.StripeSessionID ||
		existing.CheckoutUrl != input.CheckoutURL || !dbutil.TimeValue(existing.ExpiresAt).Equal(input.ExpiresAt) {
		return paymentsdomain.PaymentOrder{}, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	return PaymentOrder(existing), nil
}

func (r *Repository) MarkPaymentOrderAsyncPending(ctx context.Context, orderID string) (paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := uuid.Parse(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.DBWithContext(ctx).Queries().MarkPaymentOrderAsyncPending(ctx, pgUUIDToPgType(id))
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = r.DBWithContext(ctx).Queries().GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
	}
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("mark payment order async pending: %w", err)
	}
	return PaymentOrder(row), nil
}

func (r *Repository) MarkPaymentOrderFailed(ctx context.Context, orderID string) (paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := uuid.Parse(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.DBWithContext(ctx).Queries().MarkPaymentOrderFailed(ctx, pgUUIDToPgType(id))
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = r.DBWithContext(ctx).Queries().GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
	}
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("mark payment order failed: %w", err)
	}
	return PaymentOrder(row), nil
}

func (r *Repository) MarkPaymentOrderExpired(ctx context.Context, orderID string) (paymentsdomain.PaymentOrder, bool, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, paymentcontracts.ErrPaymentOrderNotFound
	}
	queries := r.DBWithContext(ctx).Queries()
	row, err := queries.MarkPaymentOrderExpired(ctx, pgUUIDToPgType(id))
	if err == nil {
		return PaymentOrder(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("mark payment order expired: %w", err)
	}
	existing, err := queries.GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("read payment order after expiry replay: %w", err)
	}
	return PaymentOrder(existing), false, nil
}

func (r *Repository) MarkPaymentOrderCancelled(ctx context.Context, billingAccountID int64, orderID string) (paymentsdomain.PaymentOrder, bool, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, paymentcontracts.ErrPaymentOrderNotFound
	}
	queries := r.DBWithContext(ctx).Queries()
	row, err := queries.MarkPaymentOrderCancelledByAccount(ctx, dbsqlc.MarkPaymentOrderCancelledByAccountParams{ID: pgUUIDToPgType(id), BillingAccountID: pgtype.Int8{Int64: billingAccountID, Valid: true}})
	if err == nil {
		return PaymentOrder(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("mark payment order cancelled: %w", err)
	}
	existing, err := queries.GetPaymentOrderByIDForUpdate(ctx, pgUUIDToPgType(id))
	if err != nil || pgBillingAccountID(existing.BillingAccountID) != billingAccountID {
		return paymentsdomain.PaymentOrder{}, false, paymentcontracts.ErrPaymentOrderNotFound
	}
	return PaymentOrder(existing), false, nil
}
