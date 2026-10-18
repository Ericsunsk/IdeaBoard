package postgres

import (
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentsdomain "ideaboard/api/internal/payments/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// PaymentOrder maps the generated SQL row into the Payments domain model.
// Keeping this conversion in the Postgres adapter prevents database details
// from leaking into the application composition package.
func PaymentOrder(row dbsqlc.PaymentOrder) paymentsdomain.PaymentOrder {
	return paymentsdomain.PaymentOrder{
		ID: pgUUIDString(row.ID), PublicOrderNumber: row.PublicOrderNumber, PublicToken: pgUUIDString(row.PublicToken),
		UserID: row.UserID, ClientIdempotencyKey: row.ClientIdempotencyKey, RequestHash: row.RequestHash,
		Status: paymentsdomain.PaymentOrderStatus(row.Status), StripeCredentialID: pgUUIDString(row.StripeCredentialID), StripeAccountID: row.StripeAccountID,
		StripeCustomerID: row.StripeCustomerID, StripeProductID: row.StripeProductID,
		StripeCheckoutSessionID: PgTextString(row.StripeCheckoutSessionID),
		StripePaymentIntentID:   row.StripePaymentIntentID, StripeChargeID: row.StripeChargeID,
		IdeaQuantity: row.IdeaQuantity, BaseCurrency: row.BaseCurrency, BaseAmountMinor: row.BaseAmountMinor,
		PaidSubtotalMinor: NullInt64Pointer(row.PaidSubtotalMinor), PaidTaxMinor: NullInt64Pointer(row.PaidTaxMinor),
		PaidTotalMinor: NullInt64Pointer(row.PaidTotalMinor), PaidCurrency: row.PaidCurrency,
		TermsVersion: row.TermsVersion, RefundPolicyVersion: row.RefundPolicyVersion,
		ReturnBaseURL: row.ReturnBaseUrl, CheckoutURL: row.CheckoutUrl, ReceiptURL: row.ReceiptUrl,
		CreditTransactionID: PgTextString(row.CreditTransactionID),
		ExpiresAt:           dbutil.TimeValue(row.ExpiresAt), PaidAt: NullTimePointer(row.PaidAt), CreditedAt: NullTimePointer(row.CreditedAt),
		CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
	}
}

func ParseUUID(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}

func pgUUIDToPgType(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func PgTextString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func NullInt64Pointer(value pgtype.Int8) *int64 {
	return dbutil.Int64Ptr(value)
}

func NullTimePointer(value pgtype.Timestamptz) *time.Time {
	return dbutil.TimePtr(value)
}
