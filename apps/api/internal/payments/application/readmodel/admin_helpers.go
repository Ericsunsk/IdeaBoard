package readmodel

import (
	"math/big"
	"regexp"
	"strings"

	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

var stripeDisputeIDPattern = regexp.MustCompile(`^dp_[A-Za-z0-9]+$`)

func StripeDisputeDashboardURL(mode paymentports.StripeMode, disputeID string) string {
	disputeID = strings.TrimSpace(disputeID)
	if !stripeDisputeIDPattern.MatchString(disputeID) {
		return ""
	}
	if mode == paymentports.StripeModeTest {
		return "https://dashboard.stripe.com/test/disputes/" + disputeID
	}
	if mode == paymentports.StripeModeLive {
		return "https://dashboard.stripe.com/disputes/" + disputeID
	}
	return ""
}

func DisputeRatioBasisPoints(amountMinor int64, paidTotalMinor *int64) int64 {
	if amountMinor <= 0 || paidTotalMinor == nil || *paidTotalMinor <= 0 {
		return 0
	}
	ratio := new(big.Int).Mul(big.NewInt(amountMinor), big.NewInt(10_000))
	ratio.Quo(ratio, big.NewInt(*paidTotalMinor))
	if !ratio.IsInt64() {
		return 0
	}
	return ratio.Int64()
}

func StripeStatus(status paymentsdomain.PaymentOrderStatus) string {
	switch status {
	case paymentsdomain.PaymentOrderCreating:
		return "creating"
	case paymentsdomain.PaymentOrderCheckoutOpen:
		return "open"
	case paymentsdomain.PaymentOrderAsyncPaymentPending:
		return "async_payment_pending"
	case paymentsdomain.PaymentOrderPaidPendingCredit, paymentsdomain.PaymentOrderCredited:
		return "paid"
	case paymentsdomain.PaymentOrderCancelled:
		return "cancelled"
	case paymentsdomain.PaymentOrderExpired:
		return "expired"
	case paymentsdomain.PaymentOrderFailed:
		return "payment_failed"
	default:
		return "unknown"
	}
}

func CreditStatus(status paymentsdomain.PaymentOrderStatus) string {
	switch status {
	case paymentsdomain.PaymentOrderPaidPendingCredit:
		return "pending"
	case paymentsdomain.PaymentOrderCredited:
		return "credited"
	default:
		return "not_applicable"
	}
}

func MaskPaymentIdentity(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return ""
	}
	return value[:4] + "…" + value[len(value)-4:]
}
