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
	handler := NewHandler(NewService(pool))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(7))
		c.Next()
	})
	router.POST("/api/admin/credits/grants", handler.Grant)
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
