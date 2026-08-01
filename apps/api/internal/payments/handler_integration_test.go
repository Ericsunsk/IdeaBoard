package payments

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
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
	service := NewService(ServiceDependencies{Repository: NewRepository(db), ConfigurationGateway: gateway, WebhookVerifier: &fakeWebhookVerifier{}})
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

func TestUserCheckoutHTTPUsesPrincipalAndTrustedReturnURL(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configuredFakeStripeGateway("acct_user_http", "prod_user_http"), checkoutGateway, credits.NewService(pool))
	verifier := &fakeWebhookVerifier{}
	service.verifier = verifier
	handler := NewHandler(service, fixedPublicURLProvider{"https://configured.example.com"})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.GET("/user/payments/purchase-options", handler.GetPurchaseOptions)
	router.POST("/user/payments/checkout-sessions", handler.CreateCheckoutSession)
	router.GET("/user/payments/orders/:public_token", handler.GetPaymentOrderReturnStatus)
	router.GET("/user/payments/recharges", handler.ListRecharges)
	router.GET("/user/payments/events", handler.StreamUserPaymentEvents)
	router.POST("/api/webhooks/stripe", handler.ReceiveStripeWebhook)

	options := performPaymentRequest(router, http.MethodGet, "/user/payments/purchase-options", "", nil)
	if options.Code != http.StatusOK || options.Header().Get("Cache-Control") != "no-store" || !strings.Contains(options.Body.String(), `"purchases_available":true`) {
		t.Fatalf("purchase options=%d %s", options.Code, options.Body.String())
	}

	for _, quantity := range []int{499, 500_001} {
		invalid := performPaymentRequest(router, http.MethodPost, "/user/payments/checkout-sessions", fmt.Sprintf(`{
"idempotency_key":"user-http-invalid-%d","idea_quantity":%d,"terms_accepted":true,"terms_version":"2026-08-01","refund_policy_version":"2026-08-01"}`, quantity, quantity), nil)
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("invalid quantity %d response=%d %s", quantity, invalid.Code, invalid.Body.String())
		}
	}

	checkout := performPaymentRequest(router, http.MethodPost, "/user/payments/checkout-sessions", `{
"idempotency_key":"user-http-checkout-1","idea_quantity":500,"terms_accepted":true,"terms_version":"2026-08-01","refund_policy_version":"2026-08-01"}`, nil)
	if checkout.Code != http.StatusOK || checkout.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("checkout=%d %s", checkout.Code, checkout.Body.String())
	}
	if !strings.HasPrefix(checkoutGateway.lastCreateSession.SuccessURL, "https://configured.example.com/user/boards?") {
		t.Fatalf("checkout success URL = %q", checkoutGateway.lastCreateSession.SuccessURL)
	}
	var result CheckoutResult
	decodePaymentResponseData(t, checkout.Body.Bytes(), &result)

	unknown := performPaymentRequest(router, http.MethodPost, "/user/payments/checkout-sessions", `{"idempotency_key":"user-http-checkout-2","idea_quantity":501,"terms_accepted":true,"terms_version":"2026-08-01","refund_policy_version":"2026-08-01","unknown":true}`, nil)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown request=%d %s", unknown.Code, unknown.Body.String())
	}

	status := performPaymentRequest(router, http.MethodGet, "/user/payments/orders/"+result.PublicToken, "", nil)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"processing"`) {
		t.Fatalf("status=%d %s", status.Code, status.Body.String())
	}

	server := httptest.NewServer(router)
	defer server.Close()
	streamContext, cancelStream := context.WithCancel(t.Context())
	defer cancelStream()
	streamEvent := make(chan string, 1)
	streamError := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(streamContext, http.MethodGet, server.URL+"/user/payments/events", nil)
		if err != nil {
			streamError <- err
			return
		}
		response, err := server.Client().Do(request)
		if err != nil {
			streamError <- err
			return
		}
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				streamEvent <- strings.TrimPrefix(scanner.Text(), "data: ")
				return
			}
		}
		streamError <- scanner.Err()
	}()
	waitForPaymentSubscriber(t, service.EventHub(), 42)
	checkoutGateway.paid = true
	verifier.event = StripeWebhookEvent{
		ID: "evt_user_http_paid", Type: "checkout.session.completed",
		ObjectID: checkoutGateway.createdSession.ID, CreatedAt: service.now().UTC(),
	}
	webhook := performPaymentRequest(router, http.MethodPost, "/api/webhooks/stripe", `{"id":"evt_user_http_paid"}`, map[string]string{"Stripe-Signature": "valid"})
	if webhook.Code != http.StatusOK {
		t.Fatalf("paid webhook=%d %s", webhook.Code, webhook.Body.String())
	}
	select {
	case event := <-streamEvent:
		if !strings.Contains(event, `"public_token":"`+result.PublicToken+`"`) || !strings.Contains(event, `"status":"credited"`) {
			t.Fatalf("payment SSE event=%s", event)
		}
	case err := <-streamError:
		t.Fatalf("payment SSE stream failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for committed payment SSE event")
	}

	history := performPaymentRequest(router, http.MethodGet, "/user/payments/recharges", "", nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"idea_quantity":500`) || !strings.Contains(history.Body.String(), `"status":"credited"`) {
		t.Fatalf("credited history=%d %s", history.Code, history.Body.String())
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 501 {
		t.Fatalf("credited Wallet=%+v error=%v", wallet, err)
	}
}

func waitForPaymentSubscriber(t *testing.T, hub *EventHub, userID int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		count := len(hub.subscribers[userID])
		hub.mu.RUnlock()
		if count == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("payment SSE subscriber was not ready")
}

func TestUserCheckoutHTTPRejectsWalletWithoutRechargeEligibility(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	service := newConfiguredPurchaseService(
		t, db, configuredFakeStripeGateway("acct_locked_http", "prod_locked_http"),
		&fakeCheckoutGateway{}, credits.NewService(pool),
	)
	handler := NewHandler(service, fixedPublicURLProvider{"https://configured.example.com"})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 88, principal.RoleUser)
		c.Next()
	})
	router.POST("/user/payments/checkout-sessions", handler.CreateCheckoutSession)

	response := performPaymentRequest(router, http.MethodPost, "/user/payments/checkout-sessions", `{
"idempotency_key":"user-http-no-wallet","idea_quantity":500000,"terms_accepted":true,"terms_version":"2026-08-01","refund_policy_version":"2026-08-01"}`, nil)
	if response.Code != http.StatusLocked {
		t.Fatalf("non-rechargeable Wallet response=%d %s", response.Code, response.Body.String())
	}
}

func TestStripeConfigurationHTTPRejectsOversizedAPIKeyBeforeGatewayAndAuditsFailure(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{}
	service := NewService(ServiceDependencies{Repository: NewRepository(db), ConfigurationGateway: gateway, WebhookVerifier: &fakeWebhookVerifier{}})
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
		ID: "evt_http", Type: "customer.updated", ObjectID: "cus_http", CreatedAt: time.Now(),
	}}
	service := NewService(ServiceDependencies{Repository: NewRepository(db), ConfigurationGateway: gateway, WebhookVerifier: verifier})
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
