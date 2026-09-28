package postgres

import (
	"context"
	"errors"
	"fmt"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"ideaboard/api/internal/credits"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/google/uuid"
)

type refundQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type paymentRefundOrderFacts struct {
	ID           string
	PublicNumber string
	UserID       int64
	Status       paymentsdomain.PaymentOrderStatus
	IdeaQuantity int64
	PaidSubtotal int64
	PaidTotal    int64
	PaidCurrency string
	PaidAt       time.Time
}

// CreateExternalPaymentRefund links a provider refund discovered outside the
// Admin flow to the matching order, reserving available Ideas before settling.
func (r *Repository) CreateExternalPaymentRefund(ctx context.Context, provider paymentports.StripeRefund, now time.Time) (paymentsdomain.PaymentRefund, error) {
	if r == nil || r.db == nil || provider.ID == "" || provider.ChargeID == "" || provider.AmountMinor <= 0 {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundOrderNotEligible
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	order, err := r.paymentRefundOrderForCharge(ctx, tx, provider.ChargeID)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	key := "stripe-dashboard-" + provider.ID
	if existing, found, err := r.findExistingExternalRefund(ctx, tx, order.ID, provider.ID, key); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	} else if found {
		return existing, tx.Commit(ctx)
	}
	if err := validateExternalRefund(order, provider); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if pending, err := r.findPendingRefund(ctx, tx, order, provider); err == nil {
		if err := r.attachExternalProviderRefund(ctx, tx, pending, provider, now); err != nil {
			return paymentsdomain.PaymentRefund{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return paymentsdomain.PaymentRefund{}, err
		}
		pending.StripeRefundID, pending.StripeStatus, pending.UpdatedAt, pending.PublicOrderNumber = provider.ID, provider.Status, now, order.PublicNumber
		return pending, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentRefund{}, err
	}
	refund, err := r.createExternalRefundFact(ctx, tx, order, provider, key, now)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return refund, nil
}

func (r *Repository) findExistingExternalRefund(ctx context.Context, tx pgx.Tx, orderID, stripeRefundID, idempotencyKey string) (paymentsdomain.PaymentRefund, bool, error) {
	if linked, err := r.findRefundByStripeID(ctx, tx, stripeRefundID); err == nil {
		return linked, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	if existing, err := r.findRefundByIdempotency(ctx, tx, orderID, idempotencyKey); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	return paymentsdomain.PaymentRefund{}, false, nil
}

func (r *Repository) attachExternalProviderRefund(ctx context.Context, tx pgx.Tx, pending paymentsdomain.PaymentRefund, provider paymentports.StripeRefund, now time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE payment_refunds SET stripe_refund_id = $2, stripe_status = $3, updated_at = $4 WHERE id = $1::uuid`, pending.ID, provider.ID, provider.Status, now)
	return err
}

func (r *Repository) createExternalRefundFact(ctx context.Context, tx pgx.Tx, order paymentRefundOrderFacts, provider paymentports.StripeRefund, key string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	exposure, err := queryPaymentOrderExposure(ctx, tx, order.ID)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	already, alreadyAmount := exposure.refundedIdeas, exposure.refundedAmount
	disputedIdeas, disputedAmount := exposure.disputedIdeas, exposure.disputedAmount
	already += disputedIdeas
	qty := paymentsdomain.ProportionalRefundIdeas(provider.AmountMinor, order.PaidTotal, order.IdeaQuantity)
	if qty > order.IdeaQuantity {
		qty = order.IdeaQuantity
	}
	if qty <= 0 || order.IdeaQuantity-already <= 0 || provider.AmountMinor > order.PaidTotal-alreadyAmount-disputedAmount {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundAmountExceeds
	}
	if qty > order.IdeaQuantity-already {
		qty = order.IdeaQuantity - already
	}
	hold, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditAmount, err := domain.NewCreditAmount(qty)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	refundID := uuid.NewString()
	holdResult, err := hold.Hold(ctx, ports.HoldCommand{UserID: order.UserID, Amount: creditAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "refund-hold-" + refundID, Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe Dashboard refund", BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: refundID},
	}})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'pending', $8, $9, $10, $11, NULL, $12, $13, $13)`, refundID, order.ID, order.UserID, key, qty, provider.AmountMinor, strings.ToLower(provider.Currency), provider.Status, provider.ID, provider.ChargeID, holdResult.TransactionID, "Stripe Dashboard refund", now); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return paymentsdomain.PaymentRefund{ID: refundID, PaymentOrderID: order.ID, PublicOrderNumber: order.PublicNumber, UserID: order.UserID, IdeaQuantity: qty, AmountMinor: provider.AmountMinor, Currency: strings.ToLower(provider.Currency), Status: paymentsdomain.PaymentRefundPending, StripeStatus: provider.Status, StripeRefundID: provider.ID, StripeChargeID: provider.ChargeID, CreditHoldTransactionID: holdResult.TransactionID, CreatedAt: now, UpdatedAt: now}, nil
}

func (r *Repository) RecordPaymentRefundAnomaly(ctx context.Context, provider paymentports.StripeRefund, reason string, now time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	order, err := r.paymentRefundOrderForCharge(ctx, tx, provider.ChargeID)
	if err != nil {
		return 0, err
	}
	qty := paymentsdomain.ProportionalRefundIdeas(provider.AmountMinor, order.PaidTotal, order.IdeaQuantity)
	if qty < 1 {
		qty = 1
	}
	if qty > order.IdeaQuantity {
		qty = order.IdeaQuantity
	}
	_, err = tx.Exec(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, error_summary, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'anomaly', $8, $9, $10, '', NULL, 'Stripe Dashboard refund', $11, $12, $12) ON CONFLICT DO NOTHING`, uuid.NewString(), order.ID, order.UserID, "stripe-dashboard-"+provider.ID, qty, provider.AmountMinor, strings.ToLower(provider.Currency), provider.Status, provider.ID, provider.ChargeID, boundedText(reason, 500), now)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return order.UserID, nil
}

// CreatePendingPaymentRefund reserves Ideas and records the provider intent in
// one SQL transaction. Stripe I/O happens only after this commit.
func (r *Repository) CreatePendingPaymentRefund(ctx context.Context, input paymentrefund.CreatePendingPaymentRefundInput) (paymentsdomain.PaymentRefund, bool, error) {
	if !validPendingRefundRequest(r, input) {
		return paymentsdomain.PaymentRefund{}, false, paymentcontracts.ErrRefundAmountInvalid
	}
	if strings.TrimSpace(input.Reason) == "" {
		input.Reason = "Admin refund of unused Ideas"
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, false, fmt.Errorf("begin refund transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	order, paidSubtotal, paidTotal, paidCurrency, paidAt, err := r.loadPendingRefundOrder(ctx, tx, input.OrderNumber)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	if existing, replayed, err := findPendingRefundReplay(ctx, tx, order, input); err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	} else if replayed {
		return commitPendingRefundReplay(ctx, tx, existing, order)
	}
	if err := validatePendingRefundEligibility(order, paidSubtotal, paidTotal, paidAt, input.Now); err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	alreadyRefunded, disputedIdeas, err := loadPendingRefundBalances(ctx, tx, order.ID)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	remaining := order.IdeaQuantity - alreadyRefunded - disputedIdeas
	if remaining <= 0 || input.IdeaQuantity > remaining {
		return paymentsdomain.PaymentRefund{}, false, paymentcontracts.ErrRefundAmountExceeds
	}
	amountMinor := paymentsdomain.ProportionalRefundAmount(paidSubtotal.Int64, order.IdeaQuantity, alreadyRefunded+disputedIdeas, input.IdeaQuantity)
	if amountMinor <= 0 {
		amountMinor = 1
	}
	refund, err := r.persistPendingRefund(ctx, tx, order, paidCurrency.String, input, amountMinor)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return paymentsdomain.PaymentRefund{}, false, fmt.Errorf("commit payment refund: %w", err)
	}
	return refund, false, nil
}

func validPendingRefundRequest(r *Repository, input paymentrefund.CreatePendingPaymentRefundInput) bool {
	return r != nil && r.db != nil && input.ActorID > 0 && input.IdeaQuantity > 0
}

func commitPendingRefundReplay(ctx context.Context, tx pgx.Tx, existing paymentsdomain.PaymentRefund, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentRefund, bool, error) {
	if err := tx.Commit(ctx); err != nil {
		return paymentsdomain.PaymentRefund{}, false, fmt.Errorf("commit refund replay: %w", err)
	}
	existing.PublicOrderNumber = order.PublicOrderNumber
	existing.StripeChargeID = order.StripeChargeID
	return existing, true, nil
}

func (r *Repository) persistPendingRefund(ctx context.Context, tx pgx.Tx, order paymentsdomain.PaymentOrder, currency string, input paymentrefund.CreatePendingPaymentRefundInput, amountMinor int64) (paymentsdomain.PaymentRefund, error) {
	refundID := uuid.NewString()
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditAmount, err := domain.NewCreditAmount(input.IdeaQuantity)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundAmountInvalid
	}
	hold, err := commands.PurchaseRefundHold(ctx, ports.PurchaseRefundHoldCommand{UserID: order.UserID, Amount: creditAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "refund-hold-" + refundID, Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund_hold", Reason: input.Reason,
		BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: refundID},
	}})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	currency = strings.ToLower(currency)
	if _, err := tx.Exec(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'pending', $8, $9, $10, $11, $12, $12)`, refundID, order.ID, order.UserID, input.IdempotencyKey, input.IdeaQuantity, amountMinor, currency, order.StripeChargeID, hold.TransactionID, input.ActorID, input.Reason, input.Now); err != nil {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("insert payment refund: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, metadata) VALUES ('refund_payment', $1, 'payment_refund', $2, 'succeeded', jsonb_build_object('phase', 'reserved', 'payment_order_id', $3::uuid, 'idea_quantity', $4::bigint, 'amount_minor', $5::bigint, 'currency', $6::text))`, input.ActorID, refundID, order.ID, input.IdeaQuantity, amountMinor, currency); err != nil {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("record payment refund audit: %w", err)
	}
	return paymentsdomain.PaymentRefund{ID: refundID, PaymentOrderID: order.ID, PublicOrderNumber: order.PublicOrderNumber, UserID: order.UserID, IdeaQuantity: input.IdeaQuantity, AmountMinor: amountMinor, Currency: currency, Status: paymentsdomain.PaymentRefundPending, StripeChargeID: order.StripeChargeID, CreditHoldTransactionID: hold.TransactionID, CreatedAt: input.Now, UpdatedAt: input.Now}, nil
}

func (r *Repository) MarkPaymentRefundProviderResult(ctx context.Context, refundID, stripeRefundID, stripeStatus string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	var refund paymentsdomain.PaymentRefund
	err := r.dbWithContext(ctx).executor.QueryRow(ctx, `UPDATE payment_refunds SET stripe_refund_id = CASE WHEN stripe_refund_id = '' THEN $2 ELSE stripe_refund_id END, stripe_status = $3, updated_at = $4 WHERE id = $1::uuid AND status = 'pending' AND (stripe_refund_id = '' OR stripe_refund_id = $2) RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, stripeRefundID, stripeStatus, now).Scan(&refund.ID, &refund.PaymentOrderID, &refund.UserID, &refund.IdeaQuantity, &refund.AmountMinor, &refund.Currency, &refund.Status, &refund.StripeStatus, &refund.StripeRefundID, &refund.StripeChargeID, &refund.CreditHoldTransactionID, &refund.CreditTransactionID, &refund.ErrorSummary, &refund.CreatedAt, &refund.UpdatedAt)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return paymentsdomain.PaymentRefund{}, fmt.Errorf("record Stripe refund: %w", err)
		}
		return r.GetPaymentRefundByID(ctx, refundID)
	}
	return refund, nil
}

func (r *Repository) MarkPaymentRefundFailed(ctx context.Context, refundID, summary string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var refund paymentsdomain.PaymentRefund
	if err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE id = $1::uuid FOR UPDATE`, refundID), &refund); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if refund.Status == paymentsdomain.PaymentRefundFailed || refund.Status == paymentsdomain.PaymentRefundAnomaly {
		return refund, tx.Commit(ctx)
	}
	if refund.Status == paymentsdomain.PaymentRefundSucceeded {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundOrderNotEligible
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditAmount, err := domain.NewCreditAmount(refund.IdeaQuantity)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if _, err := commands.Release(ctx, ports.ReleaseCommand{UserID: refund.UserID, HoldTransactionID: refund.CreditHoldTransactionID, Amount: creditAmount, Metadata: domain.CommandMetadata{IdempotencyKey: "refund-release-" + refund.ID, Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe refund failed", BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: refund.ID}}}); err != nil && !errors.Is(err, domain.ErrHoldSettled) {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := scanPaymentRefund(tx.QueryRow(ctx, `UPDATE payment_refunds SET status = 'failed', error_summary = $2, failed_at = $3, updated_at = $3 WHERE id = $1::uuid RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, boundedText(summary, 500), now), &refund); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, error_code, error_summary, metadata) SELECT 'refund_payment', actor_id, 'payment_refund', id::text, 'failed', 'provider_failed', $2, jsonb_build_object('phase', 'released') FROM payment_refunds WHERE id = $1::uuid AND actor_id IS NOT NULL`, refundID, boundedText(summary, 500)); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return refund, nil
}

func (r *Repository) MarkPaymentRefundSucceeded(ctx context.Context, refundID string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var refund paymentsdomain.PaymentRefund
	if err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE id = $1::uuid FOR UPDATE`, refundID), &refund); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if refund.Status == paymentsdomain.PaymentRefundSucceeded || refund.Status == paymentsdomain.PaymentRefundAnomaly {
		return refund, tx.Commit(ctx)
	}
	if refund.Status == paymentsdomain.PaymentRefundFailed {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundOrderNotEligible
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditResult, err := commands.PurchaseRefund(ctx, ports.PurchaseRefundCommand{
		UserID: refund.UserID, HoldTransactionID: refund.CreditHoldTransactionID,
		Metadata: domain.CommandMetadata{IdempotencyKey: "purchase-refund-" + refund.ID,
			Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe refund succeeded",
			BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: refund.ID}},
	})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := scanPaymentRefund(tx.QueryRow(ctx, `UPDATE payment_refunds SET status = 'succeeded', stripe_status = 'succeeded', credit_transaction_id = $2, succeeded_at = $3, updated_at = $3 WHERE id = $1::uuid RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, creditResult.TransactionID, now), &refund); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, metadata) SELECT 'refund_payment', actor_id, 'payment_refund', id::text, 'succeeded', jsonb_build_object('phase', 'settled', 'credit_transaction_id', $2::text) FROM payment_refunds WHERE id = $1::uuid AND actor_id IS NOT NULL`, refundID, creditResult.TransactionID); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return refund, nil
}
