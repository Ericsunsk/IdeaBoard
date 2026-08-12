package stripeadapter

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/payments"
)

func TestPrepareConfigurationBindsActivePaymentMethodConfiguration(t *testing.T) {
	t.Parallel()

	var retrievedPaymentMethodConfiguration bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/account":
			_, _ = io.WriteString(writer, `{"id":"acct_idea","charges_enabled":true}`)
		case "/v1/payment_method_configurations/pmc_idea":
			retrievedPaymentMethodConfiguration = true
			_, _ = io.WriteString(writer, `{"id":"pmc_idea","object":"payment_method_configuration","active":true,"livemode":false}`)
		case "/v1/products":
			if request.Method == http.MethodGet {
				_, _ = io.WriteString(writer, `{"object":"list","data":[],"has_more":false}`)
				return
			}
			_, _ = io.WriteString(writer, `{"id":"prod_idea","active":true,"livemode":false}`)
		case "/v1/webhook_endpoints":
			_, _ = io.WriteString(writer, `{"id":"we_idea","secret":"whsec_idea","livemode":false}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	prepared, err := newCheckoutTestAdapter(server.URL).PrepareConfiguration(t.Context(), payments.PrepareStripeConfiguration{
		APIKey: "sk_test_fixture", Mode: payments.StripeModeTest, PurchasesEnabled: true,
		PaymentMethodConfigurationID: "pmc_idea", ProductTaxCode: "txcd_10105001",
		WebhookURL: "https://api.ideaboard.test/api/webhooks/stripe",
	})
	if err != nil {
		t.Fatalf("prepare Stripe configuration: %v", err)
	}
	if !retrievedPaymentMethodConfiguration {
		t.Fatal("PrepareConfiguration did not retrieve the configured Payment Method Configuration")
	}
	if prepared.PaymentMethodConfigurationID != "pmc_idea" {
		t.Fatalf("Payment Method Configuration ID = %q", prepared.PaymentMethodConfigurationID)
	}
}

func TestPrepareConfigurationRejectsUnusablePaymentMethodConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     payments.StripeMode
		response string
	}{
		{name: "inactive", mode: payments.StripeModeTest, response: `{"id":"pmc_idea","active":false,"livemode":false}`},
		{name: "wrong mode", mode: payments.StripeModeLive, response: `{"id":"pmc_idea","active":true,"livemode":false}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/v1/account":
					_, _ = io.WriteString(writer, `{"id":"acct_idea","charges_enabled":true}`)
				case "/v1/payment_method_configurations/pmc_idea":
					_, _ = io.WriteString(writer, test.response)
				default:
					t.Fatalf("unexpected Stripe request after invalid configuration: %s", request.URL.Path)
				}
			}))
			defer server.Close()

			_, err := newCheckoutTestAdapter(server.URL).PrepareConfiguration(t.Context(), payments.PrepareStripeConfiguration{
				APIKey: "sk_test_fixture", Mode: test.mode, PaymentMethodConfigurationID: "pmc_idea",
			})
			var gatewayErr *payments.GatewayError
			if !errors.As(err, &gatewayErr) || gatewayErr.Code() != payments.GatewayErrorInvalidResponse {
				t.Fatalf("error = %v, want invalid Stripe response", err)
			}
		})
	}
}
