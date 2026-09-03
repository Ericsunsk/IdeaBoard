package refund

import (
	"strings"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

// ValidateProviderRefund enforces provider identity and amount invariants.
func ValidateProviderRefund(refund paymentsdomain.PaymentRefund, providerRefund paymentports.StripeRefund) error {
	providerID := strings.TrimSpace(providerRefund.ID)
	if providerID == "" || (refund.StripeRefundID != "" && refund.StripeRefundID != providerID) {
		return paymentcontracts.ErrRefundProviderConflict
	}
	if (providerRefund.ChargeID != "" && providerRefund.ChargeID != refund.StripeChargeID) ||
		(providerRefund.AmountMinor > 0 && providerRefund.AmountMinor != refund.AmountMinor) ||
		(providerRefund.Currency != "" && !strings.EqualFold(providerRefund.Currency, refund.Currency)) {
		return paymentcontracts.ErrRefundProviderConflict
	}
	return nil
}
