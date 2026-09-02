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

type paymentDisputeScanner interface {
	Scan(dest ...any) error
}

func scanPaymentDispute(row paymentDisputeScanner) (PaymentDispute, error) {
	var dispute PaymentDispute
	var restrictionID, debitID, restoreID sql.NullString
	var closedAt sql.NullTime
	if err := row.Scan(
		&dispute.ID, &dispute.PaymentOrderID, &dispute.UserID, &dispute.StripeDisputeID,
		&dispute.StripeChargeID, &dispute.StripePaymentIntentID, &dispute.ProviderStatus, &dispute.Status,
		&dispute.AmountMinor, &dispute.Currency, &dispute.TargetIdeaQuantity, &dispute.DebitedIdeaQuantity,
		&dispute.UnrecoveredCredits, &restrictionID, &debitID, &restoreID, &dispute.OpenedAt, &closedAt,
		&dispute.NextReconciliationAt, &dispute.CreatedAt, &dispute.UpdatedAt,
	); err != nil {
		return PaymentDispute{}, err
	}
	dispute.RestrictionID, dispute.DebitTransactionID, dispute.RestoreTransactionID = restrictionID.String, debitID.String, restoreID.String
	if closedAt.Valid {
		dispute.ClosedAt = &closedAt.Time
	}
	return dispute, nil
}

const disputeReconciliationInterval = time.Hour

func proportionalDisputeIdeas(amountMinor, paidTotal, totalIdeas int64) int64 {
	if amountMinor <= 0 || paidTotal <= 0 || totalIdeas <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(amountMinor), big.NewInt(totalIdeas))
	numerator.Add(numerator, big.NewInt(paidTotal-1))
	numerator.Quo(numerator, big.NewInt(paidTotal))
	if !numerator.IsInt64() {
		return 0
	}
	return numerator.Int64()
}

func (r *Repository) ConvergePaymentDispute(ctx context.Context, provider StripeDispute, now time.Time) (PaymentDispute, bool, error) {
	provider, desiredStatus, err := normalizePaymentDisputeProvider(r, provider)
	if err != nil {
		return PaymentDispute{}, false, err
	}
	tx, err := r.dbWithContext(ctx).transactionStarter.BeginTx(ctx, nil)
	if err != nil {
		return PaymentDispute{}, false, fmt.Errorf("begin Dispute transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	dispute, changed, err := r.convergePaymentDisputeTx(ctx, tx, provider, desiredStatus, now)
	if err != nil {
		return PaymentDispute{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return PaymentDispute{}, false, err
	}
	return dispute, changed, nil
}

func normalizePaymentDisputeProvider(r *Repository, provider StripeDispute) (StripeDispute, PaymentDisputeStatus, error) {
	if r == nil || r.db == nil || strings.TrimSpace(provider.ID) == "" || strings.TrimSpace(provider.ChargeID) == "" || provider.AmountMinor <= 0 {
		return StripeDispute{}, "", ErrDisputeOrderNotEligible
	}
	provider.ID = strings.TrimSpace(provider.ID)
	provider.ChargeID = strings.TrimSpace(provider.ChargeID)
	provider.PaymentIntentID = strings.TrimSpace(provider.PaymentIntentID)
	provider.Status = strings.ToLower(strings.TrimSpace(provider.Status))
	provider.Currency = strings.ToLower(strings.TrimSpace(provider.Currency))
	return provider, canonicalPaymentDisputeStatus(provider.Status), nil
}

func (r *Repository) convergePaymentDisputeTx(ctx context.Context, tx *sql.Tx, provider StripeDispute, desiredStatus PaymentDisputeStatus, now time.Time) (PaymentDispute, bool, error) {
	order, err := r.loadEligibleDisputeOrder(ctx, tx, provider)
	if err != nil {
		return PaymentDispute{}, false, err
	}
	existing, err := getPaymentDisputeInTx(ctx, tx, provider.ID)
	if err == nil {
		updated, changed, convergeErr := convergeExistingPaymentDispute(ctx, tx, existing, provider, desiredStatus, now)
		if convergeErr != nil {
			return PaymentDispute{}, false, convergeErr
		}
		updated.PublicOrderNumber = order.PublicNumber
		return updated, changed, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PaymentDispute{}, false, err
	}
	return createPaymentDisputeInTx(ctx, tx, order, provider, desiredStatus, now)
}

func (r *Repository) loadEligibleDisputeOrder(ctx context.Context, tx *sql.Tx, provider StripeDispute) (paymentRefundOrderFacts, error) {
	order, err := r.paymentRefundOrderForCharge(ctx, tx, provider.ChargeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return paymentRefundOrderFacts{}, ErrDisputeOrderNotEligible
		}
		return paymentRefundOrderFacts{}, err
	}
	if order.Status != PaymentOrderCredited || order.PaidTotal <= 0 || order.IdeaQuantity <= 0 || !strings.EqualFold(order.PaidCurrency, provider.Currency) {
		return paymentRefundOrderFacts{}, ErrDisputeOrderNotEligible
	}
	return order, nil
}

func createPaymentDisputeInTx(ctx context.Context, tx *sql.Tx, order paymentRefundOrderFacts, provider StripeDispute, desiredStatus PaymentDisputeStatus, now time.Time) (PaymentDispute, bool, error) {
	target, err := availablePaymentDisputeIdeas(ctx, tx, order, provider.AmountMinor)
	if err != nil {
		return PaymentDispute{}, false, err
	}
	dispute := newPaymentDispute(order, provider, desiredStatus, target, now)
	if desiredStatus != PaymentDisputeWon {
		if err := applyNewPaymentDisputeRecovery(ctx, tx, &dispute); err != nil {
			return PaymentDispute{}, false, err
		}
	}
	if err := insertPaymentDispute(ctx, tx, dispute, now); err != nil {
		return PaymentDispute{}, false, err
	}
	return dispute, true, nil
}

func availablePaymentDisputeIdeas(ctx context.Context, tx *sql.Tx, order paymentRefundOrderFacts, amountMinor int64) (int64, error) {
	target := proportionalDisputeIdeas(amountMinor, order.PaidTotal, order.IdeaQuantity)
	if target > order.IdeaQuantity {
		target = order.IdeaQuantity
	}
	var occupied int64
	if err := tx.QueryRowContext(ctx, `
SELECT
  COALESCE((SELECT SUM(idea_quantity) FROM payment_refunds WHERE payment_order_id = $1::uuid AND status IN ('pending', 'succeeded', 'anomaly')), 0)
  + COALESCE((SELECT SUM(target_idea_quantity) FROM payment_disputes WHERE payment_order_id = $1::uuid AND status IN ('open', 'lost')), 0)`, order.ID).Scan(&occupied); err != nil {
		return 0, fmt.Errorf("sum occupied payment Ideas: %w", err)
	}
	remaining := order.IdeaQuantity - occupied
	if remaining < 0 {
		remaining = 0
	}
	if target > remaining {
		target = remaining
	}
	return target, nil
}

func newPaymentDispute(order paymentRefundOrderFacts, provider StripeDispute, desiredStatus PaymentDisputeStatus, target int64, now time.Time) PaymentDispute {
	dispute := PaymentDispute{
		ID: uuid.NewString(), PaymentOrderID: order.ID, PublicOrderNumber: order.PublicNumber, UserID: order.UserID,
		StripeDisputeID: provider.ID, StripeChargeID: provider.ChargeID, StripePaymentIntentID: provider.PaymentIntentID,
		ProviderStatus: provider.Status, Status: desiredStatus, AmountMinor: provider.AmountMinor, Currency: provider.Currency,
		TargetIdeaQuantity: target, OpenedAt: provider.CreatedAt, NextReconciliationAt: now.Add(disputeReconciliationInterval),
		CreatedAt: now, UpdatedAt: now,
	}
	if dispute.OpenedAt.IsZero() {
		dispute.OpenedAt = now
	}
	if desiredStatus != PaymentDisputeOpen {
		closed := now
		dispute.ClosedAt = &closed
	}
	return dispute
}

func insertPaymentDispute(ctx context.Context, tx *sql.Tx, dispute PaymentDispute, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO payment_disputes (
  id, payment_order_id, user_id, stripe_dispute_id, stripe_charge_id, stripe_payment_intent_id,
  provider_status, status, amount_minor, currency, target_idea_quantity, debited_idea_quantity,
  unrecovered_credits, restriction_id, debit_transaction_id, restore_transaction_id,
  opened_at, closed_at, next_reconciliation_at, created_at, updated_at
) VALUES (
  $1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
  NULLIF($14, '')::uuid, NULLIF($15, ''), NULLIF($16, ''), $17, $18, $19, $20, $20
)`, dispute.ID, dispute.PaymentOrderID, dispute.UserID, dispute.StripeDisputeID, dispute.StripeChargeID,
		dispute.StripePaymentIntentID, dispute.ProviderStatus, dispute.Status, dispute.AmountMinor, dispute.Currency,
		dispute.TargetIdeaQuantity, dispute.DebitedIdeaQuantity, dispute.UnrecoveredCredits, dispute.RestrictionID,
		dispute.DebitTransactionID, dispute.RestoreTransactionID, dispute.OpenedAt, dispute.ClosedAt,
		dispute.NextReconciliationAt, now); err != nil {
		return fmt.Errorf("insert Payment Dispute: %w", err)
	}
	return nil
}

func applyNewPaymentDisputeRecovery(ctx context.Context, tx *sql.Tx, dispute *PaymentDispute) error {
	restrictions := credits.RestrictionsInSQLTx(tx)
	change, err := restrictions.RestrictWallet(ctx, credits.RestrictWalletCommand{
		UserID: dispute.UserID, Type: "payment_dispute", Source: "stripe_dispute",
		RelatedObject: credits.BusinessReference{Type: "payment_dispute", ID: dispute.ID},
		Reason:        "Stripe Dispute " + dispute.StripeDisputeID,
		Actor:         credits.Actor{Type: credits.ActorService, ID: "payments"},
	})
	if err != nil {
		return err
	}
	dispute.RestrictionID = change.RestrictionID
	if dispute.TargetIdeaQuantity == 0 {
		return nil
	}
	amount, err := credits.NewCreditAmount(dispute.TargetIdeaQuantity)
	if err != nil {
		return err
	}
	commands, err := credits.CommandsInSQLTx(tx)
	if err != nil {
		return err
	}
	result, err := commands.PaymentDisputeDebit(ctx, credits.PaymentDisputeDebitCommand{
		UserID: dispute.UserID, RequestedAmount: amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "dispute-debit-" + dispute.ID,
			Actor:          credits.Actor{Type: credits.ActorService, ID: "payments"}, Source: "stripe_dispute",
			Reason: "Stripe Dispute debit", BusinessReference: credits.BusinessReference{Type: "payment_dispute", ID: dispute.ID},
		},
	})
	if err != nil {
		return err
	}
	dispute.DebitedIdeaQuantity = result.Amount
	dispute.UnrecoveredCredits = result.UnrecoveredAmount
	dispute.DebitTransactionID = result.TransactionID
	return nil
}

func convergeExistingPaymentDispute(ctx context.Context, tx *sql.Tx, dispute PaymentDispute, provider StripeDispute, desired PaymentDisputeStatus, now time.Time) (PaymentDispute, bool, error) {
	if err := validateExistingPaymentDispute(dispute, provider, desired); err != nil {
		return PaymentDispute{}, false, err
	}
	changed := dispute.Status != desired || dispute.ProviderStatus != provider.Status
	if desired == PaymentDisputeWon && dispute.Status != PaymentDisputeWon {
		if err := recoverWonPaymentDispute(ctx, tx, &dispute); err != nil {
			return PaymentDispute{}, false, err
		}
		closed := now
		dispute.ClosedAt = &closed
		dispute.UnrecoveredCredits = 0
	}
	if desired == PaymentDisputeLost && dispute.Status == PaymentDisputeOpen {
		closed := now
		dispute.ClosedAt = &closed
	}
	dispute.Status = desired
	dispute.ProviderStatus = provider.Status
	dispute.UpdatedAt = now
	dispute.NextReconciliationAt = now.Add(disputeReconciliationInterval)
	_, err := tx.ExecContext(ctx, `
UPDATE payment_disputes
SET provider_status = $2, status = $3, restore_transaction_id = NULLIF($4, ''), closed_at = $5,
    unrecovered_credits = $6, next_reconciliation_at = $7, updated_at = $8
WHERE id = $1::uuid`, dispute.ID, dispute.ProviderStatus, dispute.Status, dispute.RestoreTransactionID,
		dispute.ClosedAt, dispute.UnrecoveredCredits, dispute.NextReconciliationAt, now)
	if err != nil {
		return PaymentDispute{}, false, err
	}
	return dispute, changed, nil
}

func validateExistingPaymentDispute(dispute PaymentDispute, provider StripeDispute, desired PaymentDisputeStatus) error {
	if dispute.StripeChargeID != provider.ChargeID || dispute.AmountMinor != provider.AmountMinor ||
		!strings.EqualFold(dispute.Currency, provider.Currency) ||
		(dispute.StripePaymentIntentID != "" && provider.PaymentIntentID != "" && dispute.StripePaymentIntentID != provider.PaymentIntentID) {
		return ErrDisputeProviderConflict
	}
	if dispute.Status == PaymentDisputeWon && desired != PaymentDisputeWon {
		return ErrDisputeTransition
	}
	return nil
}

func recoverWonPaymentDispute(ctx context.Context, tx *sql.Tx, dispute *PaymentDispute) error {
	if dispute.DebitedIdeaQuantity > 0 && dispute.RestoreTransactionID == "" {
		amount, err := credits.NewCreditAmount(dispute.DebitedIdeaQuantity)
		if err != nil {
			return err
		}
		commands, err := credits.CommandsInSQLTx(tx)
		if err != nil {
			return err
		}
		restored, err := commands.PaymentDisputeRestore(ctx, credits.PaymentDisputeRestoreCommand{
			UserID: dispute.UserID, OriginalTransactionID: dispute.DebitTransactionID, Amount: amount,
			Metadata: credits.CommandMetadata{
				IdempotencyKey: "dispute-restore-" + dispute.ID,
				Actor:          credits.Actor{Type: credits.ActorService, ID: "payments"}, Source: "stripe_dispute",
				Reason: "Stripe Dispute won", BusinessReference: credits.BusinessReference{Type: "payment_dispute", ID: dispute.ID},
			},
		})
		if err != nil {
			return err
		}
		dispute.RestoreTransactionID = restored.TransactionID
	}
	if dispute.RestrictionID == "" {
		return nil
	}
	_, err := credits.RestrictionsInSQLTx(tx).ReleaseWalletRestriction(ctx, credits.ReleaseWalletRestrictionCommand{
		RestrictionID: dispute.RestrictionID, Reason: "Stripe Dispute won",
		Actor: credits.Actor{Type: credits.ActorService, ID: "payments"},
	})
	return err
}

func getPaymentDisputeInTx(ctx context.Context, tx *sql.Tx, stripeDisputeID string) (PaymentDispute, error) {
	dispute, err := scanPaymentDispute(tx.QueryRowContext(ctx, `
SELECT id::text, payment_order_id::text, user_id, stripe_dispute_id, stripe_charge_id,
       stripe_payment_intent_id, provider_status, status, amount_minor, currency,
       target_idea_quantity, debited_idea_quantity, unrecovered_credits,
       restriction_id::text, debit_transaction_id, restore_transaction_id,
       opened_at, closed_at, next_reconciliation_at, created_at, updated_at
FROM payment_disputes WHERE stripe_dispute_id = $1 FOR UPDATE`, stripeDisputeID))
	if err != nil {
		return PaymentDispute{}, err
	}
	return dispute, nil
}

func (r *Repository) GetPaymentDisputeByStripeID(ctx context.Context, stripeDisputeID string) (PaymentDispute, error) {
	if r == nil || r.db == nil {
		return PaymentDispute{}, ErrDisputeUnavailable
	}
	tx, err := r.dbWithContext(ctx).transactionStarter.BeginTx(ctx, nil)
	if err != nil {
		return PaymentDispute{}, err
	}
	defer func() { _ = tx.Rollback() }()
	dispute, err := getPaymentDisputeInTx(ctx, tx, strings.TrimSpace(stripeDisputeID))
	if err != nil {
		return PaymentDispute{}, err
	}
	return dispute, tx.Commit()
}

func (r *Repository) ListDuePaymentDisputes(ctx context.Context, now time.Time, limit int) ([]PaymentDispute, error) {
	if r == nil || r.db == nil {
		return nil, ErrDisputeUnavailable
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
	result := make([]PaymentDispute, 0, limit)
	for rows.Next() {
		dispute, err := scanPaymentDispute(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, dispute)
	}
	return result, rows.Err()
}

func (r *Repository) ListPaymentDisputesForOrder(ctx context.Context, orderNumber string) ([]PaymentDispute, error) {
	if r == nil || r.db == nil {
		return nil, ErrDisputeUnavailable
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
	result := make([]PaymentDispute, 0)
	for rows.Next() {
		dispute, err := scanPaymentDispute(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, dispute)
	}
	return result, rows.Err()
}
