package refunds

import (
	"context"
	"errors"
	"fmt"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	postgres "ideaboard/api/internal/payments/infrastructure/postgres"
	paymentports "ideaboard/api/internal/payments/ports"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const paymentRefundSelect = `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds`

func (s *Store) findRefundByStripeID(ctx context.Context, tx postgres.Queryer, stripeID string) (paymentsdomain.PaymentRefund, error) {
	var refund paymentsdomain.PaymentRefund
	err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE stripe_refund_id = $1 FOR UPDATE`, stripeID), &refund)
	return refund, err
}

func (s *Store) findRefundByIdempotency(ctx context.Context, tx postgres.Queryer, orderID, key string) (paymentsdomain.PaymentRefund, error) {
	var refund paymentsdomain.PaymentRefund
	err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE payment_order_id = $1::uuid AND idempotency_key = $2`, orderID, key), &refund)
	return refund, err
}

func (s *Store) findPendingRefund(ctx context.Context, tx postgres.Queryer, order postgres.PaymentOrderFacts, provider paymentports.StripeRefund) (paymentsdomain.PaymentRefund, error) {
	var pending paymentsdomain.PaymentRefund
	err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE payment_order_id = $1::uuid AND status = 'pending' AND stripe_refund_id = '' AND stripe_charge_id = $2 AND amount_minor = $3 AND LOWER(currency) = LOWER($4) AND actor_id IS NOT NULL ORDER BY created_at ASC, id ASC LIMIT 1 FOR UPDATE`, order.ID, provider.ChargeID, provider.AmountMinor, provider.Currency), &pending)
	return pending, err
}

func (s *Store) loadPendingRefundOrder(ctx context.Context, tx pgx.Tx, orderNumber string) (paymentsdomain.PaymentOrder, pgtype.Int8, pgtype.Int8, pgtype.Text, pgtype.Timestamptz, error) {
	var order paymentsdomain.PaymentOrder
	var paidSubtotal, paidTotal pgtype.Int8
	var paidCurrency pgtype.Text
	var paidAt pgtype.Timestamptz
	var billingAccountID pgtype.Int8
	if err := tx.QueryRow(ctx, `
SELECT id::text, public_order_number, user_id, billing_account_id, status, stripe_charge_id, idea_quantity,
       paid_subtotal_minor, paid_total_minor, paid_currency, paid_at
FROM payment_orders WHERE public_order_number = $1 FOR UPDATE`, orderNumber).Scan(
		&order.ID, &order.PublicOrderNumber, &order.UserID, &billingAccountID, &order.Status, &order.StripeChargeID,
		&order.IdeaQuantity, &paidSubtotal, &paidTotal, &paidCurrency, &paidAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paymentsdomain.PaymentOrder{}, pgtype.Int8{}, pgtype.Int8{}, pgtype.Text{}, pgtype.Timestamptz{}, paymentcontracts.ErrAdminPaymentOrderNotFound
		}
		return paymentsdomain.PaymentOrder{}, pgtype.Int8{}, pgtype.Int8{}, pgtype.Text{}, pgtype.Timestamptz{}, fmt.Errorf("read payment order for refund: %w", err)
	}
	order.BillingAccountID = 0
	if billingAccountID.Valid {
		order.BillingAccountID = billingAccountID.Int64
	}
	return order, paidSubtotal, paidTotal, paidCurrency, paidAt, nil
}

func findPendingRefundReplay(ctx context.Context, tx postgres.Queryer, order paymentsdomain.PaymentOrder, input paymentrefund.CreatePendingPaymentRefundInput) (paymentsdomain.PaymentRefund, bool, error) {
	var existing paymentsdomain.PaymentRefund
	err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE payment_order_id = $1::uuid AND idempotency_key = $2`, order.ID, input.IdempotencyKey), &existing)
	if err == nil {
		if existing.IdeaQuantity != input.IdeaQuantity {
			return paymentsdomain.PaymentRefund{}, false, paymentcontracts.ErrRefundIdempotency
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentRefund{}, false, fmt.Errorf("read refund idempotency: %w", err)
	}
	return paymentsdomain.PaymentRefund{}, false, nil
}

func loadPendingRefundBalances(ctx context.Context, tx postgres.Queryer, orderID string) (int64, int64, error) {
	exposure, err := postgres.QueryPaymentOrderExposure(ctx, tx, orderID)
	if err != nil {
		return 0, 0, err
	}
	return exposure.RefundedIdeas, exposure.DisputedIdeas, nil
}

func (s *Store) GetPaymentRefundByID(ctx context.Context, refundID string) (paymentsdomain.PaymentRefund, error) {
	if s == nil || !s.Available() {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("payments repository: database unavailable")
	}
	var refund paymentsdomain.PaymentRefund
	err := scanPaymentRefund(s.DBWithContext(ctx).Executor().QueryRow(ctx, paymentRefundSelect+` WHERE id = $1::uuid`, refundID), &refund)
	return refund, err
}

func (s *Store) GetPaymentRefundByStripeID(ctx context.Context, stripeRefundID string) (paymentsdomain.PaymentRefund, error) {
	if s == nil || !s.Available() {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("payments repository: database unavailable")
	}
	var refund paymentsdomain.PaymentRefund
	err := scanPaymentRefund(s.DBWithContext(ctx).Executor().QueryRow(ctx, paymentRefundSelect+` WHERE stripe_refund_id = $1`, stripeRefundID), &refund)
	return refund, err
}

func (s *Store) ListPendingPaymentRefunds(ctx context.Context, before time.Time, limit int) ([]paymentsdomain.PaymentRefund, error) {
	if s == nil || !s.Available() {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := s.DBWithContext(ctx).Executor().Query(ctx, `
SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency,
       status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id,
       credit_transaction_id, error_summary, created_at, updated_at
FROM payment_refunds
WHERE status = 'pending' AND updated_at <= $1
ORDER BY updated_at ASC, id ASC
LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refunds := make([]paymentsdomain.PaymentRefund, 0, limit)
	for rows.Next() {
		var refund paymentsdomain.PaymentRefund
		if err := scanPaymentRefund(rows, &refund); err != nil {
			return nil, err
		}
		refunds = append(refunds, refund)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refunds, nil
}

func (s *Store) ListPaymentRefundsForOrder(ctx context.Context, orderNumber string) ([]paymentsdomain.PaymentRefund, error) {
	if s == nil || !s.Available() {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := s.DBWithContext(ctx).Executor().Query(ctx, `SELECT refunds.id::text, refunds.payment_order_id::text, refunds.user_id, refunds.idea_quantity, refunds.amount_minor, refunds.currency, refunds.status, refunds.stripe_status, refunds.stripe_refund_id, refunds.stripe_charge_id, refunds.credit_hold_transaction_id, refunds.credit_transaction_id, refunds.error_summary, refunds.created_at, refunds.updated_at FROM payment_refunds refunds JOIN payment_orders orders ON orders.id = refunds.payment_order_id WHERE orders.public_order_number = $1 ORDER BY refunds.created_at DESC, refunds.id DESC`, orderNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refunds := make([]paymentsdomain.PaymentRefund, 0)
	for rows.Next() {
		var refund paymentsdomain.PaymentRefund
		if err := scanPaymentRefund(rows, &refund); err != nil {
			return nil, err
		}
		refunds = append(refunds, refund)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refunds, nil
}
