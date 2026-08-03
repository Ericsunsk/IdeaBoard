package stripeadapter

import (
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/payments"

	"github.com/stripe/stripe-go/v86"
)

func TestMapStripeErrorTreatsNotFoundAsDeterministicInvalidResponse(t *testing.T) {
	t.Parallel()

	const rawProviderBody = `{"error":"not found","api_key":"sk_live_must_not_escape"}`
	err := mapStripeError("读取 Stripe Checkout Session", &stripe.Error{
		HTTPStatusCode: 404,
		Msg:            rawProviderBody,
	})

	var gatewayErr *payments.GatewayError
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("error type = %T, want GatewayError", err)
	}
	if gatewayErr.Code() != payments.GatewayErrorInvalidResponse {
		t.Fatalf("error code = %q, want %q", gatewayErr.Code(), payments.GatewayErrorInvalidResponse)
	}
	if gatewayErr.Error() != "Stripe 拒绝了当前配置" {
		t.Fatalf("safe error = %q", gatewayErr.Error())
	}
	if strings.Contains(err.Error(), rawProviderBody) || strings.Contains(err.Error(), "sk_live_must_not_escape") {
		t.Fatalf("gateway error leaked provider body: %q", err)
	}
}
