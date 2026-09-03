package rewards

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func TestRewardRedeemPreservesCommittedOutcomeWithoutPostCommitIdentityProjection(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), CreateRewardCodeCommand{
		DisplayName: "Projection failure", Code: "PROJECTION-FAILURE-2026", CreditAmount: 31,
		IdempotencyKey: "projection-failure", ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("CreateRewardCode() error = %v", err)
	}
	provisionRewardWallet(t, pool, creditService, 3)

	handler := NewUserHandler(service)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var authenticated principal.Principal
	router.Use(func(c *gin.Context) {
		authenticated = testsupport.SetPrincipal(t, c, 3, principal.RoleUser)
		c.Next()
	})
	router.POST("/user/rewards/redeem", handler.Redeem)

	firstRecorder := performProjectionFailureRedeem(t, router)
	replayRecorder := performProjectionFailureRedeem(t, router)

	var redemptionCount, grantCount int
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT COUNT(*) FROM reward_redemptions WHERE reward_code_id = $1 AND user_id = $2),
  (SELECT COUNT(*)
   FROM reward_redemptions redemptions
   JOIN credit_transactions transactions
     ON transactions.transaction_id = redemptions.credit_transaction_id
    AND transactions.business_reference_type = 'reward_code_redemption'
    AND transactions.business_reference_id = 'rc:' || redemptions.id::TEXT
   WHERE redemptions.reward_code_id = $1 AND redemptions.user_id = $2)`,
		created.RewardCode.ID, 3).Scan(&redemptionCount, &grantCount); err != nil {
		t.Fatalf("count committed Redemption and Credit Grant facts: %v", err)
	}
	wallet, err := creditService.GetWallet(t.Context(), 3)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if redemptionCount != 1 || grantCount != 1 || wallet.Available != 31 {
		t.Fatalf("committed facts: Redemptions=%d Grants=%d available=%d, want 1, 1 and 31",
			redemptionCount, grantCount, wallet.Available)
	}

	first := decodeRewardRedeemResult(t, firstRecorder)
	replay := decodeRewardRedeemResult(t, replayRecorder)
	if firstRecorder.Code != http.StatusOK || replayRecorder.Code != http.StatusOK {
		t.Fatalf("statuses = (%d, %d), want (%d, %d); first=%s replay=%s",
			firstRecorder.Code, replayRecorder.Code, http.StatusOK, http.StatusOK,
			firstRecorder.Body.String(), replayRecorder.Body.String())
	}
	if first.Replayed || !replay.Replayed || first.Redemption.ID == 0 || replay.Redemption.ID != first.Redemption.ID ||
		first.CreditedAmount != 31 || replay.CreditedAmount != 31 ||
		first.AvailableBalance != 31 || replay.AvailableBalance != 31 ||
		first.Redemption.UserPublicID != authenticated.PublicID() || replay.Redemption.UserPublicID != authenticated.PublicID() ||
		first.Redemption.DisplayName != authenticated.DisplayName() || replay.Redemption.DisplayName != authenticated.DisplayName() {
		t.Fatalf("first=%+v replay=%+v", first, replay)
	}
}

func performProjectionFailureRedeem(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(
		http.MethodPost,
		"/user/rewards/redeem",
		strings.NewReader(`{"code":"PROJECTION-FAILURE-2026"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeRewardRedeemResult(t *testing.T, recorder *httptest.ResponseRecorder) RedeemResult {
	t.Helper()
	var response rewardHTTPResponse[RedeemResult]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode Redeem response: %v; body=%s", err, recorder.Body.String())
	}
	return response.Data
}
