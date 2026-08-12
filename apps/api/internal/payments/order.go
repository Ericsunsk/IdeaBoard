package payments

import (
	"database/sql"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/google/uuid"
)

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

type PaymentCustomer struct {
	UserID           int64
	StripeAccountID  string
	StripeCustomerID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
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

func paymentOrder(row dbsqlc.PaymentOrder) PaymentOrder {
	return PaymentOrder{
		ID: row.ID.String(), PublicOrderNumber: row.PublicOrderNumber, PublicToken: row.PublicToken.String(),
		UserID: row.UserID, ClientIdempotencyKey: row.ClientIdempotencyKey, RequestHash: row.RequestHash,
		Status: PaymentOrderStatus(row.Status), StripeCredentialID: row.StripeCredentialID.String(), StripeAccountID: row.StripeAccountID,
		StripeCustomerID: row.StripeCustomerID, StripeProductID: row.StripeProductID,
		StripeCheckoutSessionID: row.StripeCheckoutSessionID.String,
		StripePaymentIntentID:   row.StripePaymentIntentID, StripeChargeID: row.StripeChargeID,
		IdeaQuantity: row.IdeaQuantity, BaseCurrency: row.BaseCurrency, BaseAmountMinor: row.BaseAmountMinor,
		PaidSubtotalMinor: nullInt64Pointer(row.PaidSubtotalMinor), PaidTaxMinor: nullInt64Pointer(row.PaidTaxMinor),
		PaidTotalMinor: nullInt64Pointer(row.PaidTotalMinor), PaidCurrency: row.PaidCurrency,
		TermsVersion: row.TermsVersion, RefundPolicyVersion: row.RefundPolicyVersion,
		ReturnBaseURL: row.ReturnBaseUrl, CheckoutURL: row.CheckoutUrl, ReceiptURL: row.ReceiptUrl,
		CreditTransactionID: row.CreditTransactionID.String,
		ExpiresAt:           row.ExpiresAt, PaidAt: nullTimePointer(row.PaidAt), CreditedAt: nullTimePointer(row.CreditedAt),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func parseOrderUUID(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
