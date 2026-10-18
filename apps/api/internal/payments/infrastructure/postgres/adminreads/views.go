package adminreads

import (
	"strings"

	"ideaboard/api/internal/db/dbutil"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	postgres "ideaboard/api/internal/payments/infrastructure/postgres"

	"github.com/google/uuid"
)

func decodeAdminPaymentOrderRecord(row dbsqlc.ListAdminPaymentOrdersRow) paymentreadmodel.AdminPaymentOrderRecord {
	return paymentreadmodel.AdminPaymentOrderRecord{
		OrderID:           uuid.UUID(row.ID.Bytes),
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		PublicOrderNumber: row.PublicOrderNumber,
		UserPublicID:      uuid.UUID(row.UserPublicID.Bytes),
		DisplayName:       row.DisplayName,
		Status:            row.Status,
		IdeaQuantity:      row.IdeaQuantity,
		BaseCurrency:      strings.ToLower(row.BaseCurrency),
		BaseAmountMinor:   row.BaseAmountMinor,
		PaidTotalMinor:    postgres.NullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		PaidAt:            postgres.NullTimePointer(row.PaidAt),
		CreditedAt:        postgres.NullTimePointer(row.CreditedAt),
		LastCheckedAt:     postgres.NullTimePointer(row.LastCheckedAt),
		AnomalousAt:       postgres.NullTimePointer(row.AnomalousAt),
	}
}

func decodeAdminPaymentOrderDetailFacts(row dbsqlc.GetAdminPaymentOrderRow) paymentreadmodel.AdminPaymentOrderDetailFacts {
	return paymentreadmodel.AdminPaymentOrderDetailFacts{
		PublicOrderNumber: row.PublicOrderNumber, InternalUserID: row.UserID,
		UserPublicID: uuid.UUID(row.UserPublicID.Bytes), DisplayName: row.DisplayName,
		Status: row.Status, IdeaQuantity: row.IdeaQuantity,
		BaseCurrency: strings.ToLower(row.BaseCurrency), BaseAmountMinor: row.BaseAmountMinor,
		PaidSubtotalMinor: postgres.NullInt64Pointer(row.PaidSubtotalMinor),
		PaidTaxMinor:      postgres.NullInt64Pointer(row.PaidTaxMinor),
		PaidTotalMinor:    postgres.NullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		StripeAccountID:   row.StripeAccountID, StripeCustomerID: row.StripeCustomerID,
		StripeCheckoutSessionID: postgres.PgTextString(row.StripeCheckoutSessionID),
		StripePaymentIntentID:   row.StripePaymentIntentID, StripeChargeID: row.StripeChargeID,
		TermsVersion: row.TermsVersion, RefundPolicyVersion: row.RefundPolicyVersion,
		ReceiptURL: row.ReceiptUrl, ExpiresAt: dbutil.TimeValue(row.ExpiresAt), CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
		PaidAt: postgres.NullTimePointer(row.PaidAt), CreditedAt: postgres.NullTimePointer(row.CreditedAt),
		LastErrorCode: postgres.PgTextString(row.LastErrorCode), LastErrorSummary: postgres.PgTextString(row.LastErrorSummary),
		LastCheckedAt: postgres.NullTimePointer(row.LastCheckedAt), NextAttemptAt: postgres.NullTimePointer(row.NextAttemptAt),
		AnomalousAt: postgres.NullTimePointer(row.AnomalousAt),
	}
}

func decodeAdminPaymentWebhookEventRecord(row dbsqlc.ListAdminPaymentOrderWebhookEventsRow) paymentreadmodel.AdminPaymentWebhookEventRecord {
	return paymentreadmodel.AdminPaymentWebhookEventRecord{
		EventType:      row.EventType,
		StripeObjectID: row.StripeObjectID,
		ReceivedAt:     dbutil.TimeValue(row.ReceivedAt),
		ProcessedAt:    postgres.NullTimePointer(row.ProcessedAt),
	}
}
