package ports

import (
	"context"
	"time"
)

// CheckoutGateway is the provider boundary for normalized Checkout facts.
type CheckoutGateway interface {
	CreateCustomer(context.Context, CreateCheckoutCustomerInput) (CheckoutCustomer, error)
	CreateCheckoutSession(context.Context, CreateCheckoutSessionInput) (CheckoutSession, error)
	RetrieveCheckoutSession(context.Context, RetrieveCheckoutSessionInput) (CheckoutSession, error)
	ExpireCheckoutSession(context.Context, ExpireCheckoutSessionInput) (ExpireCheckoutSessionResult, error)
}

type CreateCheckoutCustomerInput struct {
	APIKey string
	// Payer funds identity: the Billing Account the Stripe Customer is
	// reused for. The operator never scopes the customer.
	BillingAccountID int64
	IdempotencyKey   string
}

type CheckoutCustomer struct{ ID string }

type CreateCheckoutSessionInput struct {
	APIKey                       string
	PaymentMethodConfigurationID string
	CustomerID                   string
	ProductID                    string
	PublicOrderNumber            string
	PaymentOrderID               string
	RequestHash                  string
	IdeaQuantity                 int64
	AmountMinor                  int64
	ExpiresAt                    time.Time
	SuccessURL                   string
	CancelURL                    string
	IdempotencyKey               string
}

type RetrieveCheckoutSessionInput struct{ APIKey, SessionID string }
type ExpireCheckoutSessionInput struct{ APIKey, SessionID string }

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

func (s CheckoutSession) IsPaid() bool { return s.PaymentStatus == CheckoutPaymentStatusPaid }
