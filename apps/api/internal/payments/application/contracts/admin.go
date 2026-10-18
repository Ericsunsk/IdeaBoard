package contracts

import (
	"context"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"time"

	"github.com/google/uuid"
)

// AdminService is the application boundary used by the administrative HTTP
// adapter. Implementations own validation, transaction boundaries, and
// provider convergence; this contract contains only public inputs and DTOs.
type AdminService interface {
	GetStripeConfiguration(context.Context) (StripeConfigurationSummary, error)
	ConfigureStripe(context.Context, ConfigureStripeCommand) (StripeConfigurationSummary, error)
	ListAdminPaymentOrders(context.Context, AdminPaymentOrderListInput) (AdminPaymentOrderPage, error)
	GetAdminPaymentOrder(context.Context, string) (AdminPaymentOrderDetail, error)
	CreatePaymentRefund(context.Context, CreatePaymentRefundCommand) (paymentsdomain.PaymentRefund, error)
}

type AdminPaymentOrderListInput struct {
	OrderNumber string
	User        string
	Status      paymentsdomain.PaymentOrderStatus
	AnomalyOnly bool
	Cursor      string
	Limit       int
}

type AdminPaymentOrderPage struct {
	Orders     []AdminPaymentOrderSummary `json:"orders"`
	Total      int64                      `json:"total"`
	NextCursor string                     `json:"next_cursor,omitempty"`
}

type AdminPaymentOrderUser struct {
	PublicID    uuid.UUID `json:"public_id"`
	DisplayName string    `json:"display_name"`
}

type AdminPaymentOrderSummary struct {
	PublicOrderNumber string                            `json:"public_order_number"`
	User              AdminPaymentOrderUser             `json:"user"`
	IdeaQuantity      int64                             `json:"idea_quantity"`
	BaseAmountMinor   int64                             `json:"base_amount_minor"`
	BaseCurrency      string                            `json:"base_currency"`
	PaidAmountMinor   *int64                            `json:"paid_amount_minor,omitempty"`
	PaidCurrency      string                            `json:"paid_currency,omitempty"`
	Status            paymentsdomain.PaymentOrderStatus `json:"status"`
	StripeStatus      string                            `json:"stripe_status"`
	CreditStatus      string                            `json:"credit_status"`
	CreatedAt         time.Time                         `json:"created_at"`
	PaidAt            *time.Time                        `json:"paid_at,omitempty"`
	CreditedAt        *time.Time                        `json:"credited_at,omitempty"`
	LastCheckedAt     *time.Time                        `json:"last_checked_at,omitempty"`
	AnomalousAt       *time.Time                        `json:"anomalous_at,omitempty"`
}

type AdminPaymentOrderDetail struct {
	PublicOrderNumber      string                            `json:"public_order_number"`
	User                   AdminPaymentOrderUser             `json:"user"`
	IdeaQuantity           int64                             `json:"idea_quantity"`
	BaseAmountMinor        int64                             `json:"base_amount_minor"`
	BaseCurrency           string                            `json:"base_currency"`
	PaidSubtotalMinor      *int64                            `json:"paid_subtotal_minor,omitempty"`
	PaidTaxMinor           *int64                            `json:"paid_tax_minor,omitempty"`
	PaidTotalMinor         *int64                            `json:"paid_total_minor,omitempty"`
	PaidCurrency           string                            `json:"paid_currency,omitempty"`
	Status                 paymentsdomain.PaymentOrderStatus `json:"status"`
	StripeStatus           string                            `json:"stripe_status"`
	CreditStatus           string                            `json:"credit_status"`
	Stripe                 AdminPaymentStripeSummary         `json:"stripe"`
	TermsVersion           string                            `json:"terms_version"`
	RefundPolicyVersion    string                            `json:"refund_policy_version"`
	ReceiptURL             string                            `json:"receipt_url,omitempty"`
	ExpiresAt              time.Time                         `json:"expires_at"`
	CreatedAt              time.Time                         `json:"created_at"`
	UpdatedAt              time.Time                         `json:"updated_at"`
	PaidAt                 *time.Time                        `json:"paid_at,omitempty"`
	CreditedAt             *time.Time                        `json:"credited_at,omitempty"`
	Reconciliation         AdminPaymentReconciliation        `json:"reconciliation"`
	Refunds                []AdminPaymentRefundSummary       `json:"refunds"`
	Disputes               []AdminPaymentDisputeSummary      `json:"disputes"`
	RefundableIdeaQuantity int64                             `json:"refundable_idea_quantity"`
}

type AdminPaymentStripeSummary struct {
	AccountID         string `json:"account_id,omitempty"`
	CustomerID        string `json:"customer_id,omitempty"`
	CheckoutSessionID string `json:"checkout_session_id,omitempty"`
	PaymentIntentID   string `json:"payment_intent_id,omitempty"`
	ChargeID          string `json:"charge_id,omitempty"`
}

type AdminPaymentReconciliation struct {
	Anomalous        bool       `json:"anomalous"`
	AnomalousAt      *time.Time `json:"anomalous_at,omitempty"`
	LastCheckedAt    *time.Time `json:"last_checked_at,omitempty"`
	NextAttemptAt    *time.Time `json:"next_attempt_at,omitempty"`
	LastErrorCode    string     `json:"last_error_code,omitempty"`
	LastErrorSummary string     `json:"last_error_summary,omitempty"`
}

// AdminPaymentRefundSummary exposes durable Refund state without provider
// payloads, internal ledger identities, or payer PII.
type AdminPaymentRefundSummary struct {
	ID           string     `json:"id,omitempty"`
	Status       string     `json:"status"`
	IdeaQuantity int64      `json:"idea_quantity,omitempty"`
	AmountMinor  int64      `json:"amount_minor,omitempty"`
	Currency     string     `json:"currency,omitempty"`
	StripeStatus string     `json:"stripe_status,omitempty"`
	ErrorSummary string     `json:"error_summary,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

// AdminPaymentDisputeSummary follows the same safe event-fact boundary.
type AdminPaymentDisputeSummary struct {
	ID                  string     `json:"id,omitempty"`
	Status              string     `json:"status"`
	ProviderStatus      string     `json:"provider_status,omitempty"`
	AmountMinor         int64      `json:"amount_minor,omitempty"`
	Currency            string     `json:"currency,omitempty"`
	RatioBasisPoints    int64      `json:"ratio_basis_points,omitempty"`
	TargetIdeaQuantity  int64      `json:"target_idea_quantity,omitempty"`
	DebitedIdeaQuantity int64      `json:"debited_idea_quantity,omitempty"`
	UnrecoveredCredits  int64      `json:"unrecovered_credits,omitempty"`
	StripeDashboardURL  string     `json:"stripe_dashboard_url,omitempty"`
	OpenedAt            *time.Time `json:"opened_at,omitempty"`
	ClosedAt            *time.Time `json:"closed_at,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

type AdminPaymentRefundResult struct {
	PublicOrderNumber string                             `json:"public_order_number"`
	IdeaQuantity      int64                              `json:"idea_quantity"`
	AmountMinor       int64                              `json:"amount_minor"`
	Currency          string                             `json:"currency"`
	Status            paymentsdomain.PaymentRefundStatus `json:"status"`
	CreatedAt         time.Time                          `json:"created_at"`
	UpdatedAt         time.Time                          `json:"updated_at"`
}

// merged from internal/payments/application/contracts/refund.go (structural consolidation; symbols unchanged).
// CreatePaymentRefundCommand describes an administrator-requested refund.
type CreatePaymentRefundCommand struct {
	ActorID        int64
	OrderNumber    string
	IdeaQuantity   int64
	IdempotencyKey string
}
