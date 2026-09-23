package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const disputeReconciliationInterval = time.Hour

func (r *Repository) ConvergePaymentDispute(ctx context.Context, provider paymentports.StripeDispute, now time.Time) (paymentsdomain.PaymentDispute, bool, error) {
	provider, desiredStatus, err := normalizePaymentDisputeProvider(r, provider)
	if err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	tx, err := r.dbWithContext(ctx).transactionStarter.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return paymentsdomain.PaymentDispute{}, false, fmt.Errorf("begin Dispute transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dispute, changed, err := r.convergePaymentDisputeTx(ctx, tx, provider, desiredStatus, now)
	if err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	return dispute, changed, nil
}

func normalizePaymentDisputeProvider(r *Repository, provider paymentports.StripeDispute) (paymentports.StripeDispute, paymentsdomain.PaymentDisputeStatus, error) {
	if r == nil || r.db == nil || strings.TrimSpace(provider.ID) == "" || strings.TrimSpace(provider.ChargeID) == "" || provider.AmountMinor <= 0 {
		return paymentports.StripeDispute{}, "", paymentcontracts.ErrDisputeOrderNotEligible
	}
	provider.ID = strings.TrimSpace(provider.ID)
	provider.ChargeID = strings.TrimSpace(provider.ChargeID)
	provider.PaymentIntentID = strings.TrimSpace(provider.PaymentIntentID)
	provider.Status = strings.ToLower(strings.TrimSpace(provider.Status))
	provider.Currency = strings.ToLower(strings.TrimSpace(provider.Currency))
	return provider, paymentsdomain.CanonicalPaymentDisputeStatus(provider.Status), nil
}

func (r *Repository) convergePaymentDisputeTx(ctx context.Context, tx pgx.Tx, provider paymentports.StripeDispute, desiredStatus paymentsdomain.PaymentDisputeStatus, now time.Time) (paymentsdomain.PaymentDispute, bool, error) {
	order, err := r.loadEligibleDisputeOrder(ctx, tx, provider)
	if err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	existing, err := getPaymentDisputeInTx(ctx, tx, provider.ID)
	if err == nil {
		updated, changed, convergeErr := convergeExistingPaymentDispute(ctx, tx, existing, provider, desiredStatus, now)
		if convergeErr != nil {
			return paymentsdomain.PaymentDispute{}, false, convergeErr
		}
		updated.PublicOrderNumber = order.PublicNumber
		return updated, changed, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	return createPaymentDisputeInTx(ctx, tx, order, provider, desiredStatus, now)
}

func (r *Repository) loadEligibleDisputeOrder(ctx context.Context, tx pgx.Tx, provider paymentports.StripeDispute) (paymentRefundOrderFacts, error) {
	order, err := r.paymentRefundOrderForCharge(ctx, tx, provider.ChargeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paymentRefundOrderFacts{}, paymentcontracts.ErrDisputeOrderNotEligible
		}
		return paymentRefundOrderFacts{}, err
	}
	if order.Status != paymentsdomain.PaymentOrderCredited || order.PaidTotal <= 0 || order.IdeaQuantity <= 0 || !strings.EqualFold(order.PaidCurrency, provider.Currency) {
		return paymentRefundOrderFacts{}, paymentcontracts.ErrDisputeOrderNotEligible
	}
	return order, nil
}

func createPaymentDisputeInTx(ctx context.Context, tx pgx.Tx, order paymentRefundOrderFacts, provider paymentports.StripeDispute, desiredStatus paymentsdomain.PaymentDisputeStatus, now time.Time) (paymentsdomain.PaymentDispute, bool, error) {
	target, err := availablePaymentDisputeIdeas(ctx, tx, order, provider.AmountMinor)
	if err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	dispute := newPaymentDispute(order, provider, desiredStatus, target, now)
	if desiredStatus != paymentsdomain.PaymentDisputeWon {
		if err := applyNewPaymentDisputeRecovery(ctx, tx, &dispute); err != nil {
			return paymentsdomain.PaymentDispute{}, false, err
		}
	}
	if err := insertPaymentDispute(ctx, tx, dispute, now); err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	return dispute, true, nil
}

func availablePaymentDisputeIdeas(ctx context.Context, tx pgx.Tx, order paymentRefundOrderFacts, amountMinor int64) (int64, error) {
	target := paymentsdomain.ProportionalDisputeIdeas(amountMinor, order.PaidTotal, order.IdeaQuantity)
	if target > order.IdeaQuantity {
		target = order.IdeaQuantity
	}
	var occupied int64
	if err := tx.QueryRow(ctx, `
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

func newPaymentDispute(order paymentRefundOrderFacts, provider paymentports.StripeDispute, desiredStatus paymentsdomain.PaymentDisputeStatus, target int64, now time.Time) paymentsdomain.PaymentDispute {
	dispute := paymentsdomain.PaymentDispute{
		ID: uuid.NewString(), PaymentOrderID: order.ID, PublicOrderNumber: order.PublicNumber, UserID: order.UserID,
		StripeDisputeID: provider.ID, StripeChargeID: provider.ChargeID, StripePaymentIntentID: provider.PaymentIntentID,
		ProviderStatus: provider.Status, Status: desiredStatus, AmountMinor: provider.AmountMinor, Currency: provider.Currency,
		TargetIdeaQuantity: target, OpenedAt: provider.CreatedAt, NextReconciliationAt: now.Add(disputeReconciliationInterval),
		CreatedAt: now, UpdatedAt: now,
	}
	if dispute.OpenedAt.IsZero() {
		dispute.OpenedAt = now
	}
	if desiredStatus != paymentsdomain.PaymentDisputeOpen {
		closed := now
		dispute.ClosedAt = &closed
	}
	return dispute
}

func insertPaymentDispute(ctx context.Context, tx pgx.Tx, dispute paymentsdomain.PaymentDispute, now time.Time) error {
	if _, err := tx.Exec(ctx, `
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

func applyNewPaymentDisputeRecovery(ctx context.Context, tx pgx.Tx, dispute *paymentsdomain.PaymentDispute) error {
	restrictions := credits.RestrictionsInTx(tx)
	change, err := restrictions.RestrictWallet(ctx, ports.RestrictWalletCommand{
		UserID: dispute.UserID, Type: "payment_dispute", Source: "stripe_dispute",
		RelatedObject: domain.BusinessReference{Type: "payment_dispute", ID: dispute.ID},
		Reason:        "Stripe Dispute " + dispute.StripeDisputeID,
		Actor:         domain.Actor{Type: domain.ActorService, ID: "payments"},
	})
	if err != nil {
		return err
	}
	dispute.RestrictionID = change.RestrictionID
	if dispute.TargetIdeaQuantity == 0 {
		return nil
	}
	amount, err := domain.NewCreditAmount(dispute.TargetIdeaQuantity)
	if err != nil {
		return err
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return err
	}
	result, err := commands.PaymentDisputeDebit(ctx, ports.PaymentDisputeDebitCommand{
		UserID: dispute.UserID, RequestedAmount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "dispute-debit-" + dispute.ID,
			Actor:          domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_dispute",
			Reason: "Stripe Dispute debit", BusinessReference: domain.BusinessReference{Type: "payment_dispute", ID: dispute.ID},
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

func convergeExistingPaymentDispute(ctx context.Context, tx pgx.Tx, dispute paymentsdomain.PaymentDispute, provider paymentports.StripeDispute, desired paymentsdomain.PaymentDisputeStatus, now time.Time) (paymentsdomain.PaymentDispute, bool, error) {
	if err := validateExistingPaymentDispute(dispute, provider, desired); err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	changed := dispute.Status != desired || dispute.ProviderStatus != provider.Status
	if desired == paymentsdomain.PaymentDisputeWon && dispute.Status != paymentsdomain.PaymentDisputeWon {
		if err := recoverWonPaymentDispute(ctx, tx, &dispute); err != nil {
			return paymentsdomain.PaymentDispute{}, false, err
		}
		closed := now
		dispute.ClosedAt = &closed
		dispute.UnrecoveredCredits = 0
	}
	if desired == paymentsdomain.PaymentDisputeLost && dispute.Status == paymentsdomain.PaymentDisputeOpen {
		closed := now
		dispute.ClosedAt = &closed
	}
	dispute.Status = desired
	dispute.ProviderStatus = provider.Status
	dispute.UpdatedAt = now
	dispute.NextReconciliationAt = now.Add(disputeReconciliationInterval)
	_, err := tx.Exec(ctx, `
UPDATE payment_disputes
SET provider_status = $2, status = $3, restore_transaction_id = NULLIF($4, ''), closed_at = $5,
    unrecovered_credits = $6, next_reconciliation_at = $7, updated_at = $8
WHERE id = $1::uuid`, dispute.ID, dispute.ProviderStatus, dispute.Status, dispute.RestoreTransactionID,
		dispute.ClosedAt, dispute.UnrecoveredCredits, dispute.NextReconciliationAt, now)
	if err != nil {
		return paymentsdomain.PaymentDispute{}, false, err
	}
	return dispute, changed, nil
}

func validateExistingPaymentDispute(dispute paymentsdomain.PaymentDispute, provider paymentports.StripeDispute, desired paymentsdomain.PaymentDisputeStatus) error {
	if dispute.StripeChargeID != provider.ChargeID || dispute.AmountMinor != provider.AmountMinor ||
		!strings.EqualFold(dispute.Currency, provider.Currency) ||
		(dispute.StripePaymentIntentID != "" && provider.PaymentIntentID != "" && dispute.StripePaymentIntentID != provider.PaymentIntentID) {
		return paymentcontracts.ErrDisputeProviderConflict
	}
	if dispute.Status == paymentsdomain.PaymentDisputeWon && desired != paymentsdomain.PaymentDisputeWon {
		return paymentcontracts.ErrDisputeTransition
	}
	return nil
}

func recoverWonPaymentDispute(ctx context.Context, tx pgx.Tx, dispute *paymentsdomain.PaymentDispute) error {
	if dispute.DebitedIdeaQuantity > 0 && dispute.RestoreTransactionID == "" {
		amount, err := domain.NewCreditAmount(dispute.DebitedIdeaQuantity)
		if err != nil {
			return err
		}
		commands, err := credits.CommandsInTx(tx)
		if err != nil {
			return err
		}
		restored, err := commands.PaymentDisputeRestore(ctx, ports.PaymentDisputeRestoreCommand{
			UserID: dispute.UserID, OriginalTransactionID: dispute.DebitTransactionID, Amount: amount,
			Metadata: domain.CommandMetadata{
				IdempotencyKey: "dispute-restore-" + dispute.ID,
				Actor:          domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_dispute",
				Reason: "Stripe Dispute won", BusinessReference: domain.BusinessReference{Type: "payment_dispute", ID: dispute.ID},
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
	_, err := credits.RestrictionsInTx(tx).ReleaseWalletRestriction(ctx, ports.ReleaseWalletRestrictionCommand{
		RestrictionID: dispute.RestrictionID, Reason: "Stripe Dispute won",
		Actor: domain.Actor{Type: domain.ActorService, ID: "payments"},
	})
	return err
}

func getPaymentDisputeInTx(ctx context.Context, tx pgx.Tx, stripeDisputeID string) (paymentsdomain.PaymentDispute, error) {
	dispute, err := scanPaymentDispute(tx.QueryRow(ctx, `
SELECT id::text, payment_order_id::text, user_id, stripe_dispute_id, stripe_charge_id,
       stripe_payment_intent_id, provider_status, status, amount_minor, currency,
       target_idea_quantity, debited_idea_quantity, unrecovered_credits,
       restriction_id::text, debit_transaction_id, restore_transaction_id,
       opened_at, closed_at, next_reconciliation_at, created_at, updated_at
FROM payment_disputes WHERE stripe_dispute_id = $1 FOR UPDATE`, stripeDisputeID))
	if err != nil {
		return paymentsdomain.PaymentDispute{}, err
	}
	return dispute, nil
}

func (r *Repository) GetPaymentDisputeByStripeID(ctx context.Context, stripeDisputeID string) (paymentsdomain.PaymentDispute, error) {
	if r == nil || r.db == nil {
		return paymentsdomain.PaymentDispute{}, paymentcontracts.ErrDisputeUnavailable
	}
	tx, err := r.dbWithContext(ctx).transactionStarter.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return paymentsdomain.PaymentDispute{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	dispute, err := getPaymentDisputeInTx(ctx, tx, strings.TrimSpace(stripeDisputeID))
	if err != nil {
		return paymentsdomain.PaymentDispute{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return paymentsdomain.PaymentDispute{}, err
	}
	return dispute, nil
}
