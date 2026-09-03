package payments

import (
	"bytes"
	"encoding/json"
	"fmt"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/gin-gonic/gin"
)

func TestStripeConfigurationHTTPContractNeverReturnsSecretsAndRejectsUnknownFields(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: paymentports.PreparedStripeConfiguration{
		AccountID: "acct_http", Mode: paymentports.StripeModeTest, ChargesEnabled: true,
		PaymentMethodConfigurationID: "pmc_testfixture", ProductID: "prod_http", WebhookEndpointID: "we_http", WebhookSecret: "whsec_http",
	}}
	repo := paymentpostgres.NewRepository(db)
	service := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, &fakeWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	handler := paymenthttp.NewAdminHandler(service, testPublicOrigins("https://app.example.com"))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/payments/stripe-config", handler.GetStripeConfiguration)
	router.PUT("/admin/payments/stripe-config", handler.UpdateStripeConfiguration)

	response := performPaymentRequest(router, http.MethodGet, "/admin/payments/stripe-config", "", nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "payment_method_configuration_id") {
		t.Fatalf("unconfigured GET response=%d %s", response.Code, response.Body.String())
	}

	response = performPaymentRequest(router, http.MethodPut, "/admin/payments/stripe-config", `{"api_key":"sk_test_http_secret","product_tax_code":"txcd_10105001","purchases_enabled":true}`, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing Payment Method Configuration status=%d body=%s", response.Code, response.Body.String())
	}

	requestBody := `{"api_key":"sk_test_http_secret","payment_method_configuration_id":"pmc_testfixture","product_tax_code":"txcd_10105001","purchases_enabled":true}`
	response = performPaymentRequest(router, http.MethodPut, "/admin/payments/stripe-config", requestBody, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "sk_test_http_secret") || strings.Contains(response.Body.String(), "whsec_http") {
		t.Fatalf("PUT leaked secret: %s", response.Body.String())
	}
	if len(gateway.calls) != 1 || gateway.calls[0].WebhookURL != "https://api.example.com/api/webhooks/stripe" {
		t.Fatalf("Stripe webhook URL = %q, want API origin", gateway.calls[0].WebhookURL)
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
	userHandler := paymenthttp.NewUserCheckoutHandler(service, testPublicOrigins("https://configured.example.com"))
	webhookHandler := paymenthttp.NewWebhookHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.GET("/user/payments/purchase-options", userHandler.GetPurchaseOptions)
	router.POST("/user/payments/checkout-sessions", userHandler.CreateCheckoutSession)
	router.GET("/user/payments/orders/:public_token", userHandler.GetPaymentOrderReturnStatus)
	router.GET("/user/payments/recharges", userHandler.ListRecharges)
	router.POST("/api/webhooks/stripe", webhookHandler.ReceiveStripeWebhook)

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
	var result paymentcontracts.CheckoutResult
	decodePaymentResponseData(t, checkout.Body.Bytes(), &result)

	unknown := performPaymentRequest(router, http.MethodPost, "/user/payments/checkout-sessions", `{"idempotency_key":"user-http-checkout-2","idea_quantity":501,"terms_accepted":true,"terms_version":"2026-08-01","refund_policy_version":"2026-08-01","unknown":true}`, nil)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown request=%d %s", unknown.Code, unknown.Body.String())
	}

	status := performPaymentRequest(router, http.MethodGet, "/user/payments/orders/"+result.PublicToken, "", nil)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"processing"`) {
		t.Fatalf("status=%d %s", status.Code, status.Body.String())
	}

	paymentEvents, unsubscribe := subscribePaymentUserEvents(t, service.eventHub(), 42)
	defer unsubscribe()
	checkoutGateway.paid = true
	verifier.event = paymentports.StripeWebhookEvent{
		ID: "evt_user_http_paid", Type: "checkout.session.completed",
		ObjectID: checkoutGateway.createdSession.ID, CreatedAt: service.runtime.now().UTC(),
	}
	webhook := performPaymentRequest(router, http.MethodPost, "/api/webhooks/stripe", `{"id":"evt_user_http_paid"}`, map[string]string{"Stripe-Signature": "valid"})
	if webhook.Code != http.StatusOK {
		t.Fatalf("paid webhook=%d %s", webhook.Code, webhook.Body.String())
	}
	select {
	case rawEvent := <-paymentEvents:
		event := decodePaymentUserEvent(t, rawEvent)
		if event.PublicToken != result.PublicToken || event.Status != "credited" {
			t.Fatalf("payment event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for committed payment event")
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

func TestUserPendingPaymentOrderHTTPBoundaryScopesOwnershipAndRejectsActionBodies(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(
		t, db, configuredFakeStripeGateway("acct_pending_http", "prod_pending_http"), checkoutGateway, credits.NewService(pool),
	)
	handler := paymenthttp.NewUserCheckoutHandler(service, testPublicOrigins("https://configured.example.com"))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID := int64(42)
		if c.GetHeader("X-Test-Other-User") == "true" {
			userID = 99
		}
		testsupport.SetPrincipal(t, c, userID, principal.RoleUser)
		c.Next()
	})
	router.POST("/user/payments/checkout-sessions", handler.CreateCheckoutSession)
	router.GET("/user/payments/orders/:public_token", handler.GetPaymentOrderReturnStatus)
	router.GET("/user/payments/pending-orders", handler.ListPendingOrders)
	router.POST("/user/payments/pending-orders/:public_token/continue", handler.ContinuePendingOrder)
	router.POST("/user/payments/pending-orders/:public_token/abandon", handler.AbandonPendingOrder)

	checkout := performPaymentRequest(router, http.MethodPost, "/user/payments/checkout-sessions", `{
"idempotency_key":"user-pending-http-checkout","idea_quantity":500,"terms_accepted":true,"terms_version":"2026-08-01","refund_policy_version":"2026-08-01"}`, nil)
	if checkout.Code != http.StatusOK {
		t.Fatalf("checkout=%d %s", checkout.Code, checkout.Body.String())
	}
	var created paymentcontracts.CheckoutResult
	decodePaymentResponseData(t, checkout.Body.Bytes(), &created)

	pending := performPaymentRequest(router, http.MethodGet, "/user/payments/pending-orders", "", nil)
	if pending.Code != http.StatusOK || pending.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("pending list=%d %s", pending.Code, pending.Body.String())
	}
	var page paymentcontracts.PendingPaymentOrderList
	decodePaymentResponseData(t, pending.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Items[0].PublicToken != created.PublicToken || page.Items[0].CheckoutURL == "" || page.Items[0].Status != string(paymentsdomain.PaymentOrderCheckoutOpen) {
		t.Fatalf("pending page=%+v", page)
	}

	continued := performPaymentRequest(router, http.MethodPost, "/user/payments/pending-orders/"+created.PublicToken+"/continue", "", nil)
	if continued.Code != http.StatusOK || continued.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("continue=%d %s", continued.Code, continued.Body.String())
	}
	var continuedOrder paymentcontracts.PendingPaymentOrder
	decodePaymentResponseData(t, continued.Body.Bytes(), &continuedOrder)
	if continuedOrder.PublicToken != created.PublicToken || continuedOrder.CheckoutURL == "" || continuedOrder.Status != string(paymentsdomain.PaymentOrderCheckoutOpen) {
		t.Fatalf("continued order=%+v", continuedOrder)
	}

	unknownActionPayload := performPaymentRequest(router, http.MethodPost, "/user/payments/pending-orders/"+created.PublicToken+"/abandon", `{"unknown":true}`, nil)
	if unknownActionPayload.Code != http.StatusBadRequest {
		t.Fatalf("unknown action payload=%d %s", unknownActionPayload.Code, unknownActionPayload.Body.String())
	}

	for _, path := range []string{
		"/user/payments/orders/not-a-token",
		"/user/payments/pending-orders/not-a-token/continue",
		"/user/payments/pending-orders/not-a-token/abandon",
	} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/continue") || strings.HasSuffix(path, "/abandon") {
			method = http.MethodPost
		}
		response := performPaymentRequest(router, method, path, "", nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("invalid token %s=%d %s", path, response.Code, response.Body.String())
		}
	}

	for _, path := range []string{
		"/user/payments/orders/" + created.PublicToken,
		"/user/payments/pending-orders/" + created.PublicToken + "/continue",
		"/user/payments/pending-orders/" + created.PublicToken + "/abandon",
	} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/continue") || strings.HasSuffix(path, "/abandon") {
			method = http.MethodPost
		}
		response := performPaymentRequest(router, method, path, "", map[string]string{"X-Test-Other-User": "true"})
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-user %s=%d %s", path, response.Code, response.Body.String())
		}
	}

	abandoned := performPaymentRequest(router, http.MethodPost, "/user/payments/pending-orders/"+created.PublicToken+"/abandon", "", nil)
	if abandoned.Code != http.StatusOK {
		t.Fatalf("abandon=%d %s", abandoned.Code, abandoned.Body.String())
	}
	var abandonedOrder paymentcontracts.PendingPaymentOrder
	decodePaymentResponseData(t, abandoned.Body.Bytes(), &abandonedOrder)
	if abandonedOrder.Status != string(paymentsdomain.PaymentOrderCancelled) || abandonedOrder.CheckoutURL != "" {
		t.Fatalf("abandoned order=%+v", abandonedOrder)
	}

	pending = performPaymentRequest(router, http.MethodGet, "/user/payments/pending-orders", "", nil)
	if pending.Code != http.StatusOK {
		t.Fatalf("pending after abandon=%d %s", pending.Code, pending.Body.String())
	}
	decodePaymentResponseData(t, pending.Body.Bytes(), &page)
	if len(page.Items) != 0 {
		t.Fatalf("pending items after abandon=%+v", page.Items)
	}
}

func TestUserCheckoutHTTPRejectsWalletWithoutRechargeEligibility(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	if _, err := db.ExecContext(t.Context(), `UPDATE credit_wallets SET status = 'frozen' WHERE user_id = 88`); err != nil {
		t.Fatalf("freeze test wallet: %v", err)
	}
	service := newConfiguredPurchaseService(
		t, db, configuredFakeStripeGateway("acct_locked_http", "prod_locked_http"),
		&fakeCheckoutGateway{}, credits.NewService(pool),
	)
	handler := paymenthttp.NewUserCheckoutHandler(service, testPublicOrigins("https://configured.example.com"))
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
	repo := paymentpostgres.NewRepository(db)
	service := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, &fakeWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	handler := paymenthttp.NewAdminHandler(service, testPublicOrigins("https://app.example.com"))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleAdmin)
		c.Next()
	})
	router.PUT("/admin/payments/stripe-config", handler.UpdateStripeConfiguration)

	// Keep this fixture clearly above the contract limit without coupling the
	// root-package integration test to the ports package's implementation detail.
	oversizedKey := "sk_test_" + strings.Repeat("x", 1024)
	requestBody, err := json.Marshal(map[string]any{
		"api_key": oversizedKey, "payment_method_configuration_id": "pmc_testfixture",
		"product_tax_code": "txcd_10105001", "purchases_enabled": true,
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
	gateway := &fakeStripeGateway{prepared: paymentports.PreparedStripeConfiguration{
		AccountID: "acct_webhook_http", Mode: paymentports.StripeModeTest, ChargesEnabled: true,
		PaymentMethodConfigurationID: "pmc_testfixture", ProductID: "prod_webhook_http", WebhookEndpointID: "we_webhook_http", WebhookSecret: "whsec_webhook_http",
	}}
	verifier := &fakeWebhookVerifier{event: paymentports.StripeWebhookEvent{
		ID: "evt_http", Type: "customer.updated", ObjectID: "cus_http", CreatedAt: time.Now(),
	}}
	repo := paymentpostgres.NewRepository(db)
	service := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, verifier, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
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

func testPublicOrigins(webURL string) config.PublicOriginsConfig {
	return config.PublicOriginsConfig{Web: webURL, API: "https://api.example.com"}
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
