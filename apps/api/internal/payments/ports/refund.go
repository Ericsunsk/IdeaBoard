package ports

import "context"

type CreateStripeRefundInput struct {
	APIKey         string
	ChargeID       string
	AmountMinor    int64
	Currency       string
	IdempotencyKey string
}

type StripeRefund struct {
	ID          string
	Status      string
	ChargeID    string
	AmountMinor int64
	Currency    string
}

type RefundGateway interface {
	CreateRefund(context.Context, CreateStripeRefundInput) (StripeRefund, error)
	RetrieveRefund(context.Context, string, string) (StripeRefund, error)
}
