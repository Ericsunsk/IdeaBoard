package postgres

import (
	"database/sql"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentsdomain "ideaboard/api/internal/payments/domain"

	"github.com/google/uuid"
)

// PaymentOrder maps the generated SQL row into the Payments domain model.
// Keeping this conversion in the Postgres adapter prevents database details
// from leaking into the application composition package.
func PaymentOrder(row dbsqlc.PaymentOrder) paymentsdomain.PaymentOrder {
	return paymentsdomain.PaymentOrder{
		ID: row.ID.String(), PublicOrderNumber: row.PublicOrderNumber, PublicToken: row.PublicToken.String(),
		UserID: row.UserID, ClientIdempotencyKey: row.ClientIdempotencyKey, RequestHash: row.RequestHash,
		Status: paymentsdomain.PaymentOrderStatus(row.Status), StripeCredentialID: row.StripeCredentialID.String(), StripeAccountID: row.StripeAccountID,
		StripeCustomerID: row.StripeCustomerID, StripeProductID: row.StripeProductID,
		StripeCheckoutSessionID: row.StripeCheckoutSessionID.String,
		StripePaymentIntentID:   row.StripePaymentIntentID, StripeChargeID: row.StripeChargeID,
		IdeaQuantity: row.IdeaQuantity, BaseCurrency: row.BaseCurrency, BaseAmountMinor: row.BaseAmountMinor,
		PaidSubtotalMinor: NullInt64Pointer(row.PaidSubtotalMinor), PaidTaxMinor: NullInt64Pointer(row.PaidTaxMinor),
		PaidTotalMinor: NullInt64Pointer(row.PaidTotalMinor), PaidCurrency: row.PaidCurrency,
		TermsVersion: row.TermsVersion, RefundPolicyVersion: row.RefundPolicyVersion,
		ReturnBaseURL: row.ReturnBaseUrl, CheckoutURL: row.CheckoutUrl, ReceiptURL: row.ReceiptUrl,
		CreditTransactionID: row.CreditTransactionID.String,
		ExpiresAt:           row.ExpiresAt, PaidAt: NullTimePointer(row.PaidAt), CreditedAt: NullTimePointer(row.CreditedAt),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func ParseUUID(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}

func NullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func NullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
