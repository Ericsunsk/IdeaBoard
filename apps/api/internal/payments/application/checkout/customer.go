package checkout

import (
	"time"

	paymentports "ideaboard/api/internal/payments/ports"
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
