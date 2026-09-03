package stripe_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/payments/infrastructure/stripe"
	paymentports "ideaboard/api/internal/payments/ports"
)

func TestStripeTestModeSmoke(t *testing.T) {
	if os.Getenv("API_TEST_STRIPE_SMOKE") != "1" {
		t.Skip("set API_TEST_STRIPE_SMOKE=1 to run the Stripe Test Mode smoke")
	}
	apiKey := strings.TrimSpace(os.Getenv("API_TEST_STRIPE_SECRET_KEY"))
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEST_STRIPE_BASE_URL")), "/")
	taxCode := strings.TrimSpace(os.Getenv("API_TEST_STRIPE_TAX_CODE"))
	paymentMethodConfigurationID := strings.TrimSpace(os.Getenv("API_TEST_STRIPE_PAYMENT_METHOD_CONFIGURATION_ID"))
	if apiKey == "" || baseURL == "" || taxCode == "" || paymentMethodConfigurationID == "" {
		t.Fatal("API_TEST_STRIPE_SECRET_KEY, API_TEST_STRIPE_BASE_URL, API_TEST_STRIPE_TAX_CODE, and API_TEST_STRIPE_PAYMENT_METHOD_CONFIGURATION_ID are required")
	}
	mode, err := paymentports.ClassifyStripeAPIKey(apiKey)
	if err != nil || mode != paymentports.StripeModeTest {
		t.Fatal("API_TEST_STRIPE_SECRET_KEY must be a Test Mode sk_test_* or rk_test_* Key")
	}

	adapter := stripe.New()
	prepared, err := adapter.PrepareConfiguration(t.Context(), paymentports.PrepareStripeConfiguration{
		APIKey: apiKey, Mode: paymentports.StripeModeTest, PurchasesEnabled: true,
		ProductTaxCode: taxCode, PaymentMethodConfigurationID: paymentMethodConfigurationID,
		WebhookURL: baseURL + "/api/webhooks/stripe",
	})
	if err != nil {
		t.Fatalf("prepare Stripe Test Mode configuration: %v", err)
	}
	if prepared.AccountID == "" || prepared.PaymentMethodConfigurationID != paymentMethodConfigurationID || prepared.ProductID == "" || prepared.WebhookEndpointID == "" || prepared.WebhookSecret == "" {
		t.Fatalf("Stripe Test Mode configuration is incomplete: account=%t payment_method_configuration=%t product=%t webhook=%t secret=%t",
			prepared.AccountID != "", prepared.PaymentMethodConfigurationID == paymentMethodConfigurationID,
			prepared.ProductID != "", prepared.WebhookEndpointID != "", prepared.WebhookSecret != "")
	}

	const paymentOrderID = "1db90f9a-c836-4ef7-b449-6f58457d8f79"
	const requestHash = "c8ea3f9d5e50ee5151ff8535c87822384f1ea2d3fb092b6795d223244e0f9068"
	customer, err := adapter.CreateCustomer(t.Context(), paymentports.CreateCheckoutCustomerInput{
		APIKey: apiKey, UserID: 9_000_000_001, IdempotencyKey: "ideaboard-stripe-smoke-customer-v1",
	})
	if err != nil {
		t.Fatalf("create Stripe Test Mode customer: %v", err)
	}
	expiresAt := time.Now().UTC().Truncate(time.Second).Add(31 * time.Minute)
	session, err := adapter.CreateCheckoutSession(t.Context(), paymentports.CreateCheckoutSessionInput{
		APIKey: apiKey, PaymentMethodConfigurationID: prepared.PaymentMethodConfigurationID,
		CustomerID: customer.ID, ProductID: prepared.ProductID,
		PublicOrderNumber: "IBP-SMOKE-TEST-MODE", PaymentOrderID: paymentOrderID,
		RequestHash: requestHash, IdeaQuantity: 100, AmountMinor: 100,
		ExpiresAt: expiresAt, SuccessURL: baseURL + "/user/boards?stripe_smoke=success",
		CancelURL: baseURL + "/user/boards?stripe_smoke=cancel", IdempotencyKey: "ideaboard-stripe-smoke-checkout-v1",
	})
	if err != nil {
		t.Fatalf("create Stripe Test Mode Checkout Session: %v", err)
	}
	if session.ID == "" || session.URL == "" || !session.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("Stripe Test Mode Checkout Session is incomplete: id=%t url=%t expires_at=%s",
			session.ID != "", session.URL != "", session.ExpiresAt)
	}
	t.Cleanup(func() {
		if _, expireErr := adapter.ExpireCheckoutSession(t.Context(), paymentports.ExpireCheckoutSessionInput{
			APIKey: apiKey, SessionID: session.ID,
		}); expireErr != nil {
			t.Logf("expire Stripe Test Mode Checkout Session %q: %v", session.ID, expireErr)
		}
	})
}
