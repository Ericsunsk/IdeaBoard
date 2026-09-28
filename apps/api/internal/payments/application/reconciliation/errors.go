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
		return string(paymentports.GatewayErrorProviderUnavailable), msgFailureDetail
	}
	if errors.Is(err, paymentcontracts.ErrStripeConfigurationInvalid) {
		return "configuration_invalid", msgFailureDetail2
	}
	if errors.Is(err, paymentcontracts.ErrStripeConfigurationNotFound) {
		return "configuration_not_found", msgFailureDetail3
	}
	if errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) {
		return "configuration_unavailable", msgFailureDetail4
	}
	if errors.Is(err, paymentcontracts.ErrInvalidStripeAPIKey) {
		return string(paymentports.GatewayErrorInvalidKey), msgFailureDetail5
	}
	if errors.Is(err, paymentcontracts.ErrStripePermissionDenied) {
		return string(paymentports.GatewayErrorPermissionDenied), msgFailureDetail6
	}
	if errors.Is(err, paymentcontracts.ErrStripeAccountUnavailable) {
		return string(paymentports.GatewayErrorAccountUnavailable), msgFailureDetail7
	}
	if errors.Is(err, paymentcontracts.ErrStripeCapabilityUnavailable) {
		return string(paymentports.GatewayErrorCapabilityMissing), msgFailureDetail8
	}
	if errors.Is(err, paymentsdomain.ErrWalletNotRechargeable) {
		return "wallet_not_rechargeable", msgFailureDetail9
	}
	if errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyConflict) {
		return "identity_conflict", msgFailureDetail10
	}
	return "ledger_retry", msgFailureDetail11
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

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgFailureDetail   = "Stripe 服务暂不可用"                    // FailureDetail @ errors.go
	msgFailureDetail2  = "当前 Stripe 配置无效"                   // FailureDetail @ errors.go
	msgFailureDetail3  = "当前 Stripe 配置尚未完成"                 // FailureDetail @ errors.go
	msgFailureDetail4  = "当前 Stripe 配置无法收敛该订单"              // FailureDetail @ errors.go
	msgFailureDetail5  = "Stripe API Key 无效"                // FailureDetail @ errors.go
	msgFailureDetail6  = "当前 Stripe API Key 权限不足"           // FailureDetail @ errors.go
	msgFailureDetail7  = "当前 Stripe 账户不可用"                  // FailureDetail @ errors.go
	msgFailureDetail8  = "当前 Stripe 账户暂不具备收款能力"             // FailureDetail @ errors.go
	msgFailureDetail9  = "当前账户暂不可充值"                        // FailureDetail @ errors.go
	msgFailureDetail10 = "Stripe Checkout Session 与订单绑定不一致" // FailureDetail @ errors.go
	msgFailureDetail11 = "账本写入失败"                           // FailureDetail @ errors.go
)
