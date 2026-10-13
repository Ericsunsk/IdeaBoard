package contracts

// CreatePaymentRefundCommand describes an administrator-requested refund.
type CreatePaymentRefundCommand struct {
	ActorID        int64
	OrderNumber    string
	IdeaQuantity   int64
	IdempotencyKey string
}
