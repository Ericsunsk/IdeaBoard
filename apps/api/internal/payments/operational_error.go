package payments

import "errors"

// operationalErrorSummary is the persistence boundary for provider and
// internal failures. Unknown errors may contain credentials, payloads, or
// stack details, so they are never stored verbatim.
func operationalErrorSummary(err error, fallback string) string {
	if err == nil {
		return ""
	}
	var gatewayErr *GatewayError
	if errors.As(err, &gatewayErr) {
		_, summary := reconciliationFailureDetail(gatewayErr)
		return summary
	}
	known := []error{
		ErrStripeProviderUnavailable,
		ErrStripeConfigurationInvalid,
		ErrStripeConfigurationNotFound,
		ErrPurchasesUnavailable,
		ErrInvalidStripeAPIKey,
		ErrStripePermissionDenied,
		ErrStripeAccountUnavailable,
		ErrStripeCapabilityUnavailable,
		ErrWalletNotRechargeable,
		ErrCheckoutIdempotencyConflict,
	}
	for _, candidate := range known {
		if errors.Is(err, candidate) {
			_, summary := reconciliationFailureDetail(candidate)
			return summary
		}
	}
	return boundedText(fallback, 500)
}
