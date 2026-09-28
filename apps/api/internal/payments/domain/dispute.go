package domain

import (
	"math/big"
	"strings"
	"time"
)

type PaymentDisputeStatus string

const (
	PaymentDisputeOpen PaymentDisputeStatus = "open"
	PaymentDisputeWon  PaymentDisputeStatus = "won"
	PaymentDisputeLost PaymentDisputeStatus = "lost"
)

// allowedPaymentDisputeTransitions mirrors the durable guards in
// guard_payment_dispute_lifecycle
// (internal/db/schema/fragments/integrity.sql): open closes to
// won/lost; lost reopens only to won (provider reversal); won is
// terminal. The type only documents the allowed transitions at
// compile time (see CanDisputeTransition); SQL literals stay
// byte-identical.
var allowedPaymentDisputeTransitions = map[PaymentDisputeStatus]map[PaymentDisputeStatus]struct{}{
	PaymentDisputeOpen: {
		PaymentDisputeWon:  {},
		PaymentDisputeLost: {},
	},
	PaymentDisputeLost: {
		PaymentDisputeWon: {},
	},
}

// CanDisputeTransition reports whether a from -> to flip is durably
// allowed. It is documentation, not a gate: every transition it
// reports true for was already possible through the SQL guards, and no
// previously possible transition reports false.
func CanDisputeTransition(from, to PaymentDisputeStatus) bool {
	nexts, ok := allowedPaymentDisputeTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

type PaymentDispute struct {
	ID                    string
	PaymentOrderID        string
	PublicOrderNumber     string
	UserID                int64
	StripeDisputeID       string
	StripeChargeID        string
	StripePaymentIntentID string
	ProviderStatus        string
	Status                PaymentDisputeStatus
	AmountMinor           int64
	Currency              string
	TargetIdeaQuantity    int64
	DebitedIdeaQuantity   int64
	UnrecoveredCredits    int64
	RestrictionID         string
	DebitTransactionID    string
	RestoreTransactionID  string
	OpenedAt              time.Time
	ClosedAt              *time.Time
	NextReconciliationAt  time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func CanonicalPaymentDisputeStatus(providerStatus string) PaymentDisputeStatus {
	switch strings.ToLower(strings.TrimSpace(providerStatus)) {
	case "won", "warning_closed":
		return PaymentDisputeWon
	case "lost":
		return PaymentDisputeLost
	default:
		return PaymentDisputeOpen
	}
}

// ProportionalDisputeIdeas rounds up the dispute's proportional Idea quantity
// so a partial dispute cannot leave a fractional obligation unaccounted for.
func ProportionalDisputeIdeas(amountMinor, paidTotal, totalIdeas int64) int64 {
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
