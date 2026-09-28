package ports

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
type PaymentEvent struct {
	PublicToken string             `json:"public_token,omitempty"`
	Status      PaymentEventStatus `json:"status"`
}

// PaymentEventPublisher is the outbound event seam used by application
// workflows.
type PaymentEventPublisher interface {
	Publish(userID int64, event PaymentEvent)
}
