package stripe

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/stripe/stripe-go/v86"
)

func TestMapStripeErrorTreatsNotFoundAsDeterministicInvalidResponse(t *testing.T) {
	t.Parallel()

	const rawProviderBody = `{"error":"not found","api_key":"sk_live_must_not_escape"}`
	err := mapStripeError("读取 Stripe Checkout Session", &stripe.Error{
		HTTPStatusCode: 404,
		Msg:            rawProviderBody,
	})

	var gatewayErr *paymentports.GatewayError
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("error type = %T, want GatewayError", err)
	}
	if gatewayErr.Code() != paymentports.GatewayErrorInvalidResponse {
		t.Fatalf("error code = %q, want %q", gatewayErr.Code(), paymentports.GatewayErrorInvalidResponse)
	}
	if gatewayErr.Error() != "Stripe 拒绝了当前配置" {
		t.Fatalf("safe error = %q", gatewayErr.Error())
	}
	if strings.Contains(err.Error(), rawProviderBody) || strings.Contains(err.Error(), "sk_live_must_not_escape") {
		t.Fatalf("gateway error leaked provider body: %q", err)
	}
}

func TestMapStripeCheckoutCreateErrorPreservesOutcomeCertainty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode paymentports.GatewayErrorCode
	}{
		{
			name: "invalid request was definitively rejected",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusBadRequest,
				Type:           stripe.ErrorTypeInvalidRequest,
			},
			wantCode: paymentports.GatewayErrorRequestRejected,
		},
		{
			name: "idempotency mismatch has an unknown prior outcome",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusBadRequest,
				Type:           stripe.ErrorTypeIdempotency,
			},
			wantCode: paymentports.GatewayErrorInvalidResponse,
		},
		{
			name:     "transport cancellation has an unknown outcome",
			err:      context.Canceled,
			wantCode: paymentports.GatewayErrorProviderUnavailable,
		},
		{
			name: "rate limit has an unknown outcome",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusTooManyRequests,
				Type:           stripe.ErrorTypeRateLimit,
			},
			wantCode: paymentports.GatewayErrorProviderUnavailable,
		},
		{
			name: "provider failure has an unknown outcome",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusInternalServerError,
				Type:           stripe.ErrorTypeAPI,
			},
			wantCode: paymentports.GatewayErrorProviderUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := mapStripeCreateError("创建 Stripe Checkout Session", test.err)
			var gatewayErr *paymentports.GatewayError
			if !errors.As(err, &gatewayErr) || gatewayErr.Code() != test.wantCode {
				t.Fatalf("gateway error = %v, want code %q", err, test.wantCode)
			}
		})
	}
}
