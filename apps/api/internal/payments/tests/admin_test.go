package tests

import (
	payments "ideaboard/api/internal/payments"

	"github.com/jackc/pgx/v5/pgconn"

	"context"
	"encoding/base64"
	"encoding/json"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"net/http"
	"strings"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/identity/principal"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentreadmodel "ideaboard/api/internal/payments/application/readmodel"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAdminPaymentOrderHTTPReadModelUsesFiltersAndBoundedKeyset(t *testing.T) {
	db := testdb.Open(t)
	repo := paymentpostgres.NewRepository(db)
	publishTestStripeConfiguration(t, repo, "acct_admin_read", "prod_admin_read")
	service := newCompleteTestPaymentsService(t, repo, payments.NewPaymentRuntime(nil, nil), testStripeGateway{}, testCheckoutGateway{}, testWalletReader{}, testWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	user42PublicID := adminPaymentUserPublicID(t, db, 42)
	first := createAdminPaymentOrder(t, repo, 42, "IBP-20260801-AAA000000001", 501)
	second := createAdminPaymentOrder(t, repo, 42, "IBP-20260801-BBB000000002", 502)
	third := createAdminPaymentOrder(t, repo, 99, "IBP-20260801-CCC000000003", 503)
	markAdminPaymentOrderAnomalous(t, db, second.ID)

	handler := paymenthttp.NewAdminHandler(service, config.PublicOriginsConfig{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		role := principal.RoleAdmin
		if c.GetHeader("X-Test-Role") == "user" {
			role = principal.RoleUser
		}
		testsupport.SetPrincipal(t, c, 1, role)
		c.Next()
	})
	router.GET("/admin/payments/orders", handler.ListAdminPaymentOrders)
	router.GET("/admin/payments/orders/:order_number", handler.GetAdminPaymentOrder)

	firstPageResponse := performPaymentRequest(router, http.MethodGet, "/admin/payments/orders?limit=1", "", nil)
	if firstPageResponse.Code != http.StatusOK || firstPageResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("first page status=%d headers=%v body=%s", firstPageResponse.Code, firstPageResponse.Header(), firstPageResponse.Body.String())
	}
	var firstPage paymentcontracts.AdminPaymentOrderPage
	decodePaymentResponseData(t, firstPageResponse.Body.Bytes(), &firstPage)
	if firstPage.Total != 3 || len(firstPage.Orders) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first page=%+v", firstPage)
	}

	secondPageResponse := performPaymentRequest(router, http.MethodGet, "/admin/payments/orders?limit=1&cursor="+firstPage.NextCursor, "", nil)
	if secondPageResponse.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", secondPageResponse.Code, secondPageResponse.Body.String())
	}
	var secondPage paymentcontracts.AdminPaymentOrderPage
	decodePaymentResponseData(t, secondPageResponse.Body.Bytes(), &secondPage)
	if secondPage.Total != 3 || len(secondPage.Orders) != 1 || secondPage.Orders[0].PublicOrderNumber == firstPage.Orders[0].PublicOrderNumber {
		t.Fatalf("second page=%+v first=%+v", secondPage, firstPage)
	}

	for _, testCase := range []struct {
		name string
		path string
		want int
	}{
		{name: "exact order", path: "/admin/payments/orders?order_number=" + first.PublicOrderNumber, want: 1},
		{name: "user public id", path: "/admin/payments/orders?user=" + user42PublicID.String(), want: 2},
		{name: "display name", path: "/admin/payments/orders?user=test-fixture-user-99", want: 1},
		{name: "display name underscore literal", path: "/admin/payments/orders?user=test_fixture-user-99", want: 0},
		{name: "numeric display name", path: "/admin/payments/orders?user=0", want: 0},
		{name: "display name with space", path: "/admin/payments/orders?user=unsafe%20name", want: 0},
		{name: "anomaly", path: "/admin/payments/orders?anomaly_only=true", want: 1},
	} {
		response := performPaymentRequest(router, http.MethodGet, testCase.path, "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", testCase.name, response.Code, response.Body.String())
		}
		var page paymentcontracts.AdminPaymentOrderPage
		decodePaymentResponseData(t, response.Body.Bytes(), &page)
		if page.Total != int64(testCase.want) || len(page.Orders) != testCase.want {
			t.Fatalf("%s page=%+v", testCase.name, page)
		}
	}

	if third.PublicOrderNumber == first.PublicOrderNumber || second.PublicOrderNumber == first.PublicOrderNumber {
		t.Fatal("test orders must have distinct public numbers")
	}
	nonCanonicalCursorPayload, err := json.Marshal(paymentreadmodel.AdminPaymentOrderCursor{
		CreatedAt: third.CreatedAt.In(time.FixedZone("UTC+8", 8*60*60)).Format(time.RFC3339Nano),
		OrderID:   third.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	nonCanonicalCursor := base64.RawURLEncoding.EncodeToString(nonCanonicalCursorPayload)
	for _, testCase := range []struct {
		name     string
		query    string
		wantCode string
	}{
		{name: "limit zero", query: "limit=0", wantCode: "admin_payment_order_filter_invalid"},
		{name: "limit over max", query: "limit=201", wantCode: "admin_payment_order_filter_invalid"},
		{name: "limit malformed", query: "limit=x", wantCode: "admin_payment_order_filter_invalid"},
		{name: "cursor malformed", query: "cursor=bad", wantCode: "admin_payment_order_cursor_invalid"},
		{name: "cursor non-canonical", query: "cursor=" + nonCanonicalCursor, wantCode: "admin_payment_order_cursor_invalid"},
		{name: "status", query: "status=paid", wantCode: "admin_payment_order_filter_invalid"},
		{name: "order number", query: "order_number=not-an-order", wantCode: "admin_payment_order_filter_invalid"},
		{name: "user newline", query: "user=unsafe%0Aname", wantCode: "admin_payment_order_filter_invalid"},
		{name: "anomaly only", query: "anomaly_only=1", wantCode: "admin_payment_order_filter_invalid"},
		{name: "unexpected param", query: "unexpected=true", wantCode: "admin_payment_order_filter_invalid"},
		{name: "repeated limit", query: "limit=1&limit=2", wantCode: "admin_payment_order_filter_invalid"},
	} {
		response := performPaymentRequest(router, http.MethodGet, "/admin/payments/orders?"+testCase.query, "", nil)
		if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("invalid query %q status=%d headers=%v body=%s", testCase.query, response.Code, response.Header(), response.Body.String())
		}
		var machineError struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &machineError); err != nil {
			t.Fatalf("invalid query %q decode error: %v body=%s", testCase.query, err, response.Body.String())
		}
		if machineError.Code != testCase.wantCode {
			t.Fatalf("invalid query %q code=%q want %q body=%s", testCase.query, machineError.Code, testCase.wantCode, response.Body.String())
		}
	}
	for _, path := range []string{
		"/admin/payments/orders/invalid",
		"/admin/payments/orders/IBP-20260801-ZZZ000000009",
	} {
		response := performPaymentRequest(router, http.MethodGet, path, "", nil)
		wantStatus := http.StatusNotFound
		if strings.HasSuffix(path, "/invalid") {
			wantStatus = http.StatusBadRequest
		}
		if response.Code != wantStatus {
			t.Fatalf("detail %q status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	unauthorized := performPaymentRequest(router, http.MethodGet, "/admin/payments/orders", "", map[string]string{"X-Test-Role": "user"})
	if unauthorized.Code != http.StatusForbidden {
		t.Fatalf("non-admin status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
}

func TestAdminPaymentOrderDetailMasksProviderIdentifiersAndExcludesInternalFields(t *testing.T) {
	db := testdb.Open(t)
	repo := paymentpostgres.NewRepository(db)
	publishTestStripeConfiguration(t, repo, "acct_admin_read", "prod_admin_read")
	service := newCompleteTestPaymentsService(t, repo, payments.NewPaymentRuntime(nil, nil), testStripeGateway{}, testCheckoutGateway{}, testWalletReader{}, testWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	userPublicID := adminPaymentUserPublicID(t, db, 42)
	order := createAdminPaymentOrder(t, repo, 42, "IBP-20260801-DETAIL000001", 501)
	bound, err := repo.BindPaymentOrderCheckout(t.Context(), paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_admin_detail_secret", StripeSessionID: "cs_admin_detail_secret",
		CheckoutURL: "https://checkout.stripe.example/session", ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		t.Fatalf("BindPaymentOrderCheckout() error = %v", err)
	}
	markAdminPaymentOrderAnomalous(t, db, bound.ID)

	handler := paymenthttp.NewAdminHandler(service, config.PublicOriginsConfig{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/payments/orders/:order_number", handler.GetAdminPaymentOrder)

	response := performPaymentRequest(router, http.MethodGet, "/admin/payments/orders/"+order.PublicOrderNumber, "", nil)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("detail status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	var detail paymentcontracts.AdminPaymentOrderDetail
	decodePaymentResponseData(t, response.Body.Bytes(), &detail)
	if detail.User.PublicID != userPublicID || detail.User.DisplayName != "test-fixture-user-42" || detail.Status != paymentsdomain.PaymentOrderCheckoutOpen ||
		detail.Stripe.CustomerID == "" || strings.Contains(detail.Stripe.CustomerID, "admin_detail_secret") ||
		!detail.Reconciliation.Anomalous || detail.Reconciliation.LastErrorSummary != "需要人工检查" {
		t.Fatalf("detail=%+v", detail)
	}
	var payload struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode JSON payload: %v", err)
	}
	for _, field := range []string{"id", "public_token", "client_idempotency_key", "request_hash", "return_base_url", "checkout_url", "credit_transaction_id"} {
		if _, found := payload.Data[field]; found {
			t.Fatalf("detail exposed internal field %q: %s", field, response.Body.String())
		}
	}
	var publicUser map[string]json.RawMessage
	if err := json.Unmarshal(payload.Data["user"], &publicUser); err != nil {
		t.Fatalf("decode public user: %v", err)
	}
	if len(publicUser) != 2 || publicUser["public_id"] == nil || publicUser["display_name"] == nil {
		t.Fatalf("public user fields = %v, body=%s", publicUser, response.Body.String())
	}
	for _, forbidden := range []string{"cus_admin_detail_secret", "cs_admin_detail_secret", "checkout.stripe.example"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("detail exposed provider/internal value %q: %s", forbidden, response.Body.String())
		}
	}
}

func adminPaymentUserPublicID(t *testing.T, db *pgxpool.Pool, userID int64) uuid.UUID {
	t.Helper()
	var publicID uuid.UUID
	if err := db.QueryRow(t.Context(), `SELECT public_id FROM users WHERE id = $1`, userID).Scan(&publicID); err != nil {
		t.Fatalf("load payment user public identity: %v", err)
	}
	return publicID
}

func TestAdminPaymentOrderDetailAssociatesRefundAndDisputeWebhookFacts(t *testing.T) {
	db := testdb.Open(t)
	repo := paymentpostgres.NewRepository(db)
	publishTestStripeConfiguration(t, repo, "acct_admin_read", "prod_admin_read")
	service := newCompleteTestPaymentsService(t, repo, payments.NewPaymentRuntime(nil, nil), testStripeGateway{}, testCheckoutGateway{}, testWalletReader{}, testWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	order := createAdminPaymentOrder(t, repo, 42, "IBP-20260801-EVENTS000001", 501)
	bound, err := repo.BindPaymentOrderCheckout(t.Context(), paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_events", StripeSessionID: "cs_events",
		CheckoutURL: "https://checkout.stripe.example/events", ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		t.Fatalf("BindPaymentOrderCheckout() error = %v", err)
	}
	paidAt := time.Now().UTC().Truncate(time.Second)
	if _, _, err := repo.RecordPaidPaymentOrder(t.Context(), paymentsdomain.PaidCheckoutFacts{
		StripeAccountID: bound.StripeAccountID, StripeSessionID: bound.StripeCheckoutSessionID,
		ExpiresAt: bound.ExpiresAt, PublicOrderNumber: bound.PublicOrderNumber, PaymentOrderID: bound.ID,
		RequestHash: bound.RequestHash, StripeCustomerID: bound.StripeCustomerID,
		StripePaymentIntentID: "pi_events", StripeChargeID: "ch_events", IdeaQuantity: bound.IdeaQuantity,
		PaidSubtotalMinor: bound.BaseAmountMinor, PaidTaxMinor: 0, PaidTotalMinor: bound.BaseAmountMinor,
		PaidCurrency: "usd", ReceiptURL: "https://pay.stripe.com/receipts/events", PaidAt: paidAt,
	}); err != nil {
		t.Fatalf("RecordPaidPaymentOrder() error = %v", err)
	}
	processedAt := paidAt.Add(time.Minute)
	for _, event := range []paymentports.StripeWebhookEvent{
		{ID: "evt_refund_events", Type: "refund.updated", ObjectID: "re_events_12345678", PaymentIntentID: "pi_events", Livemode: false, CreatedAt: processedAt},
		{ID: "evt_dispute_events", Type: "charge.dispute.created", ObjectID: "dp_events_12345678", ChargeID: "ch_events", Livemode: false, CreatedAt: processedAt.Add(time.Second)},
	} {
		duplicate, err := repo.InsertStripeWebhookEvent(t.Context(), event, "processed", &processedAt)
		if err != nil || duplicate {
			t.Fatalf("InsertStripeWebhookEvent(%s) duplicate=%v error=%v", event.Type, duplicate, err)
		}
	}

	detail, err := service.GetAdminPaymentOrder(t.Context(), order.PublicOrderNumber)
	if err != nil {
		t.Fatalf("GetAdminPaymentOrder() error = %v", err)
	}
	if len(detail.Refunds) != 1 || detail.Refunds[0].Status != "refund.updated" || detail.Refunds[0].ID != "re_e…5678" {
		t.Fatalf("refunds = %+v", detail.Refunds)
	}
	if len(detail.Disputes) != 1 || detail.Disputes[0].Status != "charge.dispute.created" || detail.Disputes[0].ID != "dp_e…5678" {
		t.Fatalf("disputes = %+v", detail.Disputes)
	}
}

func createAdminPaymentOrder(t *testing.T, repo *paymentpostgres.Repository, userID int64, publicOrderNumber string, quantity int64) paymentsdomain.PaymentOrder {
	t.Helper()
	configuration := activeTestStripeConfiguration(t, repo)
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), paymentsdomain.CreatePaymentOrderIntentInput{
		ID: uuid.NewString(), PublicOrderNumber: publicOrderNumber, PublicToken: uuid.NewString(), UserID: userID,
		ClientIdempotencyKey: "admin-read-" + strings.ToLower(publicOrderNumber[len(publicOrderNumber)-12:]),
		RequestHash:          "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID:   configuration.CredentialID,
		StripeAccountID:      "acct_admin_read", StripeProductID: "prod_admin_read", IdeaQuantity: quantity,
		TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards", ExpiresAt: time.Now().UTC().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreatePaymentOrderIntent(%s) error = %v", publicOrderNumber, err)
	}
	return order
}

func markAdminPaymentOrderAnomalous(t *testing.T, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, orderID string) {
	t.Helper()
	parsed, err := uuid.Parse(orderID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `
UPDATE payment_order_reconciliations
SET anomalous_at = now(),
    last_checked_at = now(),
    last_error_code = 'configuration_unavailable',
    last_error_summary = '需要人工检查'
WHERE payment_order_id = $1`, parsed); err != nil {
		t.Fatalf("mark payment order anomalous: %v", err)
	}
}
