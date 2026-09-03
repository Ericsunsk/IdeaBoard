package ports

import (
	"context"
	"time"
)

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
