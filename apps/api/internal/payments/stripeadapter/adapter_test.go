package stripeadapter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/payments"

	"github.com/stripe/stripe-go/v86"
)

func TestPrepareConfigurationBlocksLivePurchasesWithoutActiveTaxRegistration(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/account":
			_, _ = io.WriteString(writer, `{"id":"acct_live","charges_enabled":true}`)
		case "/v1/tax/registrations":
			if got := request.URL.Query().Get("status"); got != "active" {
				t.Fatalf("registration status filter = %q", got)
			}
			_, _ = io.WriteString(writer, `{"object":"list","data":[],"has_more":false}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := newCheckoutTestAdapter(server.URL).PrepareConfiguration(context.Background(), payments.PrepareStripeConfiguration{
		APIKey: "rk_live_fixture", Mode: payments.StripeModeLive, PurchasesEnabled: true,
		ProductTaxCode: "txcd_10105001", WebhookURL: "https://app.example.com/api/webhooks/stripe",
	})
	var gatewayErr *payments.GatewayError
	if !errors.As(err, &gatewayErr) || gatewayErr.Code() != payments.GatewayErrorCapabilityMissing {
		t.Fatalf("PrepareConfiguration() error = %v, want capability missing", err)
	}
	if !strings.Contains(gatewayErr.Error(), "Tax Registration") {
		t.Fatalf("safe error = %q, want Tax Registration guidance", gatewayErr.Error())
	}
}

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
