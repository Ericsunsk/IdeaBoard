package credits

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
)

func TestAdminCreditHTTPContractGrantsAndReadsWallet(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	handler := NewHandler(service)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(7))
		c.Next()
	})
	router.POST("/api/admin/credits/grants", handler.Grant)
	router.POST("/api/admin/credits/deductions", handler.Deduct)
	router.POST("/api/admin/credits/refunds", handler.Refund)
	router.GET("/api/admin/credits/wallets/:user_id", handler.GetWallet)
	router.GET("/api/admin/credits/wallets/:user_id/ledger", handler.ListLedger)

	body := []byte(`{
  "user_id": 94,
  "amount_credits": 125,
  "idempotency_key": "http-contract:grant",
  "reason": "HTTP contract grant",
  "business_reference_id": "http-contract-grant"
}`)
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

	walletRecorder := httptest.NewRecorder()
	router.ServeHTTP(walletRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/credits/wallets/94", nil))
	if walletRecorder.Code != http.StatusOK || !bytes.Contains(walletRecorder.Body.Bytes(), []byte(`"available_credits":125`)) {
		t.Fatalf("wallet status = %d body=%s", walletRecorder.Code, walletRecorder.Body.String())
	}

	ledgerRecorder := httptest.NewRecorder()
	router.ServeHTTP(ledgerRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/credits/wallets/94/ledger?limit=10", nil))
	if ledgerRecorder.Code != http.StatusOK || !bytes.Contains(ledgerRecorder.Body.Bytes(), []byte(`"command_type":"grant"`)) {
		t.Fatalf("ledger status = %d body=%s", ledgerRecorder.Code, ledgerRecorder.Body.String())
	}
	if !bytes.Contains(ledgerRecorder.Body.Bytes(), []byte(`"actor":{"type":"admin","id":"7"}`)) {
		t.Fatalf("ledger actor was not derived from authenticated admin: %s", ledgerRecorder.Body.String())
	}
}

func TestAdminCreditHTTPBindsActorAndRejectsUnknownFields(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	service := NewService(pool)
	handler := NewHandler(service)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", int64(7)); c.Next() })
	router.POST("/api/admin/credits/grants", handler.Grant)
	router.POST("/api/admin/credits/deductions", handler.Deduct)
	router.POST("/api/admin/credits/refunds", handler.Refund)

	spoofed := []byte(`{"user_id":95,"amount_credits":5,"idempotency_key":"http-spoof","reason":"test","business_reference_id":"test","actor_id":"999","source":"user"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/credits/grants", bytes.NewReader(spoofed))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("spoofed grant status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	grant := []byte(`{"user_id":95,"amount_credits":5,"idempotency_key":"http-spoof-grant","reason":"grant","business_reference_id":"grant"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/grants", bytes.NewReader(grant))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("grant status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	deduction := []byte(`{"user_id":95,"amount_credits":2,"idempotency_key":"http-spoof-deduct","reason":"deduct","business_reference_id":"deduct"}`)
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

	missingCharge := []byte(`{"user_id":95,"charge_transaction_id":"ctx_missing","amount_credits":1,"idempotency_key":"http-refund-missing","reason":"refund","business_reference_id":"refund-missing"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/refunds", bytes.NewReader(missingCharge))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing refund status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	longChargeID := bytes.Repeat([]byte("x"), 65)
	invalidRefund := []byte(`{"user_id":95,"charge_transaction_id":"` + string(longChargeID) + `","amount_credits":1,"idempotency_key":"http-refund-invalid-reference","reason":"refund","business_reference_id":"refund-invalid-reference"}`)
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
	overLimit := []byte(`{"user_id":94,"charge_transaction_id":"` + capture.TransactionID + `","amount_credits":4,"idempotency_key":"http-refund-limit","reason":"refund","business_reference_id":"refund-limit"}`)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/credits/refunds", bytes.NewReader(overLimit))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit refund status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
