package payments

import (
	"context"
	"time"
)

// CheckoutGateway is the only seam through which Payments creates and verifies
// Stripe Checkout purchases. It keeps Stripe SDK values out of the domain.
type CheckoutGateway interface {
	CreateCustomer(ctx context.Context, input CreateCheckoutCustomerInput) (CheckoutCustomer, error)
	CreateCheckoutSession(ctx context.Context, input CreateCheckoutSessionInput) (CheckoutSession, error)
	RetrieveCheckoutSession(ctx context.Context, input RetrieveCheckoutSessionInput) (CheckoutSession, error)
	ExpireCheckoutSession(ctx context.Context, input ExpireCheckoutSessionInput) (ExpireCheckoutSessionResult, error)
}

type CreateCheckoutCustomerInput struct {
	APIKey         string
	UserID         int64
	IdempotencyKey string
}

type CheckoutCustomer struct {
	ID string
}

type CreateCheckoutSessionInput struct {
	APIKey            string
	CustomerID        string
	ProductID         string
	PublicOrderNumber string
	PaymentOrderID    string
	RequestHash       string
	IdeaQuantity      int64
	AmountMinor       int64
	ExpiresAt         time.Time
	SuccessURL        string
	CancelURL         string
	IdempotencyKey    string
}

type RetrieveCheckoutSessionInput struct {
	APIKey    string
	SessionID string
}

type ExpireCheckoutSessionInput struct {
	APIKey    string
	SessionID string
}

// CheckoutSessionExpirationOutcome tells the domain which canonical Stripe
// fact won an expiration race. The gateway reports facts only; the caller owns
// all order and Credit state transitions.
type CheckoutSessionExpirationOutcome string

const (
	CheckoutSessionExpirationOutcomeExpired             CheckoutSessionExpirationOutcome = "expired"
	CheckoutSessionExpirationOutcomePaid                CheckoutSessionExpirationOutcome = "paid"
	CheckoutSessionExpirationOutcomeAsyncPaymentPending CheckoutSessionExpirationOutcome = "async_payment_pending"
	CheckoutSessionExpirationOutcomeOpen                CheckoutSessionExpirationOutcome = "open"
)

type ExpireCheckoutSessionResult struct {
	Outcome CheckoutSessionExpirationOutcome
	Session CheckoutSession
}

type CheckoutPaymentStatus string

const (
	CheckoutPaymentStatusPaid   CheckoutPaymentStatus = "paid"
	CheckoutPaymentStatusUnpaid CheckoutPaymentStatus = "unpaid"
)

type CheckoutSessionStatus string

const (
	CheckoutSessionStatusOpen     CheckoutSessionStatus = "open"
	CheckoutSessionStatusComplete CheckoutSessionStatus = "complete"
	CheckoutSessionStatusExpired  CheckoutSessionStatus = "expired"
)

// CheckoutSession is a normalized Stripe fact. A browser return never becomes
// a payment fact. Any non-paid Stripe status is normalized to unpaid so callers
// fail closed before a Credit purchase is settled.
type CheckoutSession struct {
	ID                  string
	URL                 string
	CustomerID          string
	PublicOrderNumber   string
	PaymentOrderID      string
	RequestHash         string
	IdeaQuantity        int64
	PaymentIntentID     string
	ChargeID            string
	ReceiptURL          string
	Status              CheckoutSessionStatus
	PaymentStatus       CheckoutPaymentStatus
	AmountSubtotalMinor int64
	AmountTaxMinor      int64
	AmountTotalMinor    int64
	Currency            string
	ExpiresAt           time.Time
}

func (s CheckoutSession) IsPaid() bool {
	return s.PaymentStatus == CheckoutPaymentStatusPaid
}
