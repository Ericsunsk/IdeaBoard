package rewards

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestRewardCodeAdminAndUserHTTPContract(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	handler := NewHandler(service)
	gin.SetMode(gin.TestMode)

	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	adminRouter.POST("/admin/rewards/codes", handler.CreateRewardCode)
	adminRouter.GET("/admin/rewards/codes", handler.ListRewardCodes)
	adminRouter.PATCH("/admin/rewards/codes/:id", handler.UpdateRewardCode)
	adminRouter.PATCH("/admin/rewards/codes/:id/enabled", handler.SetRewardCodeEnabled)
	adminRouter.DELETE("/admin/rewards/codes/:id", handler.DeleteRewardCode)

	request := map[string]any{
		"display_name":    "HTTP 新用户奖励",
		"code":            "HTTP-REWARD-2026",
		"credit_amount":   25,
		"max_redemptions": 0,
		"idempotency_key": "http-reward-code-create",
	}
	created, createRecorder := performRewardRequestWithRecorder[CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", request, http.StatusOK,
	)
	if createRecorder.Header().Get("Cache-Control") != "no-store" ||
		created.Plaintext != "HTTPREWARD2026" || created.Replayed {
		t.Fatalf("create response = %+v Cache-Control=%q", created, createRecorder.Header().Get("Cache-Control"))
	}
	replay, replayRecorder := performRewardRequestWithRecorder[CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", request, http.StatusOK,
	)
	if replayRecorder.Header().Get("Cache-Control") != "no-store" ||
		!replay.Replayed || replay.Plaintext != "" || replay.RewardCode.ID != created.RewardCode.ID {
		t.Fatalf("create replay = %+v Cache-Control=%q", replay, replayRecorder.Header().Get("Cache-Control"))
	}
	page, listRecorder := performRewardRequestWithRecorder[RewardCodePage](
		t, adminRouter, http.MethodGet, "/admin/rewards/codes?limit=20", nil, http.StatusOK,
	)
	if listRecorder.Header().Get("Cache-Control") != "no-store" || len(page.RewardCodes) != 1 {
		t.Fatalf("list = %+v Cache-Control=%q", page, listRecorder.Header().Get("Cache-Control"))
	}
	if strings.Contains(listRecorder.Body.String(), "HTTPREWARD2026") ||
		strings.Contains(listRecorder.Body.String(), "HTTP-REWARD-2026") {
		t.Fatalf("list response leaked raw Code: %s", listRecorder.Body.String())
	}
	conflictRequest := map[string]any{
		"display_name":    "重复 Code",
		"code":            "HTTP-REWARD-2026",
		"credit_amount":   10,
		"max_redemptions": 0,
		"idempotency_key": "http-reward-code-conflict",
	}
	_, conflictRecorder := performRewardRequestWithRecorder[struct{}](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", conflictRequest, http.StatusConflict,
	)
	if conflictRecorder.Header().Get("Cache-Control") != "no-store" ||
		strings.Contains(conflictRecorder.Body.String(), "HTTPREWARD2026") ||
		strings.Contains(conflictRecorder.Body.String(), "HTTP-REWARD-2026") {
		t.Fatalf("conflict response leaked raw Code: %s", conflictRecorder.Body.String())
	}
	codePath := fmt.Sprintf("/admin/rewards/codes/%d", created.RewardCode.ID)
	updated, updateRecorder := performRewardRequestWithRecorder[UpdatedRewardCode](
		t, adminRouter, http.MethodPatch, codePath, map[string]any{
			"display_name": "HTTP 新用户奖励", "credit_amount": 25, "max_redemptions": 0,
		}, http.StatusOK,
	)
	if updateRecorder.Header().Get("Cache-Control") != "no-store" || updated.RewardCode.ID != created.RewardCode.ID ||
		updated.Plaintext != "" || updated.Replayed != nil || strings.Contains(updateRecorder.Body.String(), "plaintext") ||
		strings.Contains(updateRecorder.Body.String(), "replayed") {
		t.Fatalf("update = %+v Cache-Control=%q", updated, updateRecorder.Header().Get("Cache-Control"))
	}
	disabled, _ := performRewardRequestWithRecorder[RewardCode](
		t, adminRouter, http.MethodPatch, codePath+"/enabled", map[string]any{"enabled": false}, http.StatusOK,
	)
	if disabled.Enabled || disabled.Status != RewardCodeStatusDisabled {
		t.Fatalf("disabled RewardCode = %+v", disabled)
	}
	performRewardRequestWithRecorder[RewardCode](
		t, adminRouter, http.MethodPatch, codePath+"/enabled", map[string]any{"enabled": true}, http.StatusOK,
	)
	rotated, rotateRecorder := performRewardRequestWithRecorder[UpdatedRewardCode](
		t, adminRouter, http.MethodPatch, codePath, map[string]any{
			"display_name": "HTTP 新用户奖励", "credit_amount": 25, "max_redemptions": 0,
			"code": "HTTP-ROTATED-2026",
		}, http.StatusOK,
	)
	if rotateRecorder.Header().Get("Cache-Control") != "no-store" ||
		rotated.Plaintext != "HTTPROTATED2026" || rotated.Replayed == nil || *rotated.Replayed {
		t.Fatalf("rotate = %+v Cache-Control=%q", rotated, rotateRecorder.Header().Get("Cache-Control"))
	}
	rotateReplay, _ := performRewardRequestWithRecorder[UpdatedRewardCode](
		t, adminRouter, http.MethodPatch, codePath, map[string]any{
			"display_name": "HTTP 新用户奖励", "credit_amount": 25, "max_redemptions": 0,
			"code": "HTTP-ROTATED-2026",
		}, http.StatusOK,
	)
	if rotateReplay.Replayed == nil || !*rotateReplay.Replayed || rotateReplay.Plaintext != "" {
		t.Fatalf("rotate replay = %+v", rotateReplay)
	}
	temporary, _ := performRewardRequestWithRecorder[CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", map[string]any{
			"display_name": "待删除", "code": "HTTP-DELETE-2026", "credit_amount": 1,
			"max_redemptions": 0, "idempotency_key": "http-delete",
		}, http.StatusOK,
	)
	deleted, _ := performRewardRequestWithRecorder[DeletedRewardCode](
		t, adminRouter, http.MethodDelete,
		fmt.Sprintf("/admin/rewards/codes/%d", temporary.RewardCode.ID), nil, http.StatusOK,
	)
	if deleted.RewardCodeID != temporary.RewardCode.ID {
		t.Fatalf("delete = %+v", deleted)
	}

	provisionRewardWallet(t, pool, creditService, 3)
	userRouter := gin.New()
	userRouter.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 3, principal.RoleUser)
		c.Next()
	})
	userRouter.POST("/user/rewards/redeem", handler.Redeem)
	userRouter.GET("/user/rewards/redemptions", handler.ListRedemptions)
	redeemed, redeemRecorder := performRewardRequestWithRecorder[RedeemResult](
		t, userRouter, http.MethodPost, "/user/rewards/redeem", map[string]any{"code": "HTTP-ROTATED-2026"}, http.StatusOK,
	)
	if redeemRecorder.Header().Get("Cache-Control") != "no-store" || redeemed.CreditedAmount != 25 {
		t.Fatalf("redeem = %+v Cache-Control=%q", redeemed, redeemRecorder.Header().Get("Cache-Control"))
	}
	var redeemPayload struct {
		Data struct {
			Redemption map[string]json.RawMessage `json:"redemption"`
		} `json:"data"`
	}
	if err := json.Unmarshal(redeemRecorder.Body.Bytes(), &redeemPayload); err != nil {
		t.Fatalf("decode redeem response: %v", err)
	}
	for _, field := range []string{"reward_code_display_name", "masked_code_hint"} {
		if _, found := redeemPayload.Data.Redemption[field]; found {
			t.Fatalf("redeem redemption unexpectedly exposed snapshot field %q", field)
		}
	}
	_, redeemedDeleteRecorder := performRewardRequestWithRecorder[struct{}](
		t, adminRouter, http.MethodDelete, codePath, nil, http.StatusConflict,
	)
	if redeemedDeleteRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redeemed delete Cache-Control=%q", redeemedDeleteRecorder.Header().Get("Cache-Control"))
	}
	invalidRaw := "INVALID-SECRET-9999"
	_, invalidRecorder := performRewardRequestWithRecorder[struct{}](
		t, userRouter, http.MethodPost, "/user/rewards/redeem", map[string]any{"code": invalidRaw}, http.StatusUnprocessableEntity,
	)
	if invalidRecorder.Header().Get("Cache-Control") != "no-store" || strings.Contains(invalidRecorder.Body.String(), invalidRaw) {
		t.Fatalf("invalid response leaked raw Code: %s", invalidRecorder.Body.String())
	}
	_, historyRecorder := performRewardRequestWithRecorder[RedemptionHistoryPage](
		t, userRouter, http.MethodGet, "/user/rewards/redemptions", nil, http.StatusOK,
	)
	if historyRecorder.Header().Get("Cache-Control") != "no-store" ||
		strings.Contains(historyRecorder.Body.String(), "HTTPREWARD2026") {
		t.Fatalf("history response leaked raw Code: %s", historyRecorder.Body.String())
	}
}

type rewardHTTPResponse[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

func performRewardRequestWithRecorder[T any](
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body any,
	wantStatus int,
) (T, *httptest.ResponseRecorder) {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s body: %v", method, path, err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, recorder.Code, wantStatus, recorder.Body.String())
	}
	var response rewardHTTPResponse[T]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, recorder.Body.String())
	}
	if response.Code != 0 && wantStatus == http.StatusOK {
		t.Fatalf("%s %s response code = %d; body=%s", method, path, response.Code, recorder.Body.String())
	}
	return response.Data, recorder
}
