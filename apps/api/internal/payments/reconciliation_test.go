package payments

import (
	"errors"
	"testing"
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
		{"configuration invalid", ErrStripeConfigurationInvalid, "configuration_invalid", "当前 Stripe 配置无效", false},
		{"configuration not found", ErrStripeConfigurationNotFound, "configuration_not_found", "当前 Stripe 配置尚未完成", false},
		{"configuration unavailable", ErrPurchasesUnavailable, "configuration_unavailable", "当前 Stripe 配置无法收敛该订单", false},
		{"invalid key", ErrInvalidStripeAPIKey, string(GatewayErrorInvalidKey), "Stripe API Key 无效", false},
		{"permission denied", ErrStripePermissionDenied, string(GatewayErrorPermissionDenied), "Stripe API Key 权限不足", false},
		{"account unavailable", ErrStripeAccountUnavailable, string(GatewayErrorAccountUnavailable), "Stripe 账户不可用", false},
		{"capability missing", ErrStripeCapabilityUnavailable, string(GatewayErrorCapabilityMissing), "Stripe 账户暂不具备收款能力", false},
		{"wallet not rechargeable", ErrWalletNotRechargeable, "wallet_not_rechargeable", "当前账户暂不可充值", false},
		{"identity conflict", ErrCheckoutIdempotencyConflict, "identity_conflict", "Stripe Checkout Session 与订单绑定不一致", false},
		{"unknown", errors.New(`provider body {"api_key":"sk_live_secret"}`), "ledger_retry", "账本写入暂时失败", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, summary := reconciliationFailureDetail(test.err)
			if code != test.code || summary != test.summary {
				t.Fatalf("detail = (%q, %q), want (%q, %q)", code, summary, test.code, test.summary)
			}
			if got := reconciliationFailureIsTransient(test.err); got != test.isTransient {
				t.Fatalf("transient = %t, want %t", got, test.isTransient)
			}
		})
	}
}
