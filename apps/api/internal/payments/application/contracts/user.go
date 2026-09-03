package contracts

import (
	"context"
	"time"

	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

type UserCheckoutService interface {
	GetPurchaseOptions(context.Context, int64) (PurchaseOptions, error)
	CreateCheckout(context.Context, paymentsdomain.CreateCheckoutCommand) (CheckoutResult, error)
	GetPaymentOrderReturnStatus(context.Context, int64, string) (PaymentOrderReturnStatus, error)
	ListPendingOrders(context.Context, int64) (PendingPaymentOrderList, error)
	ContinuePendingOrder(context.Context, int64, string) (PendingPaymentOrder, error)
	AbandonPendingOrder(context.Context, int64, string) (PendingPaymentOrder, error)
	ListRecharges(context.Context, int64, string, int) (RechargeHistoryPage, error)
}

type WebhookService interface {
	ReceiveStripeWebhook(context.Context, []byte, string) (paymentports.StripeWebhookReceipt, error)
}

type PurchaseOptions struct {
	PurchasesAvailable  bool                 `json:"purchases_available"`
	UnavailableReason   string               `json:"unavailable_reason,omitempty"`
	RestrictionType     string               `json:"restriction_type,omitempty"`
	QuickIdeaQuantities []int64              `json:"quick_idea_quantities"`
	MinIdeaQuantity     int64                `json:"min_idea_quantity"`
	MaxIdeaQuantity     int64                `json:"max_idea_quantity"`
	IdeasPerUSD         int64                `json:"ideas_per_usd"`
	Terms               PaymentLegalDocument `json:"terms"`
	RefundPolicy        PaymentLegalDocument `json:"refund_policy"`
}

type PaymentLegalDocument struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type CheckoutResult struct {
	CheckoutURL string    `json:"checkout_url"`
	PublicToken string    `json:"public_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	Reused      bool      `json:"reused"`
}

type PaymentOrderReturnStatus struct {
	IdeaQuantity int64     `json:"idea_quantity"`
	Status       string    `json:"status"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PendingPaymentOrder struct {
	PublicToken  string    `json:"public_token"`
	IdeaQuantity int64     `json:"idea_quantity"`
	CheckoutURL  string    `json:"checkout_url,omitempty"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PendingPaymentOrderList struct {
	Items []PendingPaymentOrder `json:"items"`
}

type RechargeHistoryItem struct {
	OccurredAt           time.Time `json:"occurred_at"`
	IdeaQuantity         int64     `json:"idea_quantity"`
	PaidAmountMinor      int64     `json:"paid_amount_minor,omitempty"`
	PaidCurrency         string    `json:"paid_currency,omitempty"`
	Status               string    `json:"status"`
	RefundedIdeaQuantity int64     `json:"refunded_idea_quantity,omitempty"`
	RefundedAmountMinor  int64     `json:"refunded_amount_minor,omitempty"`
	ReceiptURL           string    `json:"receipt_url,omitempty"`
}

type RechargeHistoryPage struct {
	Items      []RechargeHistoryItem `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
}
