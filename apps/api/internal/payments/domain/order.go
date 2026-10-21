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

// allowedPaymentOrderTransitions mirrors the durable guards in
// guard_payment_order_lifecycle (internal/db/schema/fragments/payments.sql):
// creating fans out to checkout_open/paid_pending_credit/expired/
// payment_failed; checkout_open fans out to async_payment_pending/
// paid_pending_credit/cancelled/expired/payment_failed;
// async_payment_pending converges to paid_pending_credit/expired/
// payment_failed; paid_pending_credit credits exactly once; terminal
// unpaid states (cancelled/expired/payment_failed) resurrect only to
// paid_pending_credit for late webhooks (RecordPaymentOrderPaid).
// credited is terminal. The type only documents the allowed
// transitions at compile time (see CanOrderTransition); SQL literals
// in internal/db/queries/payments.sql stay byte-identical.
var allowedPaymentOrderTransitions = map[PaymentOrderStatus]map[PaymentOrderStatus]struct{}{
	PaymentOrderCreating: {
		PaymentOrderCheckoutOpen:      {},
		PaymentOrderPaidPendingCredit: {},
		PaymentOrderExpired:           {},
		PaymentOrderFailed:            {},
	},
	PaymentOrderCheckoutOpen: {
		PaymentOrderAsyncPaymentPending: {},
		PaymentOrderPaidPendingCredit:   {},
		PaymentOrderCancelled:           {},
		PaymentOrderExpired:             {},
		PaymentOrderFailed:              {},
	},
	PaymentOrderAsyncPaymentPending: {
		PaymentOrderPaidPendingCredit: {},
		PaymentOrderExpired:           {},
		PaymentOrderFailed:            {},
	},
	PaymentOrderPaidPendingCredit: {
		PaymentOrderCredited: {},
	},
	PaymentOrderCancelled: {
		PaymentOrderPaidPendingCredit: {},
	},
	PaymentOrderExpired: {
		PaymentOrderPaidPendingCredit: {},
	},
	PaymentOrderFailed: {
		PaymentOrderPaidPendingCredit: {},
	},
}

// CanOrderTransition reports whether a from -> to flip is durably
// allowed. It is documentation, not a gate: every transition it
// reports true for was already possible through the SQL guards, and no
// previously possible transition reports false.
func CanOrderTransition(from, to PaymentOrderStatus) bool {
	nexts, ok := allowedPaymentOrderTransitions[from]
	if !ok {
		return false
	}
	_, ok = nexts[to]
	return ok
}

type PaymentOrder struct {
	ID                string
	PublicOrderNumber string
	PublicToken       string
	// Operator attribution: the real User who created the checkout.
	// Funds identity is BillingAccountID; the two are never conflated.
	UserID                  int64
	BillingAccountID        int64
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
	ID                string
	PublicOrderNumber string
	PublicToken       string
	// Operator attribution: the real User who created the checkout.
	UserID int64
	// Payer funds identity: the Billing Account that owns the unified
	// balance. Idempotency, unpaid-slot accounting, and settlement all
	// scope to this account, never to the operator.
	BillingAccountID     int64
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
