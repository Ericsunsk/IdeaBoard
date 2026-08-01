package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/gin-gonic/gin"
)

func TestStripeConfigurationHTTPContractNeverReturnsSecretsAndRejectsUnknownFields(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_http", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_http", WebhookEndpointID: "we_http", WebhookSecret: "whsec_http",
	}}
	service := NewService(NewRepository(db), gateway, &fakeWebhookVerifier{})
	handler := NewHandler(service, fixedPublicURLProvider{"https://app.example.com"})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/payments/stripe-config", handler.GetStripeConfiguration)
	router.PUT("/admin/payments/stripe-config", handler.UpdateStripeConfiguration)

	requestBody := `{"api_key":"sk_test_http_secret","product_tax_code":"txcd_10105001","purchases_enabled":true}`
	response := performPaymentRequest(router, http.MethodPut, "/admin/payments/stripe-config", requestBody, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "sk_test_http_secret") || strings.Contains(response.Body.String(), "whsec_http") {
		t.Fatalf("PUT leaked secret: %s", response.Body.String())
	}

	response = performPaymentRequest(router, http.MethodGet, "/admin/payments/stripe-config", "", nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "sk_test_http_secret") || strings.Contains(response.Body.String(), "whsec_http") {
		t.Fatalf("GET response=%d %s", response.Code, response.Body.String())
	}

	response = performPaymentRequest(router, http.MethodPut, "/admin/payments/stripe-config", `{"product_tax_code":"txcd_10105001","purchases_enabled":true,"unknown":1}`, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStripeConfigurationHTTPRejectsOversizedAPIKeyBeforeGatewayAndAuditsFailure(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{}
	service := NewService(NewRepository(db), gateway, &fakeWebhookVerifier{})
	handler := NewHandler(service, fixedPublicURLProvider{"https://app.example.com"})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin)
		c.Next()
	})
	router.PUT("/admin/payments/stripe-config", handler.UpdateStripeConfiguration)

	oversizedKey := "sk_test_" + strings.Repeat("x", maxStripeAPIKeyLength-len("sk_test_")+1)
	requestBody, err := json.Marshal(map[string]any{
		"api_key": oversizedKey, "product_tax_code": "txcd_10105001", "purchases_enabled": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := performPaymentRequest(router, http.MethodPut, "/admin/payments/stripe-config", string(requestBody), nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(gateway.calls) != 0 {
		t.Fatalf("gateway calls=%d, want 0", len(gateway.calls))
	}

	var configurationCount, auditCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM payment_stripe_configurations`).Scan(&configurationCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `
SELECT COUNT(*) FROM payment_admin_actions
WHERE outcome = 'failed' AND error_code = 'invalid_key'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if configurationCount != 0 || auditCount != 1 {
		t.Fatalf("configuration count=%d audit count=%d", configurationCount, auditCount)
	}
}

func TestStripeWebhookHTTPContractUsesUnauthenticatedExactRawBody(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_webhook_http", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_webhook_http", WebhookEndpointID: "we_webhook_http", WebhookSecret: "whsec_webhook_http",
	}}
	verifier := &fakeWebhookVerifier{event: StripeWebhookEvent{
		ID: "evt_http", Type: "checkout.session.completed", ObjectID: "cs_http", CreatedAt: time.Now(),
	}}
	service := NewService(NewRepository(db), gateway, verifier)
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_webhook_http", ProductTaxCode: "txcd_10105001",
		WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service, fixedPublicURLProvider{"https://app.example.com"})
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

type fixedPublicURLProvider struct{ baseURL string }

func (p fixedPublicURLProvider) GetPublicURL(_ context.Context) (settings.PublicURLSetting, error) {
	return settings.PublicURLSetting{PublicBaseURL: p.baseURL}, nil
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

func decodePaymentResponseData(t *testing.T, body []byte, destination any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Data, destination); err != nil {
		t.Fatal(err)
	}
}
