package reconciliation

import (
	"errors"
	"ideaboard/api/internal/platform/errorrender"
	"strings"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

func FailureDetail(err error) (string, string) {
	var gatewayErr *paymentports.GatewayError
	if errors.As(err, &gatewayErr) {
		return string(gatewayErr.Code()), boundedText(gatewayErr.Error(), 500)
	}
	if errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable) {
		return string(paymentports.GatewayErrorProviderUnavailable), errorrender.MsgFailureDetail
	}
	if errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) {
		return "configuration_invalid", errorrender.MsgFailureDetail2
	}
	if errors.Is(err, paymentcontracts.ErrStripeConfigurationNotFound) {
		return "configuration_not_found", errorrender.MsgFailureDetail3
	}
	if errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) {
		return "configuration_unavailable", errorrender.MsgFailureDetail4
	}
	if errors.Is(err, paymentcontracts.ErrInvalidStripeAPIKey) {
		return string(paymentports.GatewayErrorInvalidKey), errorrender.MsgFailureDetail5
	}
	if errors.Is(err, paymentcontracts.ErrStripePermissionDenied) {
		return string(paymentports.GatewayErrorPermissionDenied), errorrender.MsgFailureDetail6
	}
	if errors.Is(err, paymentcontracts.ErrStripeAccountUnavailable) {
		return string(paymentports.GatewayErrorAccountUnavailable), errorrender.MsgFailureDetail7
	}
	if errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable) {
		return string(paymentports.GatewayErrorCapabilityMissing), errorrender.MsgFailureDetail8
	}
	if errors.Is(err, paymentsdomain.ErrWalletNotRechargeable) {
		return "wallet_not_rechargeable", errorrender.MsgFailureDetail9
	}
	if errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyConflict) {
		return "identity_conflict", errorrender.MsgFailureDetail10
	}
	return "ledger_retry", errorrender.MsgFailureDetail11
}

func OperationalErrorSummary(err error, fallback string) string {
	if err == nil {
		return ""
	}
	var gatewayErr *paymentports.GatewayError
	if errors.As(err, &gatewayErr) {
		_, summary := FailureDetail(gatewayErr)
		return summary
	}
	known := []error{
		paymentcontracts.ErrStripeProviderUnavailable,
		paymentcontracts.ErrStripeConfigurationInvalid,
		paymentcontracts.ErrStripeConfigurationNotFound,
		paymentsdomain.ErrPurchasesUnavailable,
		paymentcontracts.ErrInvalidStripeAPIKey,
		paymentcontracts.ErrStripePermissionDenied,
		paymentcontracts.ErrStripeAccountUnavailable,
		paymentcontracts.ErrStripeCapabilityUnavailable,
		paymentsdomain.ErrWalletNotRechargeable,
		paymentsdomain.ErrCheckoutIdempotencyConflict,
	}
	for _, candidate := range known {
		if errors.Is(err, candidate) {
			_, summary := FailureDetail(candidate)
			return summary
		}
	}
	return boundedText(fallback, 500)
}

// FailureIsTransient reports whether the reconciliation runner should ask
// River for a retry instead of recording a terminal failure.
func FailureIsTransient(err error) bool {
	var gatewayErr *paymentports.GatewayError
	if errors.As(err, &gatewayErr) {
		return gatewayErr.Code() == paymentports.GatewayErrorProviderUnavailable
	}
	if errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable) {
		return true
	}
	if errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) || errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyConflict) ||
		errors.Is(err, paymentsdomain.ErrInvalidIdeaQuantity) || errors.Is(err, paymentsdomain.ErrWalletNotRechargeable) ||
		errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) || errors.Is(err, paymentcontracts.ErrStripeConfigurationNotFound) ||
		errors.Is(err, paymentcontracts.ErrInvalidStripeAPIKey) || errors.Is(err, paymentcontracts.ErrStripePermissionDenied) ||
		errors.Is(err, paymentcontracts.ErrStripeAccountUnavailable) || errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable) {
		return false
	}
	// Credit and database failures are retriable; durable state remains pending
	// until the atomic transition succeeds.
	return true
}

func boundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
