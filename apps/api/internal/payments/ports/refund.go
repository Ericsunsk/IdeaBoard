package ports

import (
	"context"
	"time"
)

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

// StripeRefundStatus is the closed Stripe refund status vocabulary
// consumed at the application boundary (Converge/convergeRefund). The
// wire words are unchanged ("succeeded","failed","canceled") and mirror
// the Stripe API; the type only documents the allowed words at compile
// time. StripeRefund.Status stays string: the Stripe SDK payload is the
// boundary input and is converted at each switch site.
type StripeRefundStatus string

const (
	StripeRefundSucceeded StripeRefundStatus = "succeeded"
	StripeRefundFailed    StripeRefundStatus = "failed"
	StripeRefundCanceled  StripeRefundStatus = "canceled"
)

type RefundGateway interface {
	CreateRefund(context.Context, CreateStripeRefundInput) (StripeRefund, error)
	RetrieveRefund(context.Context, string, string) (StripeRefund, error)
}

// merged from internal/payments/ports/dispute.go (structural consolidation; symbols unchanged).
type StripeDispute struct {
	ID              string
	Status          string
	ChargeID        string
	PaymentIntentID string
	AmountMinor     int64
	Currency        string
	CreatedAt       time.Time
}

type DisputeGateway interface {
	RetrieveDispute(context.Context, string, string) (StripeDispute, error)
}
