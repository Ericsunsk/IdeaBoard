package tests

import (
	payments "ideaboard/api/internal/payments"

	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/gin-gonic/gin"
)

func TestStripeWebhookHTTPContractUsesUnauthenticatedExactRawBody(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: paymentports.PreparedStripeConfiguration{
		AccountID: "acct_webhook_http", Mode: paymentports.StripeModeTest, ChargesEnabled: true,
		PaymentMethodConfigurationID: "pmc_testfixture", ProductID: "prod_webhook_http", WebhookEndpointID: "we_webhook_http", WebhookSecret: "whsec_webhook_http",
	}}
	verifier := &fakeWebhookVerifier{event: paymentports.StripeWebhookEvent{
		ID: "evt_http", Type: "customer.updated", ObjectID: "cus_http", CreatedAt: time.Now(),
	}}
	repo := paymentpostgres.NewRepository(db)
	service := newCompleteTestPaymentsService(t, repo, payments.NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, verifier, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_webhook_http", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}
	handler := paymenthttp.NewWebhookHandler(service)
	router := gin.New()
	router.POST("/api/webhooks/stripe", handler.ReceiveStripeWebhook)

	raw := `{"id":"evt_http", "data":{"object":{"id":"cs_http"}}}`
	response := performPaymentRequest(router, http.MethodPost, "/api/webhooks/stripe", raw, map[string]string{"Stripe-Signature": "valid"})
	if response.Code != http.StatusOK {
		t.Fatalf("webhook status=%d body=%s", response.Code, response.Body.String())
	}
	if string(verifier.raw) != raw {
		t.Fatalf("webhook body changed: got %q want %q", verifier.raw, raw)
	}
}

func performPaymentRequest(router http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}
