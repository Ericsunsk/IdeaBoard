package refunds

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	postgres "ideaboard/api/internal/payments/infrastructure/postgres"
	paymentports "ideaboard/api/internal/payments/ports"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateExternalPaymentRefund links a provider refund discovered outside the
// Admin flow to the matching order, reserving available Ideas before settling.
func (s *Store) CreateExternalPaymentRefund(ctx context.Context, provider paymentports.StripeRefund, now time.Time) (paymentsdomain.PaymentRefund, error) {
	if s == nil || !s.Available() || provider.ID == "" || provider.ChargeID == "" || provider.AmountMinor <= 0 {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundOrderNotEligible
	}
	tx, owned, err := s.BeginDirectTx(ctx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	defer func() { postgres.RollbackDirectIfOwned(ctx, tx, owned) }()
	order, err := postgres.LoadPaymentOrderFactsForCharge(ctx, tx, provider.ChargeID)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	key := "stripe-dashboard-" + provider.ID
	if existing, found, err := s.findExistingExternalRefund(ctx, tx, order.ID, provider.ID, key); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	} else if found {
		return existing, postgres.CommitDirectIfOwned(ctx, tx, owned)
	}
	if err := validateExternalRefund(order, provider); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if pending, err := s.findPendingRefund(ctx, tx, order, provider); err == nil {
		if err := s.attachExternalProviderRefund(ctx, tx, pending, provider, now); err != nil {
			return paymentsdomain.PaymentRefund{}, err
		}
		if err := postgres.CommitDirectIfOwned(ctx, tx, owned); err != nil {
			return paymentsdomain.PaymentRefund{}, err
		}
		pending.StripeRefundID, pending.StripeStatus, pending.UpdatedAt, pending.PublicOrderNumber = provider.ID, provider.Status, now, order.PublicNumber
		return pending, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentRefund{}, err
	}
	refund, err := s.createExternalRefundFact(ctx, tx, order, provider, key, now)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := postgres.CommitDirectIfOwned(ctx, tx, owned); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return refund, nil
}

func (s *Store) findExistingExternalRefund(ctx context.Context, tx pgx.Tx, orderID, stripeRefundID, idempotencyKey string) (paymentsdomain.PaymentRefund, bool, error) {
	if linked, err := s.findRefundByStripeID(ctx, tx, stripeRefundID); err == nil {
		return linked, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	if existing, err := s.findRefundByIdempotency(ctx, tx, orderID, idempotencyKey); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	return paymentsdomain.PaymentRefund{}, false, nil
}

func (s *Store) attachExternalProviderRefund(ctx context.Context, tx pgx.Tx, pending paymentsdomain.PaymentRefund, provider paymentports.StripeRefund, now time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE payment_refunds SET stripe_refund_id = $2, stripe_status = $3, updated_at = $4 WHERE id = $1::uuid`, pending.ID, provider.ID, provider.Status, now)
	return err
}

func (s *Store) createExternalRefundFact(ctx context.Context, tx pgx.Tx, order postgres.PaymentOrderFacts, provider paymentports.StripeRefund, key string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	exposure, err := postgres.QueryPaymentOrderExposure(ctx, tx, order.ID)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	already, alreadyAmount := exposure.RefundedIdeas, exposure.RefundedAmount
	disputedIdeas, disputedAmount := exposure.DisputedIdeas, exposure.DisputedAmount
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
	// #920: Dashboard refunds reserve on the order's fixed payer account.
	// Member exits and management changes never move the obligation; a
	// missing payer fails closed instead of falling back to the operator.
	if order.BillingAccountID <= 0 {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("refund order without payer Billing Account")
	}
	accountID := order.BillingAccountID
	hold, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditAmount, err := domain.NewCreditAmount(qty)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	refundID := uuid.NewString()
	// Dashboard-initiated refunds reserve through the Payments-typed
	// PurchaseRefundHold, not the generic spending Hold: a spending
	// restriction on an active Wallet must not block legitimate refund
	// recovery, while frozen/tombstone lifecycles stay rejected by the
	// routine gate. Insufficient Ideas surface as ErrInsufficientCredits
	// so dispatch records a durable anomaly for manual recovery.
	holdResult, err := hold.PurchaseRefundHold(ctx, ports.PurchaseRefundHoldCommand{BillingAccountID: accountID, Amount: creditAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "refund-hold-" + refundID, Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund_hold", Reason: "Stripe Dashboard refund", BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: refundID},
	}})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'pending', $8, $9, $10, $11, NULL, $12, $13, $13)`, refundID, order.ID, order.UserID, key, qty, provider.AmountMinor, strings.ToLower(provider.Currency), provider.Status, provider.ID, provider.ChargeID, holdResult.TransactionID, "Stripe Dashboard refund", now); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return paymentsdomain.PaymentRefund{ID: refundID, PaymentOrderID: order.ID, PublicOrderNumber: order.PublicNumber, UserID: order.UserID, IdeaQuantity: qty, AmountMinor: provider.AmountMinor, Currency: strings.ToLower(provider.Currency), Status: paymentsdomain.PaymentRefundPending, StripeStatus: provider.Status, StripeRefundID: provider.ID, StripeChargeID: provider.ChargeID, CreditHoldTransactionID: holdResult.TransactionID, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) RecordPaymentRefundAnomaly(ctx context.Context, provider paymentports.StripeRefund, reason string, now time.Time) (int64, error) {
	tx, owned, err := s.BeginDirectTx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { postgres.RollbackDirectIfOwned(ctx, tx, owned) }()
	order, err := postgres.LoadPaymentOrderFactsForCharge(ctx, tx, provider.ChargeID)
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
	_, err = tx.Exec(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, actor_id, reason, error_summary, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'anomaly', $8, $9, $10, '', NULL, 'Stripe Dashboard refund', $11, $12, $12) ON CONFLICT DO NOTHING`, uuid.NewString(), order.ID, order.UserID, "stripe-dashboard-"+provider.ID, qty, provider.AmountMinor, strings.ToLower(provider.Currency), provider.Status, provider.ID, provider.ChargeID, postgres.BoundedText(reason, 500), now)
	if err != nil {
		return 0, err
	}
	if err := postgres.CommitDirectIfOwned(ctx, tx, owned); err != nil {
		return 0, err
	}
	return order.UserID, nil
}

// CreatePendingPaymentRefund reserves Ideas and records the provider intent in
// one SQL transaction. Stripe I/O happens only after this commit.
func (s *Store) CreatePendingPaymentRefund(ctx context.Context, input paymentrefund.CreatePendingPaymentRefundInput) (paymentsdomain.PaymentRefund, bool, error) {
	if !validPendingRefundRequest(s, input) {
		return paymentsdomain.PaymentRefund{}, false, paymentcontracts.ErrRefundAmountInvalid
	}
	// This reservation is the durable hand-off to Stripe. It must commit on
	// its own connection before CreatePaymentRefund performs provider I/O;
	// borrowing the Control guard transaction would leave the hold and phase
	// audit rollbackable while cash could already have moved.
	tx, owned, err := s.BeginCommittedRefundTx(ctx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, false, fmt.Errorf("begin refund transaction: %w", err)
	}
	defer func() { postgres.RollbackDirectIfOwned(ctx, tx, owned) }()
	order, paidSubtotal, paidTotal, paidCurrency, paidAt, err := s.loadPendingRefundOrder(ctx, tx, input.OrderNumber)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	if existing, replayed, err := findPendingRefundReplay(ctx, tx, order, input); err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	} else if replayed {
		return commitPendingRefundReplay(ctx, tx, owned, existing, order)
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
	refund, err := s.persistPendingRefund(ctx, tx, order, paidCurrency.String, input, amountMinor)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, false, err
	}
	if owned {
		if err := tx.Commit(ctx); err != nil {
			return paymentsdomain.PaymentRefund{}, false, fmt.Errorf("commit payment refund: %w", err)
		}
	}
	return refund, false, nil
}

func validPendingRefundRequest(s *Store, input paymentrefund.CreatePendingPaymentRefundInput) bool {
	return s != nil && s.Available() && input.ActorID > 0 && input.IdeaQuantity > 0
}

func commitPendingRefundReplay(ctx context.Context, tx pgx.Tx, owned bool, existing paymentsdomain.PaymentRefund, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentRefund, bool, error) {
	if err := postgres.CommitDirectIfOwned(ctx, tx, owned); err != nil {
		return paymentsdomain.PaymentRefund{}, false, fmt.Errorf("commit refund replay: %w", err)
	}
	existing.PublicOrderNumber = order.PublicOrderNumber
	existing.StripeChargeID = order.StripeChargeID
	return existing, true, nil
}

func (s *Store) persistPendingRefund(ctx context.Context, tx pgx.Tx, order paymentsdomain.PaymentOrder, currency string, input paymentrefund.CreatePendingPaymentRefundInput, amountMinor int64) (paymentsdomain.PaymentRefund, error) {
	refundID := uuid.NewString()
	// #920: Admin refunds reserve on the order's fixed payer account, never
	// the operator's personal account.
	if order.BillingAccountID <= 0 {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("refund order without payer Billing Account")
	}
	accountID := order.BillingAccountID
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditAmount, err := domain.NewCreditAmount(input.IdeaQuantity)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundAmountInvalid
	}
	hold, err := commands.PurchaseRefundHold(ctx, ports.PurchaseRefundHoldCommand{BillingAccountID: accountID, Amount: creditAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "refund-hold-" + refundID, Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund_hold", Reason: "Stripe refund hold",
		BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: refundID},
	}})
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	currency = strings.ToLower(currency)
	if _, err := tx.Exec(ctx, `INSERT INTO payment_refunds (id, payment_order_id, user_id, idempotency_key, idea_quantity, amount_minor, currency, status, stripe_charge_id, credit_hold_transaction_id, actor_id, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, 'pending', $8, $9, $10, $11, $11)`, refundID, order.ID, order.UserID, input.IdempotencyKey, input.IdeaQuantity, amountMinor, currency, order.StripeChargeID, hold.TransactionID, input.ActorID, input.Now); err != nil {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("insert payment refund: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, metadata) VALUES ('refund_payment', $1, 'payment_refund', $2, 'succeeded', jsonb_build_object('phase', 'reserved', 'payment_order_id', $3::uuid, 'idea_quantity', $4::bigint, 'amount_minor', $5::bigint, 'currency', $6::text))`, input.ActorID, refundID, order.ID, input.IdeaQuantity, amountMinor, currency); err != nil {
		return paymentsdomain.PaymentRefund{}, fmt.Errorf("record payment refund audit: %w", err)
	}
	return paymentsdomain.PaymentRefund{ID: refundID, PaymentOrderID: order.ID, PublicOrderNumber: order.PublicOrderNumber, UserID: order.UserID, IdeaQuantity: input.IdeaQuantity, AmountMinor: amountMinor, Currency: currency, Status: paymentsdomain.PaymentRefundPending, StripeChargeID: order.StripeChargeID, CreditHoldTransactionID: hold.TransactionID, CreatedAt: input.Now, UpdatedAt: input.Now}, nil
}

func (s *Store) MarkPaymentRefundProviderResult(ctx context.Context, refundID, stripeRefundID, stripeStatus string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	// Single auto-commit compare-and-set on the pending row. DBWithContext
	// only borrows an AdvisoryConnection, never the ambient Control guard
	// transaction, so the provider outcome lands on a fresh connection and
	// always observes the committed reservation above.
	var refund paymentsdomain.PaymentRefund
	err := s.DBWithContext(ctx).Executor().QueryRow(ctx, `UPDATE payment_refunds SET stripe_refund_id = CASE WHEN stripe_refund_id = '' THEN $2 ELSE stripe_refund_id END, stripe_status = $3, updated_at = $4 WHERE id = $1::uuid AND status = 'pending' AND (stripe_refund_id = '' OR stripe_refund_id = $2) RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, stripeRefundID, stripeStatus, now).Scan(&refund.ID, &refund.PaymentOrderID, &refund.UserID, &refund.IdeaQuantity, &refund.AmountMinor, &refund.Currency, &refund.Status, &refund.StripeStatus, &refund.StripeRefundID, &refund.StripeChargeID, &refund.CreditHoldTransactionID, &refund.CreditTransactionID, &refund.ErrorSummary, &refund.CreatedAt, &refund.UpdatedAt)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return paymentsdomain.PaymentRefund{}, fmt.Errorf("record Stripe refund: %w", err)
		}
		return s.GetPaymentRefundByID(ctx, refundID)
	}
	return refund, nil
}

func (s *Store) MarkPaymentRefundFailed(ctx context.Context, refundID, summary string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	// Settlement runs after provider I/O and therefore must not be attached to
	// a still-open Control guard transaction. Its own commit leaves a durable
	// result for recovery even when the later HTTP audit commit fails.
	tx, owned, err := s.BeginCommittedRefundTx(ctx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	defer func() { postgres.RollbackDirectIfOwned(ctx, tx, owned) }()
	var refund paymentsdomain.PaymentRefund
	if err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE id = $1::uuid FOR UPDATE`, refundID), &refund); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if refund.Status == paymentsdomain.PaymentRefundFailed || refund.Status == paymentsdomain.PaymentRefundAnomaly {
		return refund, postgres.CommitDirectIfOwned(ctx, tx, owned)
	}
	if refund.Status == paymentsdomain.PaymentRefundSucceeded {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundOrderNotEligible
	}
	// #920: release on the order's fixed payer account. The refund row keeps
	// the operator as UserID; funds never follow the operator.
	accountID, err := loadRefundOrderBillingAccount(ctx, tx, refund.PaymentOrderID)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditAmount, err := domain.NewCreditAmount(refund.IdeaQuantity)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if _, err := commands.Release(ctx, ports.ReleaseCommand{BillingAccountID: accountID, HoldTransactionID: refund.CreditHoldTransactionID, Amount: creditAmount, Metadata: domain.CommandMetadata{IdempotencyKey: "refund-release-" + refund.ID, Actor: domain.Actor{Type: domain.ActorService, ID: "payments"}, Source: "stripe_refund", Reason: "Stripe refund failed", BusinessReference: domain.BusinessReference{Type: "payment_refund", ID: refund.ID}}}); err != nil && !errors.Is(err, domain.ErrHoldSettled) {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := scanPaymentRefund(tx.QueryRow(ctx, `UPDATE payment_refunds SET status = 'failed', error_summary = $2, failed_at = $3, updated_at = $3 WHERE id = $1::uuid RETURNING id::text, payment_order_id::text, user_id, idea_quantity, amount_minor, currency, status, stripe_status, stripe_refund_id, stripe_charge_id, credit_hold_transaction_id, credit_transaction_id, error_summary, created_at, updated_at`, refundID, postgres.BoundedText(summary, 500), now), &refund); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, error_code, error_summary, metadata) SELECT 'refund_payment', actor_id, 'payment_refund', id::text, 'failed', 'provider_failed', $2, jsonb_build_object('phase', 'released') FROM payment_refunds WHERE id = $1::uuid AND actor_id IS NOT NULL`, refundID, postgres.BoundedText(summary, 500)); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if err := postgres.CommitDirectIfOwned(ctx, tx, owned); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return refund, nil
}

func (s *Store) MarkPaymentRefundSucceeded(ctx context.Context, refundID string, now time.Time) (paymentsdomain.PaymentRefund, error) {
	// See MarkPaymentRefundFailed: provider convergence has its own explicit
	// transaction boundary and never borrows the pre-response Control tx.
	tx, owned, err := s.BeginCommittedRefundTx(ctx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	defer func() { postgres.RollbackDirectIfOwned(ctx, tx, owned) }()
	var refund paymentsdomain.PaymentRefund
	if err := scanPaymentRefund(tx.QueryRow(ctx, paymentRefundSelect+` WHERE id = $1::uuid FOR UPDATE`, refundID), &refund); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	if refund.Status == paymentsdomain.PaymentRefundSucceeded || refund.Status == paymentsdomain.PaymentRefundAnomaly {
		return refund, postgres.CommitDirectIfOwned(ctx, tx, owned)
	}
	if refund.Status == paymentsdomain.PaymentRefundFailed {
		return paymentsdomain.PaymentRefund{}, paymentcontracts.ErrRefundOrderNotEligible
	}
	// #920: settle on the order's fixed payer account.
	accountID, err := loadRefundOrderBillingAccount(ctx, tx, refund.PaymentOrderID)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	commands, err := credits.CommandsInTx(tx)
	if err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	creditResult, err := commands.PurchaseRefund(ctx, ports.PurchaseRefundCommand{
		BillingAccountID: accountID, HoldTransactionID: refund.CreditHoldTransactionID,
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
	if err := postgres.CommitDirectIfOwned(ctx, tx, owned); err != nil {
		return paymentsdomain.PaymentRefund{}, err
	}
	return refund, nil
}

// merged from internal/payments/infrastructure/postgres/refunds/helpers.go (structural consolidation; symbols unchanged).
func validateExternalRefund(order postgres.PaymentOrderFacts, provider paymentports.StripeRefund) error {
	if order.BillingAccountID <= 0 {
		return paymentcontracts.ErrRefundOrderNotEligible
	}
	if order.Status != paymentsdomain.PaymentOrderCredited || order.PaidSubtotal <= 0 || order.PaidTotal <= 0 || order.IdeaQuantity <= 0 {
		return paymentcontracts.ErrRefundOrderNotEligible
	}
	if !strings.EqualFold(provider.Currency, order.PaidCurrency) {
		return paymentcontracts.ErrRefundProviderConflict
	}
	return nil
}

func validatePendingRefundEligibility(order paymentsdomain.PaymentOrder, paidSubtotal, paidTotal pgtype.Int8, paidAt pgtype.Timestamptz, now time.Time) error {
	if order.BillingAccountID <= 0 {
		return paymentcontracts.ErrRefundOrderNotEligible
	}
	if order.Status != paymentsdomain.PaymentOrderCredited || order.StripeChargeID == "" || order.IdeaQuantity <= 0 || !paidSubtotal.Valid || paidSubtotal.Int64 <= 0 || !paidTotal.Valid || paidTotal.Int64 <= 0 || !paidAt.Valid {
		return paymentcontracts.ErrRefundOrderNotEligible
	}
	if !now.Before(paidAt.Time.Add(paymentsdomain.RefundEligibilityWindow)) {
		return paymentcontracts.ErrRefundExpired
	}
	return nil
}

// loadRefundOrderBillingAccount resolves the fixed payer account for
// post-reservation settlement (#920). The refund row keeps the operator as
// UserID; funds converge on the order row, so member exits and management
// changes never move the obligation. A missing payer fails closed.
func loadRefundOrderBillingAccount(ctx context.Context, tx pgx.Tx, orderID string) (int64, error) {
	var billingAccountID pgtype.Int8
	if err := tx.QueryRow(ctx, `SELECT billing_account_id FROM payment_orders WHERE id = $1::uuid`, orderID).Scan(&billingAccountID); err != nil {
		return 0, fmt.Errorf("read refund order payer: %w", err)
	}
	if !billingAccountID.Valid || billingAccountID.Int64 <= 0 {
		return 0, fmt.Errorf("refund order without payer Billing Account")
	}
	return billingAccountID.Int64, nil
}

func scanPaymentRefund(row interface{ Scan(...any) error }, refund *paymentsdomain.PaymentRefund) error {
	return row.Scan(&refund.ID, &refund.PaymentOrderID, &refund.UserID, &refund.IdeaQuantity, &refund.AmountMinor, &refund.Currency, &refund.Status, &refund.StripeStatus, &refund.StripeRefundID, &refund.StripeChargeID, &refund.CreditHoldTransactionID, &refund.CreditTransactionID, &refund.ErrorSummary, &refund.CreatedAt, &refund.UpdatedAt)
}
