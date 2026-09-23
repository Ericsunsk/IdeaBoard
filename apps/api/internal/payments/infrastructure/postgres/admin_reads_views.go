package postgres

import (
	"ideaboard/api/internal/db/dbutil"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	paymentsdomain "ideaboard/api/internal/payments/domain"

	"github.com/google/uuid"
)

func adminPaymentOrderSummary(row dbsqlc.ListAdminPaymentOrdersRow) paymentcontracts.AdminPaymentOrderSummary {
	return paymentcontracts.AdminPaymentOrderSummary{
		PublicOrderNumber: row.PublicOrderNumber,
		User:              paymentcontracts.AdminPaymentOrderUser{PublicID: uuid.UUID(row.UserPublicID.Bytes), DisplayName: row.DisplayName},
		IdeaQuantity:      row.IdeaQuantity,
		BaseAmountMinor:   row.BaseAmountMinor,
		BaseCurrency:      strings.ToLower(row.BaseCurrency),
		PaidAmountMinor:   NullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		Status:            paymentsdomain.PaymentOrderStatus(row.Status),
		StripeStatus:      paymentreadmodel.StripeStatus(paymentsdomain.PaymentOrderStatus(row.Status)),
		CreditStatus:      paymentreadmodel.CreditStatus(paymentsdomain.PaymentOrderStatus(row.Status)),
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
		PaidAt:            NullTimePointer(row.PaidAt),
		CreditedAt:        NullTimePointer(row.CreditedAt),
		LastCheckedAt:     NullTimePointer(row.LastCheckedAt),
		AnomalousAt:       NullTimePointer(row.AnomalousAt),
	}
}

func adminPaymentOrderDetail(row dbsqlc.GetAdminPaymentOrderRow) paymentreadmodel.AdminPaymentOrderDetailRecord {
	status := paymentsdomain.PaymentOrderStatus(row.Status)
	return paymentreadmodel.AdminPaymentOrderDetailRecord{AdminPaymentOrderDetail: paymentcontracts.AdminPaymentOrderDetail{
		PublicOrderNumber: row.PublicOrderNumber,
		User:              paymentcontracts.AdminPaymentOrderUser{PublicID: uuid.UUID(row.UserPublicID.Bytes), DisplayName: row.DisplayName},
		IdeaQuantity:      row.IdeaQuantity,
		BaseAmountMinor:   row.BaseAmountMinor,
		BaseCurrency:      strings.ToLower(row.BaseCurrency),
		PaidSubtotalMinor: NullInt64Pointer(row.PaidSubtotalMinor),
		PaidTaxMinor:      NullInt64Pointer(row.PaidTaxMinor),
		PaidTotalMinor:    NullInt64Pointer(row.PaidTotalMinor),
		PaidCurrency:      strings.ToLower(row.PaidCurrency),
		Status:            status,
		StripeStatus:      paymentreadmodel.StripeStatus(status),
		CreditStatus:      paymentreadmodel.CreditStatus(status),
		Stripe: paymentcontracts.AdminPaymentStripeSummary{
			AccountID: paymentreadmodel.MaskPaymentIdentity(row.StripeAccountID), CustomerID: paymentreadmodel.MaskPaymentIdentity(row.StripeCustomerID),
			CheckoutSessionID: paymentreadmodel.MaskPaymentIdentity(pgTextString(row.StripeCheckoutSessionID)),
			PaymentIntentID:   paymentreadmodel.MaskPaymentIdentity(row.StripePaymentIntentID), ChargeID: paymentreadmodel.MaskPaymentIdentity(row.StripeChargeID),
		},
		TermsVersion: row.TermsVersion, RefundPolicyVersion: row.RefundPolicyVersion,
		ReceiptURL: row.ReceiptUrl, ExpiresAt: dbutil.TimeValue(row.ExpiresAt), CreatedAt: dbutil.TimeValue(row.CreatedAt), UpdatedAt: dbutil.TimeValue(row.UpdatedAt),
		PaidAt: NullTimePointer(row.PaidAt), CreditedAt: NullTimePointer(row.CreditedAt),
		Reconciliation: paymentcontracts.AdminPaymentReconciliation{
			Anomalous: row.AnomalousAt.Valid, AnomalousAt: NullTimePointer(row.AnomalousAt),
			LastCheckedAt: NullTimePointer(row.LastCheckedAt), NextAttemptAt: NullTimePointer(row.NextAttemptAt),
			LastErrorCode: pgTextString(row.LastErrorCode), LastErrorSummary: pgTextString(row.LastErrorSummary),
		},
		Refunds:  []paymentcontracts.AdminPaymentRefundSummary{},
		Disputes: []paymentcontracts.AdminPaymentDisputeSummary{},
	}, InternalUserID: row.UserID}
}
