package stripeadapter

import (
	"context"
	"errors"
	"net/http"
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

func TestMapStripeCheckoutCreateErrorPreservesOutcomeCertainty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode payments.GatewayErrorCode
	}{
		{
			name: "invalid request was definitively rejected",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusBadRequest,
				Type:           stripe.ErrorTypeInvalidRequest,
			},
			wantCode: payments.GatewayErrorRequestRejected,
		},
		{
			name: "idempotency mismatch has an unknown prior outcome",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusBadRequest,
				Type:           stripe.ErrorTypeIdempotency,
			},
			wantCode: payments.GatewayErrorInvalidResponse,
		},
		{
			name:     "transport cancellation has an unknown outcome",
			err:      context.Canceled,
			wantCode: payments.GatewayErrorProviderUnavailable,
		},
		{
			name: "rate limit has an unknown outcome",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusTooManyRequests,
				Type:           stripe.ErrorTypeRateLimit,
			},
			wantCode: payments.GatewayErrorProviderUnavailable,
		},
		{
			name: "provider failure has an unknown outcome",
			err: &stripe.Error{
				HTTPStatusCode: http.StatusInternalServerError,
				Type:           stripe.ErrorTypeAPI,
			},
			wantCode: payments.GatewayErrorProviderUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := mapStripeCreateError("创建 Stripe Checkout Session", test.err)
			var gatewayErr *payments.GatewayError
			if !errors.As(err, &gatewayErr) || gatewayErr.Code() != test.wantCode {
				t.Fatalf("gateway error = %v, want code %q", err, test.wantCode)
			}
		})
	}
}
