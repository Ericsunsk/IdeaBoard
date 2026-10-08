package postgres

import (
	"strings"

	"ideaboard/api/internal/db/dbutil"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"

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
		PaidTotalMinor:    NullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		PaidAt:            NullTimePointer(row.PaidAt),
		CreditedAt:        NullTimePointer(row.CreditedAt),
		LastCheckedAt:     NullTimePointer(row.LastCheckedAt),
		AnomalousAt:       NullTimePointer(row.AnomalousAt),
	}
}

func decodeAdminPaymentOrderDetailFacts(row dbsqlc.GetAdminPaymentOrderRow) paymentreadmodel.AdminPaymentOrderDetailFacts {
	return paymentreadmodel.AdminPaymentOrderDetailFacts{
		PublicOrderNumber: row.PublicOrderNumber, InternalUserID: row.UserID,
		UserPublicID: uuid.UUID(row.UserPublicID.Bytes), DisplayName: row.DisplayName,
		Status: row.Status, IdeaQuantity: row.IdeaQuantity,
		BaseCurrency: strings.ToLower(row.BaseCurrency), BaseAmountMinor: row.BaseAmountMinor,
		PaidSubtotalMinor: NullInt64Pointer(row.PaidSubtotalMinor),
		PaidTaxMinor:      NullInt64Pointer(row.PaidTaxMinor),
		PaidTotalMinor:    NullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		StripeAccountID:   row.StripeAccountID, StripeCustomerID: row.StripeCustomerID,
		StripeCheckoutSessionID: pgTextString(row.StripeCheckoutSessionID),
		StripePaymentIntentID:   row.StripePaymentIntentID, StripeChargeID: row.StripeChargeID,
		TermsVersion: row.TermsVersion, RefundPolicyVersion: row.RefundPolicyVersion,
		ReceiptURL: row.ReceiptUrl, ExpiresAt: dbutil.TimeValue(row.ExpiresAt), CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
		PaidAt: NullTimePointer(row.PaidAt), CreditedAt: NullTimePointer(row.CreditedAt),
		LastErrorCode: pgTextString(row.LastErrorCode), LastErrorSummary: pgTextString(row.LastErrorSummary),
		LastCheckedAt: NullTimePointer(row.LastCheckedAt), NextAttemptAt: NullTimePointer(row.NextAttemptAt),
		AnomalousAt: NullTimePointer(row.AnomalousAt),
	}
}

func decodeAdminPaymentWebhookEventRecord(row dbsqlc.ListAdminPaymentOrderWebhookEventsRow) paymentreadmodel.AdminPaymentWebhookEventRecord {
	return paymentreadmodel.AdminPaymentWebhookEventRecord{
		EventType:      row.EventType,
		StripeObjectID: row.StripeObjectID,
		ReceivedAt:     dbutil.TimeValue(row.ReceivedAt),
		ProcessedAt:    NullTimePointer(row.ProcessedAt),
	}
}
