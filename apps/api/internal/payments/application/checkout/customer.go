package checkout

import (
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentports "ideaboard/api/internal/payments/ports"
	"time"
)

// PaymentCustomer is the Stripe customer identity used by Checkout. It is
// owned by the Checkout application boundary rather than the root composition
// package, while persistence adapters remain free to map it to SQL rows.
type PaymentCustomer struct {
	UserID           int64
	StripeAccountID  string
	StripeMode       paymentports.StripeMode
	StripeCustomerID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// merged from internal/payments/application/checkout/shutdown.go (structural consolidation; symbols unchanged).
// PaymentCheckoutShutdown is owned by configuration publication while
// checkout consumes it to expire stale sessions.
type PaymentCheckoutShutdown = paymentconfiguration.PaymentCheckoutShutdown
