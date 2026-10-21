package ports

import "context"

// PaymentEventStatus is the user-facing top-up hint vocabulary carried
// by PaymentEvent. The wire words are unchanged ("processing",
// "credited", "failed", "refund_updated", "dispute_updated"); the type
// only documents the closed vocabulary at compile time. Hints are
// post-commit SSE notifications, not a durable lifecycle: there is no
// transition table, and PostgreSQL remains the payment fact source.
type PaymentEventStatus string

const (
	PaymentEventProcessing     PaymentEventStatus = "processing"
	PaymentEventCredited       PaymentEventStatus = "credited"
	PaymentEventFailed         PaymentEventStatus = "failed"
	PaymentEventRefundUpdated  PaymentEventStatus = "refund_updated"
	PaymentEventDisputeUpdated PaymentEventStatus = "dispute_updated"
)

// PaymentEvent is the minimal user-facing event emitted by the Payments
// application. Keeping this DTO in ports prevents event consumers and
// infrastructure adapters from depending on the Payments composition package.
// BillingAccountID scopes the hint to the payer account (#919): Payments
// delivers it only to the account's current financial managers, and consumers
// invalidate only the matching account's queries. Hints are post-commit
// notifications, never a payment fact source.
type PaymentEvent struct {
	PublicToken      string             `json:"public_token,omitempty"`
	Status           PaymentEventStatus `json:"status"`
	BillingAccountID int64              `json:"billing_account_id,omitempty"`
}

// PaymentEventPublisher is the per-user transport seam used by the Payments
// hint publisher. Workflows use PaymentHintPublisher instead.
type PaymentEventPublisher interface {
	Publish(userID int64, event PaymentEvent)
}

// PaymentHintPublisher delivers a committed hint to the persisted payer's
// current financial managers, with operator-only fallback when unavailable.
// Callers own the payment facts and post-commit timing, never the audience.
type PaymentHintPublisher interface {
	Publish(ctx context.Context, operatorID int64, event PaymentEvent)
}
