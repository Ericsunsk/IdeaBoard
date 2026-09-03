package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
)

type paymentDisputeScanner interface {
	Scan(dest ...any) error
}

func scanPaymentDispute(row paymentDisputeScanner) (paymentsdomain.PaymentDispute, error) {
	var dispute paymentsdomain.PaymentDispute
	var restrictionID, debitID, restoreID sql.NullString
	var closedAt sql.NullTime
	if err := row.Scan(
		&dispute.ID, &dispute.PaymentOrderID, &dispute.UserID, &dispute.StripeDisputeID,
		&dispute.StripeChargeID, &dispute.StripePaymentIntentID, &dispute.ProviderStatus, &dispute.Status,
		&dispute.AmountMinor, &dispute.Currency, &dispute.TargetIdeaQuantity, &dispute.DebitedIdeaQuantity,
		&dispute.UnrecoveredCredits, &restrictionID, &debitID, &restoreID, &dispute.OpenedAt, &closedAt,
		&dispute.NextReconciliationAt, &dispute.CreatedAt, &dispute.UpdatedAt,
	); err != nil {
		return paymentsdomain.PaymentDispute{}, err
	}
	dispute.RestrictionID, dispute.DebitTransactionID, dispute.RestoreTransactionID = restrictionID.String, debitID.String, restoreID.String
	if closedAt.Valid {
		dispute.ClosedAt = &closedAt.Time
	}
	return dispute, nil
}

func (r *Repository) ListDuePaymentDisputes(ctx context.Context, now time.Time, limit int) ([]paymentsdomain.PaymentDispute, error) {
	if r == nil || r.db == nil {
		return nil, paymentcontracts.ErrDisputeUnavailable
	}
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := r.dbWithContext(ctx).executor.QueryContext(ctx, `
SELECT id::text, payment_order_id::text, user_id, stripe_dispute_id, stripe_charge_id,
       stripe_payment_intent_id, provider_status, status, amount_minor, currency,
       target_idea_quantity, debited_idea_quantity, unrecovered_credits,
       restriction_id::text, debit_transaction_id, restore_transaction_id,
       opened_at, closed_at, next_reconciliation_at, created_at, updated_at
FROM payment_disputes
WHERE status IN ('open', 'lost') AND next_reconciliation_at <= $1
ORDER BY next_reconciliation_at, id
LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]paymentsdomain.PaymentDispute, 0, limit)
	for rows.Next() {
		dispute, err := scanPaymentDispute(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, dispute)
	}
	return result, rows.Err()
}

func (r *Repository) ListPaymentDisputesForOrder(ctx context.Context, orderNumber string) ([]paymentsdomain.PaymentDispute, error) {
	if r == nil || r.db == nil {
		return nil, paymentcontracts.ErrDisputeUnavailable
	}
	rows, err := r.dbWithContext(ctx).executor.QueryContext(ctx, `
SELECT d.id::text, d.payment_order_id::text, d.user_id, d.stripe_dispute_id, d.stripe_charge_id,
       d.stripe_payment_intent_id, d.provider_status, d.status, d.amount_minor, d.currency,
       d.target_idea_quantity, d.debited_idea_quantity, d.unrecovered_credits,
       d.restriction_id::text, d.debit_transaction_id, d.restore_transaction_id,
       d.opened_at, d.closed_at, d.next_reconciliation_at, d.created_at, d.updated_at
FROM payment_disputes d
JOIN payment_orders o ON o.id = d.payment_order_id
WHERE o.public_order_number = $1
ORDER BY d.opened_at, d.id`, strings.TrimSpace(orderNumber))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]paymentsdomain.PaymentDispute, 0)
	for rows.Next() {
		dispute, err := scanPaymentDispute(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, dispute)
	}
	return result, rows.Err()
}
