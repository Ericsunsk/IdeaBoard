package reconciliation_test

import (
	"errors"
	"strings"
	"testing"

	paymentports "ideaboard/api/internal/payments/ports"

	"ideaboard/api/internal/payments/application/reconciliation"
)

func TestOperationalErrorSummaryNeverPersistsUnknownErrorDetails(t *testing.T) {
	const secret = "sk_live_must_not_escape"
	const payload = `{"billing_details":{"address":"private"}}`

	summary := reconciliation.OperationalErrorSummary(errors.New(secret+" "+payload+"\ninternal stack"), "Stripe operation failed")
	if summary != "Stripe operation failed" {
		t.Fatalf("summary = %q, want fixed fallback", summary)
	}
	if strings.Contains(summary, secret) || strings.Contains(summary, payload) || strings.Contains(summary, "stack") {
		t.Fatalf("summary leaked operational error details: %q", summary)
	}
}

func TestOperationalErrorSummaryKeepsOnlyTypedSafeGatewayMessage(t *testing.T) {
	err := paymentports.NewGatewayError(paymentports.GatewayErrorPermissionDenied, "Stripe API Key 权限不足", errors.New("rk_live_secret"))
	if got := reconciliation.OperationalErrorSummary(err, "fallback"); got != "Stripe API Key 权限不足" {
		t.Fatalf("summary = %q", got)
	}
}
