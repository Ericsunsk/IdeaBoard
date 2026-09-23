package postgres

import (
	"context"
	"errors"
	"fmt"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcheckout "ideaboard/api/internal/payments/application/checkout"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreatePaymentOrderIntent persists an idempotent checkout intent while
// serializing concurrent requests for the same user/client key.
func (r *Repository) CreatePaymentOrderIntent(ctx context.Context, input paymentsdomain.CreatePaymentOrderIntentInput) (paymentsdomain.PaymentOrder, bool, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := ParseUUID(input.ID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("parse payment order ID: %w", err)
	}
	publicToken, err := ParseUUID(input.PublicToken)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("parse payment order public token: %w", err)
	}
	stripeCredentialID, err := ParseUUID(input.StripeCredentialID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("parse Stripe credential ID: %w", err)
	}
	var order paymentsdomain.PaymentOrder
	var reused bool
	err = r.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries, tx pgx.Tx) error {
		lockKey := fmt.Sprintf("%d:%s", input.UserID, input.ClientIdempotencyKey)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 8188403870220423781))`, lockKey); err != nil {
			return fmt.Errorf("lock payment order idempotency: %w", err)
		}
		existing, err := q.GetPaymentOrderByIdempotency(ctx, dbsqlc.GetPaymentOrderByIdempotencyParams{
			UserID: input.UserID, ClientIdempotencyKey: input.ClientIdempotencyKey,
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
			UserID: input.UserID, ClientIdempotencyKey: input.ClientIdempotencyKey, RequestHash: input.RequestHash,
			StripeCredentialID: pgUUIDToPgType(stripeCredentialID), StripeAccountID: input.StripeAccountID, StripeProductID: input.StripeProductID,
			IdeaQuantity: input.IdeaQuantity, TermsVersion: input.TermsVersion,
			RefundPolicyVersion: input.RefundPolicyVersion, ReturnBaseUrl: input.ReturnBaseURL, ExpiresAt: dbutil.TimestamptzFromTime(input.ExpiresAt),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return paymentsdomain.ErrPurchasesUnavailable
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
	row, err := r.dbWithContext(ctx).queries().UpsertPaymentCustomer(ctx, dbsqlc.UpsertPaymentCustomerParams{
		UserID: customer.UserID, StripeAccountID: customer.StripeAccountID,
		StripeMode: string(customer.StripeMode), StripeCustomerID: customer.StripeCustomerID,
	})
	if err != nil {
		return paymentcheckout.PaymentCustomer{}, fmt.Errorf("upsert payment customer: %w", err)
	}
	return paymentcheckout.PaymentCustomer{
		UserID: row.UserID, StripeAccountID: row.StripeAccountID, StripeMode: paymentports.StripeMode(row.StripeMode), StripeCustomerID: row.StripeCustomerID,
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}, nil
}

func (r *Repository) BindPaymentOrderCheckout(ctx context.Context, input paymentsdomain.BindPaymentOrderCheckoutInput) (paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := ParseUUID(input.OrderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.dbWithContext(ctx).queries().BindPaymentOrderCheckout(ctx, dbsqlc.BindPaymentOrderCheckoutParams{
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
	existing, readErr := r.dbWithContext(ctx).queries().GetPaymentOrderByID(ctx, pgUUIDToPgType(id))
	if readErr != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("read payment order after bind conflict: %w", readErr)
	}
	if existing.StripeCustomerID != input.StripeCustomerID || pgTextString(existing.StripeCheckoutSessionID) != input.StripeSessionID ||
		existing.CheckoutUrl != input.CheckoutURL || !dbutil.TimeValue(existing.ExpiresAt).Equal(input.ExpiresAt) {
		return paymentsdomain.PaymentOrder{}, paymentsdomain.ErrCheckoutIdempotencyConflict
	}
	return PaymentOrder(existing), nil
}

func (r *Repository) MarkPaymentOrderAsyncPending(ctx context.Context, orderID string) (paymentsdomain.PaymentOrder, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("payments repository: database unavailable")
	}
	id, err := ParseUUID(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.dbWithContext(ctx).queries().MarkPaymentOrderAsyncPending(ctx, pgUUIDToPgType(id))
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = r.dbWithContext(ctx).queries().GetPaymentOrderByID(ctx, pgUUIDToPgType(id))
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
	id, err := ParseUUID(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("parse payment order ID: %w", err)
	}
	row, err := r.dbWithContext(ctx).queries().MarkPaymentOrderFailed(ctx, pgUUIDToPgType(id))
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = r.dbWithContext(ctx).queries().GetPaymentOrderByID(ctx, pgUUIDToPgType(id))
	}
	if err != nil {
		return paymentsdomain.PaymentOrder{}, fmt.Errorf("mark payment order failed: %w", err)
	}
	return PaymentOrder(row), nil
}

func (r *Repository) MarkPaymentOrderExpired(ctx context.Context, orderID string) (paymentsdomain.PaymentOrder, bool, error) {
	id, err := ParseUUID(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, paymentcontracts.ErrPaymentOrderNotFound
	}
	queries := r.dbWithContext(ctx).queries()
	row, err := queries.MarkPaymentOrderExpired(ctx, pgUUIDToPgType(id))
	if err == nil {
		return PaymentOrder(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("mark payment order expired: %w", err)
	}
	existing, err := queries.GetPaymentOrderByID(ctx, pgUUIDToPgType(id))
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("read payment order after expiry replay: %w", err)
	}
	return PaymentOrder(existing), false, nil
}

func (r *Repository) MarkPaymentOrderCancelled(ctx context.Context, userID int64, orderID string) (paymentsdomain.PaymentOrder, bool, error) {
	id, err := ParseUUID(orderID)
	if err != nil {
		return paymentsdomain.PaymentOrder{}, false, paymentcontracts.ErrPaymentOrderNotFound
	}
	queries := r.dbWithContext(ctx).queries()
	row, err := queries.MarkPaymentOrderCancelled(ctx, dbsqlc.MarkPaymentOrderCancelledParams{ID: pgUUIDToPgType(id), UserID: userID})
	if err == nil {
		return PaymentOrder(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentOrder{}, false, fmt.Errorf("mark payment order cancelled: %w", err)
	}
	existing, err := queries.GetPaymentOrderByID(ctx, pgUUIDToPgType(id))
	if err != nil || existing.UserID != userID {
		return paymentsdomain.PaymentOrder{}, false, paymentcontracts.ErrPaymentOrderNotFound
	}
	return PaymentOrder(existing), false, nil
}
