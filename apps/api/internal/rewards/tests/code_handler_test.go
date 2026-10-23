package tests

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
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func TestRewardCodeAdminHTTPContract(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	adminCommandHandler := rewardhttp.NewAdminCommandHandler(service)
	adminReadHandler := rewardhttp.NewAdminReadHandler(service, nil)
	gin.SetMode(gin.TestMode)

	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	adminRouter.POST("/admin/rewards/codes", adminCommandHandler.CreateRewardCode)
	adminRouter.POST("/admin/rewards/codes/search", adminReadHandler.SearchRewardCodes)
	adminRouter.PATCH("/admin/rewards/codes/:id", adminCommandHandler.UpdateRewardCode)
	adminRouter.PATCH("/admin/rewards/codes/:id/enabled", adminCommandHandler.SetRewardCodeEnabled)
	adminRouter.DELETE("/admin/rewards/codes/:id", adminCommandHandler.DeleteRewardCode)

	request := map[string]any{
		"display_name":    "HTTP 新用户奖励",
		"code":            "HTTP-REWARD-2026",
		"credit_amount":   25,
		"max_redemptions": 0,
	}
	created, createRecorder := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", request, "http-reward-code-create", http.StatusOK,
	)
	if createRecorder.Header().Get("Cache-Control") != "no-store" ||
		created.Plaintext != "HTTPREWARD2026" || created.Replayed {
		t.Fatalf("create response = %+v Cache-Control=%q", created, createRecorder.Header().Get("Cache-Control"))
	}
	replay, replayRecorder := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", request, "http-reward-code-create", http.StatusOK,
	)
	if replayRecorder.Header().Get("Cache-Control") != "no-store" ||
		!replay.Replayed || replay.Plaintext != "" || replay.RewardCode.ID != created.RewardCode.ID {
		t.Fatalf("create replay = %+v Cache-Control=%q", replay, replayRecorder.Header().Get("Cache-Control"))
	}
	page, searchRecorder := performRewardRequestWithRecorder[ports.RewardCodePage](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes/search", map[string]any{"limit": 20}, http.StatusOK,
	)
	if searchRecorder.Header().Get("Cache-Control") != "no-store" || len(page.RewardCodes) != 1 || page.Total != 1 {
		t.Fatalf("search = %+v Cache-Control=%q", page, searchRecorder.Header().Get("Cache-Control"))
	}
	if strings.Contains(searchRecorder.Body.String(), "HTTPREWARD2026") ||
		strings.Contains(searchRecorder.Body.String(), "HTTP-REWARD-2026") {
		t.Fatalf("search response leaked raw Code: %s", searchRecorder.Body.String())
	}
	exactPage, exactRecorder := performRewardRequestWithRecorder[ports.RewardCodePage](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes/search",
		map[string]any{"query": "HTTP-REWARD-2026"}, http.StatusOK,
	)
	if len(exactPage.RewardCodes) != 1 || exactPage.RewardCodes[0].ID != created.RewardCode.ID ||
		exactRecorder.Header().Get("Cache-Control") != "no-store" ||
		strings.Contains(exactRecorder.Body.String(), "HTTP-REWARD-2026") ||
		strings.Contains(exactRecorder.Body.String(), "HTTPREWARD2026") {
		t.Fatalf("exact Code search = %+v body=%s", exactPage, exactRecorder.Body.String())
	}
	enabledForPagination, _ := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", map[string]any{
			"display_name": "HTTP 分页奖励", "code": "HTTP-PAGE-ENABLED-2026", "credit_amount": 5,
			"max_redemptions": 0,
		}, "http-page-enabled", http.StatusOK,
	)
	disabledForFilter, _ := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", map[string]any{
			"display_name": "HTTP 停用奖励", "code": "HTTP-PAGE-DISABLED-2026", "credit_amount": 5,
			"max_redemptions": 0,
		}, "http-page-disabled", http.StatusOK,
	)
	disabledPath := fmt.Sprintf("/admin/rewards/codes/%d/enabled", disabledForFilter.RewardCode.ID)
	performRewardRequestWithRecorder[ports.RewardCode](
		t, adminRouter, http.MethodPatch, disabledPath, map[string]any{"enabled": false}, http.StatusOK,
	)
	firstEnabledPage, _ := performRewardRequestWithRecorder[ports.RewardCodePage](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes/search",
		map[string]any{"status": ports.RewardCodeStatusEnabled, "limit": 1}, http.StatusOK,
	)
	if firstEnabledPage.Total != 2 || len(firstEnabledPage.RewardCodes) != 1 ||
		firstEnabledPage.RewardCodes[0].ID != enabledForPagination.RewardCode.ID ||
		firstEnabledPage.NextBeforeID != enabledForPagination.RewardCode.ID {
		t.Fatalf("first enabled search page = %+v", firstEnabledPage)
	}
	secondEnabledPage, _ := performRewardRequestWithRecorder[ports.RewardCodePage](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes/search", map[string]any{
			"status": ports.RewardCodeStatusEnabled, "before_id": firstEnabledPage.NextBeforeID, "limit": 1,
		}, http.StatusOK,
	)
	if secondEnabledPage.Total != 2 || len(secondEnabledPage.RewardCodes) != 1 ||
		secondEnabledPage.RewardCodes[0].ID != created.RewardCode.ID || secondEnabledPage.NextBeforeID != 0 ||
		secondEnabledPage.RewardCodes[0].ID == firstEnabledPage.RewardCodes[0].ID {
		t.Fatalf("second enabled search page = %+v; first page = %+v", secondEnabledPage, firstEnabledPage)
	}
	for _, testCase := range []struct {
		name string
		path string
		body string
	}{
		{name: "unknown field", path: "/admin/rewards/codes/search", body: `{"unknown":true}`},
		{name: "unknown status", path: "/admin/rewards/codes/search", body: `{"status":"unknown"}`},
		{name: "zero cursor", path: "/admin/rewards/codes/search", body: `{"before_id":0}`},
		{name: "oversized limit", path: "/admin/rewards/codes/search", body: `{"limit":101}`},
		{name: "malformed body", path: "/admin/rewards/codes/search", body: `{"query":`},
		{name: "URL input", path: "/admin/rewards/codes/search?query=HTTP-REWARD-2026", body: `{}`},
	} {
		t.Run("search rejects "+testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			adminRouter.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || recorder.Header().Get("Cache-Control") != "no-store" ||
				strings.Contains(recorder.Body.String(), "HTTP-REWARD-2026") {
				t.Fatalf("response status=%d Cache-Control=%q body=%s", recorder.Code,
					recorder.Header().Get("Cache-Control"), recorder.Body.String())
			}
		})
	}
	conflictRequest := map[string]any{
		"display_name":    "重复 Code",
		"code":            "HTTP-REWARD-2026",
		"credit_amount":   10,
		"max_redemptions": 0,
	}
	_, conflictRecorder := performRewardWriteWithKey[struct{}](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", conflictRequest, "http-reward-code-conflict", http.StatusConflict,
	)
	if conflictRecorder.Header().Get("Cache-Control") != "no-store" ||
		strings.Contains(conflictRecorder.Body.String(), "HTTPREWARD2026") ||
		strings.Contains(conflictRecorder.Body.String(), "HTTP-REWARD-2026") {
		t.Fatalf("conflict response leaked raw Code: %s", conflictRecorder.Body.String())
	}
	codePath := fmt.Sprintf("/admin/rewards/codes/%d", created.RewardCode.ID)
	updated, updateRecorder := performRewardRequestWithRecorder[ports.UpdatedRewardCode](
		t, adminRouter, http.MethodPatch, codePath, map[string]any{
			"display_name": "HTTP 新用户奖励", "credit_amount": 25, "max_redemptions": 0,
		}, http.StatusOK,
	)
	if updateRecorder.Header().Get("Cache-Control") != "no-store" || updated.RewardCode.ID != created.RewardCode.ID ||
		updated.Plaintext != "" || updated.Replayed != nil || strings.Contains(updateRecorder.Body.String(), "plaintext") ||
		strings.Contains(updateRecorder.Body.String(), "replayed") {
		t.Fatalf("update = %+v Cache-Control=%q", updated, updateRecorder.Header().Get("Cache-Control"))
	}
	disabled, _ := performRewardRequestWithRecorder[ports.RewardCode](
		t, adminRouter, http.MethodPatch, codePath+"/enabled", map[string]any{"enabled": false}, http.StatusOK,
	)
	if disabled.Enabled || disabled.Status != ports.RewardCodeStatusDisabled {
		t.Fatalf("disabled RewardCode = %+v", disabled)
	}
	performRewardRequestWithRecorder[ports.RewardCode](
		t, adminRouter, http.MethodPatch, codePath+"/enabled", map[string]any{"enabled": true}, http.StatusOK,
	)
	rotated, rotateRecorder := performRewardRequestWithRecorder[ports.UpdatedRewardCode](
		t, adminRouter, http.MethodPatch, codePath, map[string]any{
			"display_name": "HTTP 新用户奖励", "credit_amount": 25, "max_redemptions": 0,
			"code": "HTTP-ROTATED-2026",
		}, http.StatusOK,
	)
	if rotateRecorder.Header().Get("Cache-Control") != "no-store" ||
		rotated.Plaintext != "HTTPROTATED2026" || rotated.Replayed == nil || *rotated.Replayed {
		t.Fatalf("rotate = %+v Cache-Control=%q", rotated, rotateRecorder.Header().Get("Cache-Control"))
	}
	rotateReplay, _ := performRewardRequestWithRecorder[ports.UpdatedRewardCode](
		t, adminRouter, http.MethodPatch, codePath, map[string]any{
			"display_name": "HTTP 新用户奖励", "credit_amount": 25, "max_redemptions": 0,
			"code": "HTTP-ROTATED-2026",
		}, http.StatusOK,
	)
	if rotateReplay.Replayed == nil || !*rotateReplay.Replayed || rotateReplay.Plaintext != "" {
		t.Fatalf("rotate replay = %+v", rotateReplay)
	}
	temporary, _ := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, adminRouter, http.MethodPost, "/admin/rewards/codes", map[string]any{
			"display_name": "待删除", "code": "HTTP-DELETE-2026", "credit_amount": 1,
			"max_redemptions": 0,
		}, "http-delete", http.StatusOK,
	)
	deleted, _ := performRewardRequestWithRecorder[ports.DeletedRewardCode](
		t, adminRouter, http.MethodDelete,
		fmt.Sprintf("/admin/rewards/codes/%d", temporary.RewardCode.ID), nil, http.StatusOK,
	)
	if deleted.RewardCodeID != temporary.RewardCode.ID {
		t.Fatalf("delete = %+v", deleted)
	}

	provisionRewardWallet(t, pool, creditService, 3)
	// User HTTP edge is deleted; the canonical redeem path is the v1
	// workspace surface (billing_gaps). Seed the redeemed fact through
	// the service seam so the admin delete-conflict below still covers
	// the redeemed-code guard.
	redeemed, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "HTTP-ROTATED-2026"})
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	if redeemed.CreditedAmount != 25 {
		t.Fatalf("redeem = %+v, want credited 25", redeemed)
	}
	_, redeemedDeleteRecorder := performRewardRequestWithRecorder[struct{}](
		t, adminRouter, http.MethodDelete, codePath, nil, http.StatusConflict,
	)
	if redeemedDeleteRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redeemed delete Cache-Control=%q", redeemedDeleteRecorder.Header().Get("Cache-Control"))
	}
}

type rewardHTTPResponse[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

// rewardMachineErrorResponse mirrors response.MachineErrorResponse: rewards
// failures always render here so callers branch on code, never on copy.
type rewardMachineErrorResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
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
	if wantStatus < 200 || wantStatus >= 300 {
		var machine rewardMachineErrorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &machine); err != nil {
			t.Fatalf("decode %s %s machine error: %v; body=%s", method, path, err, recorder.Body.String())
		}
		if machine.Code == "" {
			t.Fatalf("%s %s machine error missing code; body=%s", method, path, recorder.Body.String())
		}
		var zero T
		return zero, recorder
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
