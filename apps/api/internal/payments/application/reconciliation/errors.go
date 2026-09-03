package reconciliation

import (
	"errors"
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
		return string(paymentports.GatewayErrorProviderUnavailable), "Stripe 服务暂时不可用"
	}
	if errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) {
		return "configuration_invalid", "当前 Stripe 配置无效"
	}
	if errors.Is(err, paymentcontracts.ErrStripeConfigurationNotFound) {
		return "configuration_not_found", "当前 Stripe 配置尚未完成"
	}
	if errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) {
		return "configuration_unavailable", "当前 Stripe 配置无法收敛该订单"
	}
	if errors.Is(err, paymentcontracts.ErrInvalidStripeAPIKey) {
		return string(paymentports.GatewayErrorInvalidKey), "Stripe API Key 无效"
	}
	if errors.Is(err, paymentcontracts.ErrStripePermissionDenied) {
		return string(paymentports.GatewayErrorPermissionDenied), "Stripe API Key 权限不足"
	}
	if errors.Is(err, paymentcontracts.ErrStripeAccountUnavailable) {
		return string(paymentports.GatewayErrorAccountUnavailable), "Stripe 账户不可用"
	}
	if errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable) {
		return string(paymentports.GatewayErrorCapabilityMissing), "Stripe 账户暂不具备收款能力"
	}
	if errors.Is(err, paymentsdomain.ErrWalletNotRechargeable) {
		return "wallet_not_rechargeable", "当前账户暂不可充值"
	}
	if errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyConflict) {
		return "identity_conflict", "Stripe Checkout Session 与订单绑定不一致"
	}
	return "ledger_retry", "账本写入暂时失败"
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
