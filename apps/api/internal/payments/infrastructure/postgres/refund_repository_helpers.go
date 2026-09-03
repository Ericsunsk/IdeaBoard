package postgres

import (
	"database/sql"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"strings"
	"time"
)

func validateExternalRefund(order paymentRefundOrderFacts, provider paymentports.StripeRefund) error {
	if order.Status != paymentsdomain.PaymentOrderCredited || order.PaidSubtotal <= 0 || order.PaidTotal <= 0 || order.IdeaQuantity <= 0 {
		return paymentcontracts.ErrRefundOrderNotEligible
	}
	if !strings.EqualFold(provider.Currency, order.PaidCurrency) {
		return paymentcontracts.ErrRefundProviderConflict
	}
	return nil
}

func validatePendingRefundEligibility(order paymentsdomain.PaymentOrder, paidSubtotal, paidTotal sql.NullInt64, paidAt sql.NullTime, now time.Time) error {
	if order.Status != paymentsdomain.PaymentOrderCredited || order.StripeChargeID == "" || order.IdeaQuantity <= 0 || !paidSubtotal.Valid || paidSubtotal.Int64 <= 0 || !paidTotal.Valid || paidTotal.Int64 <= 0 || !paidAt.Valid {
		return paymentcontracts.ErrRefundOrderNotEligible
	}
	if !now.Before(paidAt.Time.Add(paymentsdomain.RefundEligibilityWindow)) {
		return paymentcontracts.ErrRefundExpired
	}
	return nil
}

func scanPaymentRefund(row interface{ Scan(...any) error }, refund *paymentsdomain.PaymentRefund) error {
	return row.Scan(&refund.ID, &refund.PaymentOrderID, &refund.UserID, &refund.IdeaQuantity, &refund.AmountMinor, &refund.Currency, &refund.Status, &refund.StripeStatus, &refund.StripeRefundID, &refund.StripeChargeID, &refund.CreditHoldTransactionID, &refund.CreditTransactionID, &refund.ErrorSummary, &refund.CreatedAt, &refund.UpdatedAt)
}
