package configuration

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentports "ideaboard/api/internal/payments/ports"
)

var (
	productTaxCodePattern               = regexp.MustCompile(`^txcd_[0-9]{8}$`)
	paymentMethodConfigurationIDPattern = regexp.MustCompile(`^pmc_[A-Za-z0-9]+$`)
)

func NormalizeProductTaxCode(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !productTaxCodePattern.MatchString(value) {
		return "", paymentcontracts.ErrInvalidProductTaxCode
	}
	return value, nil
}

func NormalizePaymentMethodConfigurationID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) > 255 || !paymentMethodConfigurationIDPattern.MatchString(value) {
		return "", paymentcontracts.ErrInvalidPaymentMethodConfiguration
	}
	return value, nil
}

func NormalizeWebhookURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", paymentcontracts.ErrInvalidWebhookURL
	}
	if parsed.Path != "/api/webhooks/stripe" {
		return "", paymentcontracts.ErrInvalidWebhookURL
	}
	hostname := strings.ToLower(parsed.Hostname())
	isLoopback := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopback) {
		return "", paymentcontracts.ErrInvalidWebhookURL
	}
	return parsed.String(), nil
}

func ValidatePreparedConfiguration(prepared paymentports.PreparedStripeConfiguration, expectedMode paymentports.StripeMode, expectedPaymentMethodConfigurationID string) error {
	if strings.TrimSpace(prepared.AccountID) == "" || prepared.Mode != expectedMode ||
		prepared.PaymentMethodConfigurationID != expectedPaymentMethodConfigurationID ||
		strings.TrimSpace(prepared.ProductID) == "" || strings.TrimSpace(prepared.WebhookEndpointID) == "" ||
		strings.TrimSpace(prepared.WebhookSecret) == "" {
		return paymentcontracts.ErrStripeConfigurationInvalid
	}
	if !prepared.ChargesEnabled {
		return paymentcontracts.ErrStripeCapabilityUnavailable
	}
	return nil
}

func GatewayFailure(err error) (paymentports.GatewayErrorCode, string, error) {
	var gatewayErr *paymentports.GatewayError
	if !errors.As(err, &gatewayErr) {
		return paymentports.GatewayErrorProviderUnavailable, "Stripe 服务暂时不可用", paymentcontracts.ErrStripeProviderUnavailable
	}
	summary := gatewayErr.Error()
	if summary == "" {
		summary = "Stripe operation failed"
	}
	switch gatewayErr.Code() {
	case paymentports.GatewayErrorInvalidKey:
		return gatewayErr.Code(), summary, paymentcontracts.ErrInvalidStripeAPIKey
	case paymentports.GatewayErrorPermissionDenied:
		return gatewayErr.Code(), summary, paymentcontracts.ErrStripePermissionDenied
	case paymentports.GatewayErrorAccountUnavailable:
		return gatewayErr.Code(), summary, paymentcontracts.ErrStripeAccountUnavailable
	case paymentports.GatewayErrorCapabilityMissing:
		return gatewayErr.Code(), summary, paymentcontracts.ErrStripeCapabilityUnavailable
	case paymentports.GatewayErrorInvalidResponse:
		return gatewayErr.Code(), summary, paymentcontracts.ErrStripeConfigurationInvalid
	default:
		return paymentports.GatewayErrorProviderUnavailable, summary, paymentcontracts.ErrStripeProviderUnavailable
	}
}

func WrapPublicError(public error, summary string) error {
	if strings.TrimSpace(summary) == "" {
		return public
	}
	return fmt.Errorf("%w: %s", public, summary)
}
