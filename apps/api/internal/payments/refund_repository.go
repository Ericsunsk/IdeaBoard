package payments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"ideaboard/api/internal/credits"

	"github.com/google/uuid"
)

type refundQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type CreatePendingPaymentRefundInput struct {
	ActorID        int64
	OrderNumber    string
	IdeaQuantity   int64
	IdempotencyKey string
	Reason         string
	Now            time.Time
}

type paymentRefundOrderFacts struct {
	ID           string
	PublicNumber string
	UserID       int64
	Status       PaymentOrderStatus
	IdeaQuantity int64
	PaidSubtotal int64
	PaidTotal    int64
	PaidCurrency string
	PaidAt       time.Time
}

func (r *Repository) paymentRefundOrderForCharge(ctx context.Context, tx refundQueryer, chargeID string) (paymentRefundOrderFacts, error) {
	var order paymentRefundOrderFacts
	var paidAt sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT id::text, public_order_number, user_id, status, idea_quantity, paid_subtotal_minor, paid_total_minor, paid_currency, paid_at FROM payment_orders WHERE stripe_charge_id = $1 FOR UPDATE`, chargeID).Scan(
		&order.ID, &order.PublicNumber, &order.UserID, &order.Status, &order.IdeaQuantity, &order.PaidSubtotal, &order.PaidTotal, &order.PaidCurrency, &paidAt,
	)
	if err != nil {
		return paymentRefundOrderFacts{}, err
	}
	if paidAt.Valid {
		order.PaidAt = paidAt.Time
	}
	return order, nil
}

func proportionalRefundIdeas(amountMinor, subtotal, totalIdeas int64) int64 {
	if amountMinor <= 0 || subtotal <= 0 || totalIdeas <= 0 {
		return 0
	}
	n := new(big.Int).Mul(big.NewInt(amountMinor), big.NewInt(totalIdeas))
	n.Quo(n, big.NewInt(subtotal))
	if !n.IsInt64() {
		return 0
	}
	return n.Int64()
}

// CreateExternalPaymentRefund links a provider refund discovered outside the
// Admin flow to the matching order, reserving available Ideas before settling.
func (r *Repository) CreateExternalPaymentRefund(ctx context.Context, provider StripeRefund, now time.Time) (PaymentRefund, error) {
	if r == nil || r.db == nil || provider.ID == "" || provider.ChargeID == "" || provider.AmountMinor <= 0 {
		return PaymentRefund{}, ErrRefundOrderNotEligible
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PaymentRefund{}, err
	}
	defer func() { _ = tx.Rollback() }()
	order, err := r.paymentRefundOrderForCharge(ctx, tx, provider.ChargeID)
	if err != nil {
		return PaymentRefund{}, err
	}
	// A webhook can race the Admin request between Stripe's response and the
	// local provider-fact update. Resolve an already-linked provider ID first,
	// then attach the uniquely matching local intent before creating an external
	// refund fact. This keeps the provider refund and local hold one durable row.
	var linked PaymentRefund
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE stripe_refund_id = $1 FOR UPDATE`, provider.ID), &linked); err == nil {
		return linked, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PaymentRefund{}, err
	}
	key := "stripe-dashboard-" + provider.ID
	var existing PaymentRefund
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE payment_order_id = $1::uuid AND idempotency_key = $2`, order.ID, key), &existing); err == nil {
		return existing, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PaymentRefund{}, err
	}
	if order.Status != PaymentOrderCredited || order.PaidSubtotal <= 0 || order.PaidTotal <= 0 || order.IdeaQuantity <= 0 {
		return PaymentRefund{}, ErrRefundOrderNotEligible
	}
	if !strings.EqualFold(provider.Currency, order.PaidCurrency) {
		return PaymentRefund{}, ErrRefundProviderConflict
	}
	var pending PaymentRefund
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE payment_order_id = $1::uuid AND status = 'pending' AND stripe_refund_id = '' AND stripe_charge_id = $2 AND amount_minor = $3 AND LOWER(currency) = LOWER($4) AND actor_id IS NOT NULL ORDER BY created_at ASC, id ASC LIMIT 1 FOR UPDATE`, order.ID, provider.ChargeID, provider.AmountMinor, provider.Currency), &pending); err == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE payment_refunds SET stripe_refund_id = $2, stripe_status = $3, updated_at = $4 WHERE id = $1::uuid`, pending.ID, provider.ID, provider.Status, now); err != nil {
			return PaymentRefund{}, err
		}
		pending.StripeRefundID = provider.ID
		pending.StripeStatus = provider.Status
		pending.UpdatedAt = now
		if err := tx.Commit(); err != nil {
			return PaymentRefund{}, err
		}
		pending.PublicOrderNumber = order.PublicNumber
		return pending, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PaymentRefund{}, err
	}
	qty := proportionalRefundIdeas(provider.AmountMinor, order.PaidTotal, order.IdeaQuantity)
	if qty > order.IdeaQuantity {
		qty = order.IdeaQuantity
	}
	var already, alreadyAmount, disputedIdeas, disputedAmount int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(idea_quantity), 0), COALESCE(SUM(amount_minor), 0) FROM payment_refunds WHERE payment_order_id = $1::uuid AND status IN ('pending', 'succeeded', 'anomaly')`, order.ID).Scan(&already, &alreadyAmount); err != nil {
		return PaymentRefund{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(target_idea_quantity), 0), COALESCE(SUM(amount_minor), 0) FROM payment_disputes WHERE payment_order_id = $1::uuid AND status IN ('open', 'lost')`, order.ID).Scan(&disputedIdeas, &disputedAmount); err != nil {
		return PaymentRefund{}, err
	}
	already += disputedIdeas
	if qty <= 0 || order.IdeaQuantity-already <= 0 || provider.AmountMinor > order.PaidTotal-alreadyAmount-disputedAmount {
		return PaymentRefund{}, ErrRefundAmountExceeds
	}
	if qty > order.IdeaQuantity-already {
		qty = order.IdeaQuantity - already
	}
	hold, err := credits.CommandsInSQLTx(tx)
	if err != nil {
		return PaymentRefund{}, err
	}
	creditAmount, err := credits.NewCreditAmount(qty)
	if err != nil {
		return PaymentRefund{}, err
	}
	refundID := uuid.NewString()
	holdResult, err := hold.Hold(ctx, credits.HoldCommand{UserID: order.UserID, Amount: creditAmount, Metadata: credits.CommandMetadata{
		IdempotencyKey: "refund-hold-" + refundID, Actor: credits.Actor{Type: credits.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe Dashboard refund", BusinessReference: credits.BusinessReference{Type: "payment_refund", ID: refundID},
	}})
	if err != nil {
		return PaymentRefund{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'pending', $8, $9, $10, $11, NULL, $12, $13, $13)`, refundID, order.ID, order.UserID, key, qty, provider.AmountMinor, strings.ToLower(provider.Currency), provider.Status, provider.ID, provider.ChargeID, holdResult.TransactionID, "Stripe Dashboard refund", now); err != nil {
		return PaymentRefund{}, err
	}
	refund := PaymentRefund{ID: refundID, PaymentOrderID: order.ID, PublicOrderNumber: order.PublicNumber, UserID: order.UserID, IdeaQuantity: qty, AmountMinor: provider.AmountMinor, Currency: strings.ToLower(provider.Currency), Status: PaymentRefundPending, StripeStatus: provider.Status, StripeRefundID: provider.ID, StripeChargeID: provider.ChargeID, CreditHoldTransactionID: holdResult.TransactionID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Commit(); err != nil {
		return PaymentRefund{}, err
	}
	return refund, nil
}

func (r *Repository) RecordPaymentRefundAnomaly(ctx context.Context, provider StripeRefund, reason string, now time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	order, err := r.paymentRefundOrderForCharge(ctx, tx, provider.ChargeID)
	if err != nil {
		return 0, err
	}
	qty := proportionalRefundIdeas(provider.AmountMinor, order.PaidTotal, order.IdeaQuantity)
	if qty < 1 {
		qty = 1
	}
	if qty > order.IdeaQuantity {
		qty = order.IdeaQuantity
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, error_summary, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'anomaly', $8, $9, $10, '', NULL, 'Stripe Dashboard refund', $11, $12, $12) ON CONFLICT DO NOTHING`, uuid.NewString(), order.ID, order.UserID, "stripe-dashboard-"+provider.ID, qty, provider.AmountMinor, strings.ToLower(provider.Currency), provider.Status, provider.ID, provider.ChargeID, boundedText(reason, 500), now)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return order.UserID, nil
}

// CreatePendingPaymentRefund reserves Ideas and records the provider intent in
// one SQL transaction. Stripe I/O happens only after this commit.
func (r *Repository) CreatePendingPaymentRefund(ctx context.Context, input CreatePendingPaymentRefundInput) (PaymentRefund, bool, error) {
	if r == nil || r.db == nil || input.ActorID <= 0 || input.IdeaQuantity <= 0 {
		return PaymentRefund{}, false, ErrRefundAmountInvalid
	}
	if strings.TrimSpace(input.Reason) == "" {
		input.Reason = "Admin refund of unused Ideas"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PaymentRefund{}, false, fmt.Errorf("begin refund transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var order PaymentOrder
	var paidSubtotal, paidTotal sql.NullInt64
	var paidCurrency sql.NullString
	var paidAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT id::text, public_order_number, user_id, status, stripe_charge_id, idea_quantity,
       paid_subtotal_minor, paid_total_minor, paid_currency, paid_at
FROM payment_orders WHERE public_order_number = $1 FOR UPDATE`, input.OrderNumber).Scan(
		&order.ID, &order.PublicOrderNumber, &order.UserID, &order.Status, &order.StripeChargeID,
		&order.IdeaQuantity, &paidSubtotal, &paidTotal, &paidCurrency, &paidAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PaymentRefund{}, false, ErrAdminPaymentOrderNotFound
		}
		return PaymentRefund{}, false, fmt.Errorf("read payment order for refund: %w", err)
	}
	var existing PaymentRefund
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE payment_order_id = $1::uuid AND idempotency_key = $2`, order.ID, input.IdempotencyKey), &existing); err == nil {
		if existing.IdeaQuantity != input.IdeaQuantity {
			return PaymentRefund{}, false, ErrRefundIdempotency
		}
		if err := tx.Commit(); err != nil {
			return PaymentRefund{}, false, fmt.Errorf("commit refund replay: %w", err)
		}
		existing.PublicOrderNumber = order.PublicOrderNumber
		existing.StripeChargeID = order.StripeChargeID
		return existing, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PaymentRefund{}, false, fmt.Errorf("read refund idempotency: %w", err)
	}
	if order.Status != PaymentOrderCredited || order.StripeChargeID == "" || order.IdeaQuantity <= 0 || !paidSubtotal.Valid || paidSubtotal.Int64 <= 0 || !paidTotal.Valid || paidTotal.Int64 <= 0 || !paidAt.Valid {
		return PaymentRefund{}, false, ErrRefundOrderNotEligible
	}
	if !input.Now.Before(paidAt.Time.Add(refundEligibilityWindow)) {
		return PaymentRefund{}, false, ErrRefundExpired
	}
	var alreadyRefunded, disputedIdeas int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(idea_quantity), 0) FROM payment_refunds WHERE payment_order_id = $1::uuid AND status IN ('pending', 'succeeded', 'anomaly')`, order.ID).Scan(&alreadyRefunded); err != nil {
		return PaymentRefund{}, false, fmt.Errorf("sum existing refunds: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(target_idea_quantity), 0) FROM payment_disputes WHERE payment_order_id = $1::uuid AND status IN ('open', 'lost')`, order.ID).Scan(&disputedIdeas); err != nil {
		return PaymentRefund{}, false, fmt.Errorf("sum active Disputes: %w", err)
	}
	remaining := order.IdeaQuantity - alreadyRefunded - disputedIdeas
	if remaining <= 0 || input.IdeaQuantity > remaining {
		return PaymentRefund{}, false, ErrRefundAmountExceeds
	}
	amountMinor := proportionalRefundAmount(paidSubtotal.Int64, order.IdeaQuantity, alreadyRefunded+disputedIdeas, input.IdeaQuantity)
	if amountMinor <= 0 {
		amountMinor = 1
	}
	refundID := uuid.NewString()
	commands, err := credits.CommandsInSQLTx(tx)
	if err != nil {
		return PaymentRefund{}, false, err
	}
	creditAmount, err := credits.NewCreditAmount(input.IdeaQuantity)
	if err != nil {
		return PaymentRefund{}, false, ErrRefundAmountInvalid
	}
	hold, err := commands.PurchaseRefundHold(ctx, credits.PurchaseRefundHoldCommand{UserID: order.UserID, Amount: creditAmount, Metadata: credits.CommandMetadata{
		IdempotencyKey: "refund-hold-" + refundID, Actor: credits.Actor{Type: credits.ActorService, ID: "payments"}, Source: "stripe_refund_hold", Reason: input.Reason,
		BusinessReference: credits.BusinessReference{Type: "payment_refund", ID: refundID},
	}})
	if err != nil {
		return PaymentRefund{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'pending', $8, $9, $10, $11, $12, $12)`, refundID, order.ID, order.UserID, input.IdempotencyKey, input.IdeaQuantity, amountMinor, strings.ToLower(paidCurrency.String), order.StripeChargeID, hold.TransactionID, input.ActorID, input.Reason, input.Now); err != nil {
		return PaymentRefund{}, false, fmt.Errorf("insert payment refund: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, metadata) VALUES ('refund_payment', $1, 'payment_refund', $2, 'succeeded', jsonb_build_object('phase', 'reserved', 'payment_order_id', $3::uuid, 'idea_quantity', $4::bigint, 'amount_minor', $5::bigint, 'currency', $6::text))`, input.ActorID, refundID, order.ID, input.IdeaQuantity, amountMinor, strings.ToLower(paidCurrency.String)); err != nil {
		return PaymentRefund{}, false, fmt.Errorf("record payment refund audit: %w", err)
	}
	refund := PaymentRefund{ID: refundID, PaymentOrderID: order.ID, PublicOrderNumber: order.PublicOrderNumber, UserID: order.UserID, IdeaQuantity: input.IdeaQuantity, AmountMinor: amountMinor, Currency: strings.ToLower(paidCurrency.String), Status: PaymentRefundPending, StripeChargeID: order.StripeChargeID, CreditHoldTransactionID: hold.TransactionID, CreatedAt: input.Now, UpdatedAt: input.Now}
	if err := tx.Commit(); err != nil {
		return PaymentRefund{}, false, fmt.Errorf("commit payment refund: %w", err)
	}
	return refund, false, nil
}

func proportionalRefundAmount(subtotal, totalIdeas, alreadyRefunded, newIdeas int64) int64 {
	if subtotal <= 0 || totalIdeas <= 0 || newIdeas <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(subtotal), big.NewInt(alreadyRefunded+newIdeas))
	numerator.Quo(numerator, big.NewInt(totalIdeas))
	previous := new(big.Int).Mul(big.NewInt(subtotal), big.NewInt(alreadyRefunded))
	previous.Quo(previous, big.NewInt(totalIdeas))
	numerator.Sub(numerator, previous)
	if !numerator.IsInt64() {
		return 0
	}
	return numerator.Int64()
}

func scanPaymentRefund(row interface{ Scan(...any) error }, refund *PaymentRefund) error {
	return row.Scan(&refund.ID, &refund.PaymentOrderID, &refund.UserID, &refund.IdeaQuantity, &refund.AmountMinor, &refund.Currency, &refund.Status, &refund.StripeStatus, &refund.StripeRefundID, &refund.StripeChargeID, &refund.CreditHoldTransactionID, &refund.CreditTransactionID, &refund.ErrorSummary, &refund.CreatedAt, &refund.UpdatedAt)
}

func (r *Repository) MarkPaymentRefundProviderResult(ctx context.Context, refundID, stripeRefundID, stripeStatus string, now time.Time) (PaymentRefund, error) {
	var refund PaymentRefund
	err := r.dbWithContext(ctx).executor.QueryRowContext(ctx, `UPDATE payment_refunds SET stripe_refund_id = CASE WHEN stripe_refund_id = '' THEN $2 ELSE stripe_refund_id END, stripe_status = $3, updated_at = $4 WHERE id = $1::uuid AND status = 'pending' AND (stripe_refund_id = '' OR stripe_refund_id = $2) RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, stripeRefundID, stripeStatus, now).Scan(&refund.ID, &refund.PaymentOrderID, &refund.UserID, &refund.IdeaQuantity, &refund.AmountMinor, &refund.Currency, &refund.Status, &refund.StripeStatus, &refund.StripeRefundID, &refund.StripeChargeID, &refund.CreditHoldTransactionID, &refund.CreditTransactionID, &refund.ErrorSummary, &refund.CreatedAt, &refund.UpdatedAt)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return PaymentRefund{}, fmt.Errorf("record Stripe refund: %w", err)
		}
		return r.GetPaymentRefundByID(ctx, refundID)
	}
	return refund, nil
}

func (r *Repository) GetPaymentRefundByID(ctx context.Context, refundID string) (PaymentRefund, error) {
	if r == nil || r.db == nil {
		return PaymentRefund{}, fmt.Errorf("payments repository: database unavailable")
	}
	var refund PaymentRefund
	err := scanPaymentRefund(r.dbWithContext(ctx).executor.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE id = $1::uuid`, refundID), &refund)
	return refund, err
}

func (r *Repository) GetPaymentRefundByStripeID(ctx context.Context, stripeRefundID string) (PaymentRefund, error) {
	if r == nil || r.db == nil {
		return PaymentRefund{}, fmt.Errorf("payments repository: database unavailable")
	}
	var refund PaymentRefund
	err := scanPaymentRefund(r.dbWithContext(ctx).executor.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE stripe_refund_id = $1`, stripeRefundID), &refund)
	return refund, err
}

func (r *Repository) ListPendingPaymentRefunds(ctx context.Context, before time.Time, limit int) ([]PaymentRefund, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).executor.QueryContext(ctx, `
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
	refunds := make([]PaymentRefund, 0, limit)
	for rows.Next() {
		var refund PaymentRefund
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

func (r *Repository) ListPaymentRefundsForOrder(ctx context.Context, orderNumber string) ([]PaymentRefund, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("payments repository: database unavailable")
	}
	rows, err := r.dbWithContext(ctx).executor.QueryContext(ctx, `SELECT refunds.id::text, refunds.payment_order_id::text, refunds.user_id, refunds.idea_quantity, refunds.amount_minor, refunds.currency, refunds.status, refunds.stripe_status, refunds.stripe_refund_id, refunds.stripe_charge_id, refunds.credit_hold_transaction_id, refunds.credit_transaction_id, refunds.error_summary, refunds.created_at, refunds.updated_at FROM payment_refunds refunds JOIN payment_orders orders ON orders.id = refunds.payment_order_id WHERE orders.public_order_number = $1 ORDER BY refunds.created_at DESC, refunds.id DESC`, orderNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refunds := make([]PaymentRefund, 0)
	for rows.Next() {
		var refund PaymentRefund
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

func (r *Repository) MarkPaymentRefundFailed(ctx context.Context, refundID, summary string, now time.Time) (PaymentRefund, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PaymentRefund{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var refund PaymentRefund
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE id = $1::uuid FOR UPDATE`, refundID), &refund); err != nil {
		return PaymentRefund{}, err
	}
	if refund.Status == PaymentRefundFailed || refund.Status == PaymentRefundAnomaly {
		return refund, tx.Commit()
	}
	if refund.Status == PaymentRefundSucceeded {
		return PaymentRefund{}, ErrRefundOrderNotEligible
	}
	commands, err := credits.CommandsInSQLTx(tx)
	if err != nil {
		return PaymentRefund{}, err
	}
	creditAmount, err := credits.NewCreditAmount(refund.IdeaQuantity)
	if err != nil {
		return PaymentRefund{}, err
	}
	if _, err := commands.Release(ctx, credits.ReleaseCommand{UserID: refund.UserID, HoldTransactionID: refund.CreditHoldTransactionID, Amount: creditAmount, Metadata: credits.CommandMetadata{IdempotencyKey: "refund-release-" + refund.ID, Actor: credits.Actor{Type: credits.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe refund failed", BusinessReference: credits.BusinessReference{Type: "payment_refund", ID: refund.ID}}}); err != nil && !errors.Is(err, credits.ErrHoldSettled) {
		return PaymentRefund{}, err
	}
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `UPDATE payment_refunds SET status = 'failed', error_summary = $2, failed_at = $3, updated_at = $3 WHERE id = $1::uuid RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, boundedText(summary, 500), now), &refund); err != nil {
		return PaymentRefund{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, error_code, error_summary, metadata) SELECT 'refund_payment', actor_id, 'payment_refund', id::text, 'failed', 'provider_failed', $2, jsonb_build_object('phase', 'released') FROM payment_refunds WHERE id = $1::uuid AND actor_id IS NOT NULL`, refundID, boundedText(summary, 500)); err != nil {
		return PaymentRefund{}, err
	}
	if err := tx.Commit(); err != nil {
		return PaymentRefund{}, err
	}
	return refund, nil
}

func (r *Repository) MarkPaymentRefundSucceeded(ctx context.Context, refundID string, now time.Time) (PaymentRefund, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return PaymentRefund{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var refund PaymentRefund
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `SELECT id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at FROM payment_refunds WHERE id = $1::uuid FOR UPDATE`, refundID), &refund); err != nil {
		return PaymentRefund{}, err
	}
	if refund.Status == PaymentRefundSucceeded || refund.Status == PaymentRefundAnomaly {
		return refund, tx.Commit()
	}
	if refund.Status == PaymentRefundFailed {
		return PaymentRefund{}, ErrRefundOrderNotEligible
	}
	commands, err := credits.CommandsInSQLTx(tx)
	if err != nil {
		return PaymentRefund{}, err
	}
	creditResult, err := commands.PurchaseRefund(ctx, credits.PurchaseRefundCommand{
		UserID: refund.UserID, HoldTransactionID: refund.CreditHoldTransactionID,
		Metadata: credits.CommandMetadata{IdempotencyKey: "purchase-refund-" + refund.ID,
			Actor: credits.Actor{Type: credits.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe refund succeeded",
			BusinessReference: credits.BusinessReference{Type: "payment_refund", ID: refund.ID}},
	})
	if err != nil {
		return PaymentRefund{}, err
	}
	if err := scanPaymentRefund(tx.QueryRowContext(ctx, `UPDATE payment_refunds SET status = 'succeeded', stripe_status = 'succeeded', credit_transaction_id = $2, succeeded_at = $3, updated_at = $3 WHERE id = $1::uuid RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, creditResult.TransactionID, now), &refund); err != nil {
		return PaymentRefund{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, metadata) SELECT 'refund_payment', actor_id, 'payment_refund', id::text, 'succeeded', jsonb_build_object('phase', 'settled', 'credit_transaction_id', $2::text) FROM payment_refunds WHERE id = $1::uuid AND actor_id IS NOT NULL`, refundID, creditResult.TransactionID); err != nil {
		return PaymentRefund{}, err
	}
	if err := tx.Commit(); err != nil {
		return PaymentRefund{}, err
	}
	return refund, nil
}
