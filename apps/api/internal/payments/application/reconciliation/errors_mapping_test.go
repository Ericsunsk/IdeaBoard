package reconciliation_test

import (
	"errors"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreconciliation "ideaboard/api/internal/payments/application/reconciliation"
	"testing"

	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

func TestReconciliationFailureDetailUsesStableSafeDeterministicCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		code        string
		summary     string
		isTransient bool
	}{
		{"configuration invalid", paymentcontracts.ErrStripeConfigurationInvalid, "configuration_invalid", "当前 Stripe 配置无效", false},
		{"configuration not found", paymentcontracts.ErrStripeConfigurationNotFound, "configuration_not_found", "当前 Stripe 配置尚未完成", false},
		{"configuration unavailable", paymentsdomain.ErrPurchasesUnavailable, "configuration_unavailable", "当前 Stripe 配置无法收敛该订单", false},
		{"invalid key", paymentcontracts.ErrInvalidStripeAPIKey, string(paymentports.GatewayErrorInvalidKey), "Stripe API Key 无效", false},
		{"permission denied", paymentcontracts.ErrStripePermissionDenied, string(paymentports.GatewayErrorPermissionDenied), "当前 Stripe API Key 权限不足", false},
		{"account unavailable", paymentcontracts.ErrStripeAccountUnavailable, string(paymentports.GatewayErrorAccountUnavailable), "当前 Stripe 账户不可用", false},
		{"capability missing", paymentcontracts.ErrStripeCapabilityUnavailable, string(paymentports.GatewayErrorCapabilityMissing), "当前 Stripe 账户暂不具备收款能力", false},
		{"wallet not rechargeable", paymentsdomain.ErrWalletNotRechargeable, "wallet_not_rechargeable", "当前账户暂不可充值", false},
		{"identity conflict", paymentsdomain.ErrCheckoutIdempotencyConflict, "identity_conflict", "Stripe Checkout Session 与订单绑定不一致", false},
		{"unknown", errors.New(`provider body {"api_key":"sk_live_secret"}`), "ledger_retry", "账本写入失败", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, summary := paymentreconciliation.FailureDetail(test.err)
			if code != test.code || summary != test.summary {
				t.Fatalf("detail = (%q, %q), want (%q, %q)", code, summary, test.code, test.summary)
			}
			if got := paymentreconciliation.FailureIsTransient(test.err); got != test.isTransient {
				t.Fatalf("transient = %t, want %t", got, test.isTransient)
			}
		})
	}
}
