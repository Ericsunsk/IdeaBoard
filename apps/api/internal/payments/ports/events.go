package ports

// PaymentEvent is the minimal user-facing event emitted by the Payments
// application. Keeping this DTO in ports prevents event consumers and
// infrastructure adapters from depending on the Payments composition package.
type PaymentEvent struct {
	PublicToken string `json:"public_token,omitempty"`
	Status      string `json:"status"`
}

// PaymentEventPublisher is the outbound event seam used by application
// workflows.
type PaymentEventPublisher interface {
	Publish(userID int64, event PaymentEvent)
}
