package credits

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type integrationCreditUserResolver struct{}

func (integrationCreditUserResolver) ResolvePublicID(_ context.Context, rawPublicID string) (UserIdentity, error) {
	publicID, err := uuid.Parse(rawPublicID)
	if err != nil {
		return UserIdentity{}, err
	}
	for _, userID := range []int64{94, 95} {
		if publicID == integrationCreditPublicID(userID) {
			return integrationCreditIdentity(userID), nil
		}
	}
	return UserIdentity{}, fmt.Errorf("test user not found")
}

func (integrationCreditUserResolver) ResolveID(_ context.Context, userID int64) (UserIdentity, error) {
	if userID <= 0 {
		return UserIdentity{}, fmt.Errorf("test user not found")
	}
	return integrationCreditIdentity(userID), nil
}

func integrationCreditIdentity(userID int64) UserIdentity {
	return UserIdentity{
		ID:          userID,
		PublicID:    integrationCreditPublicID(userID),
		DisplayName: fmt.Sprintf("User %d", userID),
	}
}

func integrationCreditPublicID(userID int64) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("credits-test-user:%d", userID)))
}

func TestAdminCreditReconciliationHTTPContractStrictBodyAndPagination(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	service.SetReconciliationEnqueuer(&reconciliationEnqueuerStub{})
	handler := NewHandler(service, integrationCreditUserResolver{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/api/admin/credits/reconciliation/runs", handler.StartReconciliation)
	router.GET("/api/admin/credits/health", handler.GetCreditHealth)
	router.GET("/api/admin/credits/reconciliation/runs", handler.ListReconciliationRuns)
	router.GET("/api/admin/credits/reconciliation/runs/:run_id", handler.GetReconciliationRun)
	router.GET("/api/admin/credits/reconciliation/findings", handler.ListGlobalReconciliationFindings)

	unknown := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/credits/reconciliation/runs", bytes.NewBufferString(`{"reason":"manual audit","unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(unknown, request)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, body=%s", unknown.Code, unknown.Body.String())
	}

	oversized := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/reconciliation/runs", bytes.NewReader(bytes.Repeat([]byte(" "), maxAdminCreditRequestBytes+1)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(oversized, request)
	if oversized.Code != http.StatusBadRequest {
		t.Fatalf("oversized status = %d, body=%s", oversized.Code, oversized.Body.String())
	}
	for _, body := range []string{`{}`, `{"reason":"  "}`, `{"reason":"` + strings.Repeat("r", 1001) + `"}`} {
		rejected := httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/reconciliation/runs", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rejected, request)
		if rejected.Code != http.StatusBadRequest {
			t.Fatalf("invalid reason body=%s status=%d body=%s", body, rejected.Code, rejected.Body.String())
		}
	}

	created := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/reconciliation/runs", bytes.NewBufferString(`{"reason":"  manual audit  "}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(created, request)
	if created.Code != http.StatusOK ||
		!bytes.Contains(created.Body.Bytes(), []byte(`"status":"queued"`)) ||
		!bytes.Contains(created.Body.Bytes(), []byte(`"trigger":"manual"`)) {
		t.Fatalf("created response = %d %s", created.Code, created.Body.String())
	}
	assertNoPublicStaleHoldThreshold(t, created.Body.Bytes())
	var runID int64
	var trigger string
	var requestedBy int64
	var storedReason string
	var staleSeconds float64
	if err := pool.QueryRow(t.Context(), `
SELECT id, trigger_type, requested_by, reason, EXTRACT(EPOCH FROM stale_hold_after)
FROM credit_reconciliation_runs
`).Scan(&runID, &trigger, &requestedBy, &storedReason, &staleSeconds); err != nil {
		t.Fatal(err)
	}
	if trigger != "manual" || requestedBy != 1 || storedReason != "manual audit" || int64(staleSeconds) != 86400 {
		t.Fatalf("stored trigger=%q requestedBy=%d reason=%q stale=%v", trigger, requestedBy, storedReason, staleSeconds)
	}
	if err := service.ExecuteReconciliation(t.Context(), runID); err != nil {
		t.Fatal(err)
	}
	healthRecorder := httptest.NewRecorder()
	router.ServeHTTP(healthRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/credits/health", nil))
	if healthRecorder.Code != http.StatusOK ||
		!bytes.Contains(healthRecorder.Body.Bytes(), []byte(`"status":"healthy"`)) ||
		!bytes.Contains(healthRecorder.Body.Bytes(), []byte(`"latest_successful_run":{"id":`)) ||
		!bytes.Contains(healthRecorder.Body.Bytes(), []byte(`"open_findings":{"total":0`)) ||
		!bytes.Contains(healthRecorder.Body.Bytes(), []byte(`"restricted_wallet_count":0`)) {
		t.Fatalf("health response = %d %s", healthRecorder.Code, healthRecorder.Body.String())
	}
	assertNoPublicStaleHoldThreshold(t, healthRecorder.Body.Bytes())

	for path := range map[string]struct{}{
		"/api/admin/credits/reconciliation/runs?limit=1":                 {},
		fmt.Sprintf("/api/admin/credits/reconciliation/runs/%d", runID):  {},
		"/api/admin/credits/reconciliation/findings?limit=1&status=open": {},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d body=%s", path, recorder.Code, recorder.Body.String())
		}
		assertNoPublicStaleHoldThreshold(t, recorder.Body.Bytes())
	}
	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/admin/credits/reconciliation/runs?limit=201", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid pagination status = %d", invalid.Code)
	}
	invalidStatus := httptest.NewRecorder()
	router.ServeHTTP(invalidStatus, httptest.NewRequest(http.MethodGet, "/api/admin/credits/reconciliation/findings?status=pending", nil))
	if invalidStatus.Code != http.StatusBadRequest {
		t.Fatalf("invalid finding status = %d", invalidStatus.Code)
	}
	missingGlobalStatus := httptest.NewRecorder()
	router.ServeHTTP(missingGlobalStatus, httptest.NewRequest(http.MethodGet, "/api/admin/credits/reconciliation/findings", nil))
	if missingGlobalStatus.Code != http.StatusBadRequest {
		t.Fatalf("missing global finding status = %d", missingGlobalStatus.Code)
	}
}

func assertNoPublicStaleHoldThreshold(t testing.TB, body []byte) {
	t.Helper()
	if bytes.Contains(body, []byte(`"stale_hold_after_seconds"`)) {
		t.Fatalf("public reconciliation response exposes internal Stale Hold threshold: %s", body)
	}
}

func TestAdminCreditReconciliationReadHandlersReturnServiceUnavailable(t *testing.T) {
	handler := NewHandler(nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/runs", handler.ListReconciliationRuns)
	router.GET("/health", handler.GetCreditHealth)
	router.GET("/runs/:run_id", handler.GetReconciliationRun)
	router.GET("/findings", handler.ListGlobalReconciliationFindings)
	for _, path := range []string{"/health", "/runs", "/runs/1", "/findings?status=open"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET %s status = %d, want 503; body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestAdminCreditHTTPContractGrantsAndReadsWallet(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	handler := NewHandler(service, integrationCreditUserResolver{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/api/admin/credits/grants", handler.Grant)
	router.POST("/api/admin/credits/deductions", handler.Deduct)
	router.POST("/api/admin/credits/refunds", handler.Refund)
	router.GET("/api/admin/credits/wallets/:public_id", handler.GetWallet)
	router.GET("/api/admin/credits/wallets/:public_id/ledger", handler.ListLedger)

	body := []byte(fmt.Sprintf(`{
  "user_public_id": %q,
  "amount_credits": 125,
  "idempotency_key": "http-contract:grant",
  "reason": "HTTP contract grant",
  "business_reference_id": "http-contract-grant"
}`, integrationCreditPublicID(94).String()))
	grantRecorder := httptest.NewRecorder()
	grantRequest := httptest.NewRequest(http.MethodPost, "/api/admin/credits/grants", bytes.NewReader(body))
	grantRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(grantRecorder, grantRequest)
	if grantRecorder.Code != http.StatusOK {
		t.Fatalf("grant status = %d body=%s", grantRecorder.Code, grantRecorder.Body.String())
	}
	var grantResponse struct {
		Code int           `json:"code"`
		Data CommandResult `json:"data"`
	}
	if err := json.Unmarshal(grantRecorder.Body.Bytes(), &grantResponse); err != nil {
		t.Fatalf("decode grant response error = %v", err)
	}
	if grantResponse.Code != 0 || grantResponse.Data.Wallet.Available != 125 {
		t.Fatalf("grant response = %+v", grantResponse)
	}
	if _, err := service.RestrictWallet(t.Context(), RestrictWalletCommand{
		UserID:        94,
		Type:          "ledger_integrity",
		Source:        "reconciliation",
		RelatedObject: BusinessReference{Type: "finding", ID: "http-contract-finding"},
		Reason:        "verify Wallet restriction HTTP read model",
		Actor:         Actor{Type: ActorSystem, ID: "reconciliation"},
	}); err != nil {
		t.Fatalf("RestrictWallet() error = %v", err)
	}

	walletRecorder := httptest.NewRecorder()
	router.ServeHTTP(walletRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/credits/wallets/"+integrationCreditPublicID(94).String(), nil))
	if walletRecorder.Code != http.StatusOK ||
		!bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"available_credits":125`)) ||
		!bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"active_restrictions":[{`)) ||
		!bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"source":"reconciliation"`)) ||
		!bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"id":"http-contract-finding"`)) {
		t.Fatalf("wallet status = %d body=%s", walletRecorder.Code, walletRecorder.Body.String())
	}

	ledgerRecorder := httptest.NewRecorder()
	router.ServeHTTP(ledgerRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/credits/wallets/"+integrationCreditPublicID(94).String()+"/ledger?limit=10", nil))
	if ledgerRecorder.Code != http.StatusOK || !bytes.Contains(ledgerRecorder.Body.Bytes(), []byte(`"command_type":"grant"`)) {
		t.Fatalf("ledger status = %d body=%s", ledgerRecorder.Code, ledgerRecorder.Body.String())
	}
	if !bytes.Contains(ledgerRecorder.Body.Bytes(), []byte(`"actor":{"type":"admin"}`)) ||
		bytes.Contains(ledgerRecorder.Body.Bytes(), []byte(`"id":"7"`)) {
		t.Fatalf("ledger actor was not derived from authenticated admin: %s", ledgerRecorder.Body.String())
	}
}

func TestUserCreditWalletHTTPScopesReadToAuthenticatedUser(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	ctx := context.Background()
	for _, grant := range []struct {
		userID int64
		amount int64
		key    string
	}{
		{userID: 94, amount: 125, key: "user-http:94"},
		{userID: 95, amount: 999, key: "user-http:95"},
	} {
		if _, err := service.Grant(ctx, GrantCommand{
			UserID:   grant.userID,
			Amount:   mustCreditAmount(t, grant.amount),
			Metadata: testMetadata(grant.key, grant.key),
		}); err != nil {
			t.Fatalf("Grant(%d) error = %v", grant.userID, err)
		}
	}

	handler := NewHandler(service, integrationCreditUserResolver{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 94, principal.RoleUser)
		c.Next()
	})
	router.GET("/api/user/credits/wallet", handler.GetCurrentWallet)

	walletRecorder := httptest.NewRecorder()
	router.ServeHTTP(walletRecorder, httptest.NewRequest(http.MethodGet, "/api/user/credits/wallet", nil))
	if walletRecorder.Code != http.StatusOK || !bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"public_id":"`+integrationCreditPublicID(94).String()+`"`)) ||
		bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"user_id"`)) ||
		!bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"available_credits":125`)) || bytes.Contains(walletRecorder.Body.Bytes(), []byte(`999`)) {
		t.Fatalf("wallet status = %d body=%s", walletRecorder.Code, walletRecorder.Body.String())
	}

}

func TestAdminCreditHTTPBindsActorAndRejectsUnknownFields(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	handler := NewHandler(service, integrationCreditUserResolver{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/api/admin/credits/grants", handler.Grant)
	router.POST("/api/admin/credits/deductions", handler.Deduct)
	router.POST("/api/admin/credits/refunds", handler.Refund)

	spoofed := []byte(`{"user_public_id":"` + integrationCreditPublicID(95).String() + `","amount_credits":5,"idempotency_key":"http-spoof","reason":"test","business_reference_id":"test","actor_id":"999","source":"user"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/credits/grants", bytes.NewReader(spoofed))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("spoofed grant status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	grant := []byte(`{"user_public_id":"` + integrationCreditPublicID(95).String() + `","amount_credits":5,"idempotency_key":"http-spoof-grant","reason":"grant","business_reference_id":"grant"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/grants", bytes.NewReader(grant))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("grant status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	deduction := []byte(`{"user_public_id":"` + integrationCreditPublicID(95).String() + `","amount_credits":2,"idempotency_key":"http-spoof-deduct","reason":"deduct","business_reference_id":"deduct"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/deductions", bytes.NewReader(deduction))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"command_type":"deduction"`)) {
		t.Fatalf("deduction status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	page, err := service.ListLedger(request.Context(), ListLedgerInput{UserID: 95, Limit: 10})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if len(page.Transactions) == 0 || page.Transactions[0].Actor.ID != "7" || page.Transactions[0].Source != "admin_deduction" {
		t.Fatalf("deduction audit = %+v", page.Transactions)
	}

	missingCharge := []byte(`{"user_public_id":"` + integrationCreditPublicID(95).String() + `","charge_transaction_id":"ctx_missing","amount_credits":1,"idempotency_key":"http-refund-missing","reason":"refund","business_reference_id":"refund-missing"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/refunds", bytes.NewReader(missingCharge))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing refund status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	longChargeID := bytes.Repeat([]byte("x"), 65)
	invalidRefund := []byte(`{"user_public_id":"` + integrationCreditPublicID(95).String() + `","charge_transaction_id":"` + string(longChargeID) + `","amount_credits":1,"idempotency_key":"http-refund-invalid-reference","reason":"refund","business_reference_id":"refund-invalid-reference"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/refunds", bytes.NewReader(invalidRefund))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid refund reference status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := service.Grant(request.Context(), GrantCommand{UserID: 94, Amount: mustCreditAmount(t, 3), Metadata: testMetadata("http-refund-limit-grant", "http-refund-limit-grant")}); err != nil {
		t.Fatalf("seed refund grant error = %v", err)
	}
	hold, err := service.Hold(request.Context(), HoldCommand{UserID: 94, Amount: mustCreditAmount(t, 3), Metadata: testMetadata("http-refund-limit-hold", "http-refund-limit-hold")})
	if err != nil {
		t.Fatalf("seed refund hold error = %v", err)
	}
	capture, err := service.Capture(request.Context(), CaptureCommand{UserID: 94, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("http-refund-limit-capture", "http-refund-limit-capture")})
	if err != nil {
		t.Fatalf("seed refund capture error = %v", err)
	}
	overLimit := []byte(`{"user_public_id":"` + integrationCreditPublicID(94).String() + `","charge_transaction_id":"` + capture.TransactionID + `","amount_credits":4,"idempotency_key":"http-refund-limit","reason":"refund","business_reference_id":"refund-limit"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/refunds", bytes.NewReader(overLimit))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit refund status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
