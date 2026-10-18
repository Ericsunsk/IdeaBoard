package readmodel

import (
	"strings"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"

	"github.com/google/uuid"
)

// AdminPaymentOrderFilters is the validated keyset query used by the Admin
// payment-order read model. It contains no persistence implementation detail.
type AdminPaymentOrderFilters struct {
	OrderNumber  string
	UserPublicID uuid.UUID
	DisplayName  string
	Status       string
	AnomalyOnly  bool
	Cursor       *AdminPaymentOrderCursor
	Limit        int
}

// AdminPaymentOrderRecord is the application-owned Admin list row projected
// by the postgres adapter. It carries only the facts actually rendered by
// the Admin order summary plus the keyset position (order ID + created_at).
// Derived Stripe/credit display status, pagination (limit+1/cursor), and
// sorting stay in the application service.
type AdminPaymentOrderRecord struct {
	OrderID           uuid.UUID
	CreatedAt         time.Time
	PublicOrderNumber string
	UserPublicID      uuid.UUID
	DisplayName       string
	Status            string
	IdeaQuantity      int64
	BaseCurrency      string
	BaseAmountMinor   int64
	PaidTotalMinor    *int64
	PaidCurrency      string
	PaidAt            *time.Time
	CreditedAt        *time.Time
	LastCheckedAt     *time.Time
	AnomalousAt       *time.Time
}

// AdminPaymentOrderDetailFacts is the application-owned raw detail projected
// by the postgres adapter. It carries only the facts actually rendered by
// the safe Admin detail (order, paid, Stripe identity, reconciliation) plus
// the internal user key for application-side wallet reads. Redaction,
// derived Stripe/credit status, and refund/dispute/webhook aggregation stay
// in the application service.
type AdminPaymentOrderDetailFacts struct {
	PublicOrderNumber       string
	InternalUserID          int64
	UserPublicID            uuid.UUID
	DisplayName             string
	Status                  string
	IdeaQuantity            int64
	BaseCurrency            string
	BaseAmountMinor         int64
	PaidSubtotalMinor       *int64
	PaidTaxMinor            *int64
	PaidTotalMinor          *int64
	PaidCurrency            string
	StripeAccountID         string
	StripeCustomerID        string
	StripeCheckoutSessionID string
	StripePaymentIntentID   string
	StripeChargeID          string
	TermsVersion            string
	RefundPolicyVersion     string
	ReceiptURL              string
	ExpiresAt               time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
	PaidAt                  *time.Time
	CreditedAt              *time.Time
	LastErrorCode           string
	LastErrorSummary        string
	LastCheckedAt           *time.Time
	NextAttemptAt           *time.Time
	AnomalousAt             *time.Time
}

// AdminPaymentWebhookEventRecord is the application-owned webhook fact used
// for Admin detail aggregation. It carries only the facts actually rendered:
// the Stripe event vocabulary for order/refund/dispute attribution, the
// object ID for durable-set dedup, and receive/process times. Provider
// payloads, processing results, and Stripe creation times are never exposed.
type AdminPaymentWebhookEventRecord struct {
	EventType      string
	StripeObjectID string
	ReceivedAt     time.Time
	ProcessedAt    *time.Time
}

// AdminPaymentOrderDetailRecord carries the sanitized Admin detail together
// with the internal user key needed for application-side wallet reads. The
// internal key is never serialized by the HTTP adapter.
type AdminPaymentOrderDetailRecord struct {
	paymentcontracts.AdminPaymentOrderDetail
	InternalUserID int64
}

// AdminPaymentOrderSummaryFromRecord builds the safe Admin list summary from
// an application-owned record. Derived Stripe/credit display status lives
// here, not in the postgres adapter.
func AdminPaymentOrderSummaryFromRecord(record AdminPaymentOrderRecord) paymentcontracts.AdminPaymentOrderSummary {
	status := paymentsdomain.PaymentOrderStatus(record.Status)
	return paymentcontracts.AdminPaymentOrderSummary{
		PublicOrderNumber: record.PublicOrderNumber,
		User:              paymentcontracts.AdminPaymentOrderUser{PublicID: record.UserPublicID, DisplayName: record.DisplayName},
		IdeaQuantity:      record.IdeaQuantity,
		BaseAmountMinor:   record.BaseAmountMinor,
		BaseCurrency:      strings.ToLower(record.BaseCurrency),
		PaidAmountMinor:   record.PaidTotalMinor,
		PaidCurrency:      strings.ToLower(record.PaidCurrency),
		Status:            status,
		StripeStatus:      StripeStatus(status),
		CreditStatus:      CreditStatus(status),
		CreatedAt:         record.CreatedAt,
		PaidAt:            record.PaidAt,
		CreditedAt:        record.CreditedAt,
		LastCheckedAt:     record.LastCheckedAt,
		AnomalousAt:       record.AnomalousAt,
	}
}

// AdminPaymentOrderDetailFromFacts builds the sanitized Admin detail from
// application-owned facts. Redaction and derived display status live here;
// the postgres adapter only decodes nullable storage facts.
func AdminPaymentOrderDetailFromFacts(facts AdminPaymentOrderDetailFacts) AdminPaymentOrderDetailRecord {
	status := paymentsdomain.PaymentOrderStatus(facts.Status)
	anomalousAt := facts.AnomalousAt
	return AdminPaymentOrderDetailRecord{AdminPaymentOrderDetail: paymentcontracts.AdminPaymentOrderDetail{
		PublicOrderNumber: facts.PublicOrderNumber,
		User:              paymentcontracts.AdminPaymentOrderUser{PublicID: facts.UserPublicID, DisplayName: facts.DisplayName},
		IdeaQuantity:      facts.IdeaQuantity,
		BaseAmountMinor:   facts.BaseAmountMinor,
		BaseCurrency:      strings.ToLower(facts.BaseCurrency),
		PaidSubtotalMinor: facts.PaidSubtotalMinor,
		PaidTaxMinor:      facts.PaidTaxMinor,
		PaidTotalMinor:    facts.PaidTotalMinor,
		PaidCurrency:      strings.ToLower(facts.PaidCurrency),
		Status:            status,
		StripeStatus:      StripeStatus(status),
		CreditStatus:      CreditStatus(status),
		Stripe: paymentcontracts.AdminPaymentStripeSummary{
			AccountID: MaskPaymentIdentity(facts.StripeAccountID), CustomerID: MaskPaymentIdentity(facts.StripeCustomerID),
			CheckoutSessionID: MaskPaymentIdentity(facts.StripeCheckoutSessionID),
			PaymentIntentID:   MaskPaymentIdentity(facts.StripePaymentIntentID), ChargeID: MaskPaymentIdentity(facts.StripeChargeID),
		},
		TermsVersion: facts.TermsVersion, RefundPolicyVersion: facts.RefundPolicyVersion,
		ReceiptURL: facts.ReceiptURL, ExpiresAt: facts.ExpiresAt, CreatedAt: facts.CreatedAt, UpdatedAt: facts.UpdatedAt,
		PaidAt: facts.PaidAt, CreditedAt: facts.CreditedAt,
		Reconciliation: paymentcontracts.AdminPaymentReconciliation{
			Anomalous: anomalousAt != nil, AnomalousAt: anomalousAt,
			LastCheckedAt: facts.LastCheckedAt, NextAttemptAt: facts.NextAttemptAt,
			LastErrorCode: facts.LastErrorCode, LastErrorSummary: facts.LastErrorSummary,
		},
		Refunds:  []paymentcontracts.AdminPaymentRefundSummary{},
		Disputes: []paymentcontracts.AdminPaymentDisputeSummary{},
	}, InternalUserID: facts.InternalUserID}
}
