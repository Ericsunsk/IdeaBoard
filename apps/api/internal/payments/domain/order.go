package domain

import "time"

type PaymentOrderStatus string

const (
	PaymentOrderCreating            PaymentOrderStatus = "creating"
	PaymentOrderCheckoutOpen        PaymentOrderStatus = "checkout_open"
	PaymentOrderAsyncPaymentPending PaymentOrderStatus = "async_payment_pending"
	PaymentOrderPaidPendingCredit   PaymentOrderStatus = "paid_pending_credit"
	PaymentOrderCredited            PaymentOrderStatus = "credited"
	PaymentOrderCancelled           PaymentOrderStatus = "cancelled"
	PaymentOrderExpired             PaymentOrderStatus = "expired"
	PaymentOrderFailed              PaymentOrderStatus = "payment_failed"
)

type PaymentOrder struct {
	ID                      string
	PublicOrderNumber       string
	PublicToken             string
	UserID                  int64
	ClientIdempotencyKey    string
	RequestHash             string
	Status                  PaymentOrderStatus
	StripeCredentialID      string
	StripeAccountID         string
	StripeCustomerID        string
	StripeProductID         string
	StripeCheckoutSessionID string
	StripePaymentIntentID   string
	StripeChargeID          string
	IdeaQuantity            int64
	BaseCurrency            string
	BaseAmountMinor         int64
	PaidSubtotalMinor       *int64
	PaidTaxMinor            *int64
	PaidTotalMinor          *int64
	PaidCurrency            string
	TermsVersion            string
	RefundPolicyVersion     string
	ReturnBaseURL           string
	CheckoutURL             string
	ReceiptURL              string
	CreditTransactionID     string
	ExpiresAt               time.Time
	PaidAt                  *time.Time
	CreditedAt              *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type CreatePaymentOrderIntentInput struct {
	ID                   string
	PublicOrderNumber    string
	PublicToken          string
	UserID               int64
	ClientIdempotencyKey string
	RequestHash          string
	StripeCredentialID   string
	StripeAccountID      string
	StripeProductID      string
	IdeaQuantity         int64
	TermsVersion         string
	RefundPolicyVersion  string
	ReturnBaseURL        string
	ExpiresAt            time.Time
}

type BindPaymentOrderCheckoutInput struct {
	OrderID          string
	StripeCustomerID string
	StripeSessionID  string
	CheckoutURL      string
	ExpiresAt        time.Time
}

type PaidCheckoutFacts struct {
	StripeAccountID       string
	StripeSessionID       string
	ExpiresAt             time.Time
	PublicOrderNumber     string
	PaymentOrderID        string
	RequestHash           string
	StripeCustomerID      string
	StripePaymentIntentID string
	StripeChargeID        string
	IdeaQuantity          int64
	PaidSubtotalMinor     int64
	PaidTaxMinor          int64
	PaidTotalMinor        int64
	PaidCurrency          string
	ReceiptURL            string
	PaidAt                time.Time
}

type PaymentConvergenceResult struct {
	Order          PaymentOrder
	AvailableIdeas int64
	Replayed       bool
}
