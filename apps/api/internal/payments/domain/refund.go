package domain

import (
	"math/big"
	"time"
)

type PaymentRefundStatus string

// RefundEligibilityWindow is the maximum age of a credited order eligible for refund.
const RefundEligibilityWindow = 14 * 24 * time.Hour

const (
	PaymentRefundPending   PaymentRefundStatus = "pending"
	PaymentRefundSucceeded PaymentRefundStatus = "succeeded"
	PaymentRefundFailed    PaymentRefundStatus = "failed"
	PaymentRefundAnomaly   PaymentRefundStatus = "anomaly"
)

// allowedPaymentRefundTransitions mirrors the durable guards in
// enforce_payment_refund_transition
// (internal/db/schema/fragments/payments.sql): pending advances to
// succeeded/failed (pending -> pending replays converge at the service
// layer without a database flip, matching the workspacetasks.Cancel
// precedent). Terminal states (succeeded/failed/anomaly) have no
// outgoing transitions. anomaly is insert-only (dashboard refunds that
// need manual recovery): it is never a transition target from pending
// via UPDATE, so it has no incoming edge here. The type only
// documents the allowed transitions at compile time (see
// CanRefundTransition); SQL literals stay byte-identical.
var allowedPaymentRefundTransitions = map[PaymentRefundStatus]map[PaymentRefundStatus]struct{}{
	PaymentRefundPending: {
		PaymentRefundPending:   {},
		PaymentRefundSucceeded: {},
		PaymentRefundFailed:    {},
	},
}

// CanRefundTransition reports whether a from -> to flip is durably
// allowed. It is documentation, not a gate: every transition it
// reports true for was already possible through the SQL guards, and no
// previously possible transition reports false.
func CanRefundTransition(from, to PaymentRefundStatus) bool {
	nexts, ok := allowedPaymentRefundTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

type PaymentRefund struct {
	ID                      string              `json:"id"`
	PaymentOrderID          string              `json:"payment_order_id"`
	PublicOrderNumber       string              `json:"public_order_number"`
	UserID                  int64               `json:"user_id"`
	IdeaQuantity            int64               `json:"idea_quantity"`
	AmountMinor             int64               `json:"amount_minor"`
	Currency                string              `json:"currency"`
	Status                  PaymentRefundStatus `json:"status"`
	StripeStatus            string              `json:"stripe_status,omitempty"`
	StripeChargeID          string              `json:"-"`
	StripeRefundID          string              `json:"stripe_refund_id,omitempty"`
	CreditHoldTransactionID string              `json:"credit_hold_transaction_id,omitempty"`
	CreditTransactionID     string              `json:"credit_transaction_id,omitempty"`
	ErrorSummary            string              `json:"error_summary,omitempty"`
	CreatedAt               time.Time           `json:"created_at"`
	UpdatedAt               time.Time           `json:"updated_at"`
}

// ProportionalRefundIdeas converts a provider refund amount to the original
// Idea quantity using integer arithmetic, preserving cent-level boundaries.
func ProportionalRefundIdeas(amountMinor, subtotal, totalIdeas int64) int64 {
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

func ProportionalRefundAmount(subtotal, totalIdeas, alreadyRefunded, newIdeas int64) int64 {
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
