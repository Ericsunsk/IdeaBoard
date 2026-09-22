package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

// TestRewardFailuresRenderMachineCodes pins the single-track error contract:
// every rewards failure renders response.MachineError so callers branch on
// code, never on copy. Statuses and safe messages stay unchanged.
func TestRewardFailuresRenderMachineCodes(t *testing.T) {
	adminCommandCases := []struct {
		name       string
		method     string
		path       string
		body       string
		stub       rewardHandlerServiceStub
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:   "create invalid Code",
			method: http.MethodPost, path: "/admin/rewards/codes",
			body: `{"display_name":"Bad","code":"x","credit_amount":1,"idempotency_key":"k1"}`,
			stub: rewardHandlerServiceStub{create: func(context.Context, CreateRewardCodeCommand) (CreatedRewardCode, error) {
				return CreatedRewardCode{}, ErrInvalidCommand
			}},
			wantStatus: http.StatusBadRequest, wantCode: "reward_code_invalid", wantMsg: "参数错误",
		},
		{
			name:   "create idempotency conflict",
			method: http.MethodPost, path: "/admin/rewards/codes",
			body: `{"display_name":"Bad","code":"CONFLICT01","credit_amount":1,"idempotency_key":"k2"}`,
			stub: rewardHandlerServiceStub{create: func(context.Context, CreateRewardCodeCommand) (CreatedRewardCode, error) {
				return CreatedRewardCode{}, ErrRewardCodeConflict
			}},
			wantStatus: http.StatusConflict, wantCode: "reward_code_idempotency_conflict", wantMsg: "幂等请求与原创建参数不一致",
		},
		{
			name:   "create Code collision",
			method: http.MethodPost, path: "/admin/rewards/codes",
			body: `{"display_name":"Bad","code":"DUP00001","credit_amount":1,"idempotency_key":"k3"}`,
			stub: rewardHandlerServiceStub{create: func(context.Context, CreateRewardCodeCommand) (CreatedRewardCode, error) {
				return CreatedRewardCode{}, ErrCodeCollision
			}},
			wantStatus: http.StatusConflict, wantCode: "reward_code_conflict", wantMsg: "兑换码已存在",
		},
		{
			name:   "update Code not found",
			method: http.MethodPatch, path: "/admin/rewards/codes/7",
			body: `{"display_name":"Missing","credit_amount":1,"max_redemptions":0}`,
			stub: rewardHandlerServiceStub{update: func(context.Context, UpdateRewardCodeCommand) (UpdatedRewardCode, error) {
				return UpdatedRewardCode{}, ErrRewardCodeNotFound
			}},
			wantStatus: http.StatusNotFound, wantCode: "reward_code_not_found", wantMsg: "兑换码不存在",
		},
		{
			name:   "update cap below redemptions",
			method: http.MethodPatch, path: "/admin/rewards/codes/7",
			body: `{"display_name":"Cap","credit_amount":1,"max_redemptions":1}`,
			stub: rewardHandlerServiceStub{update: func(context.Context, UpdateRewardCodeCommand) (UpdatedRewardCode, error) {
				return UpdatedRewardCode{}, ErrCapBelowRedemptions
			}},
			wantStatus: http.StatusConflict, wantCode: "reward_code_cap_invalid", wantMsg: "兑换上限不能低于已兑换次数",
		},
		{
			name:   "delete redeemed Code",
			method: http.MethodDelete, path: "/admin/rewards/codes/7",
			body: ``,
			stub: rewardHandlerServiceStub{delete: func(context.Context, DeleteRewardCodeCommand) (DeletedRewardCode, error) {
				return DeletedRewardCode{}, ErrRewardCodeRedeemed
			}},
			wantStatus: http.StatusConflict, wantCode: "reward_code_redeemed", wantMsg: "已有兑换记录，只能停用该兑换码",
		},
		{
			name:   "delete unknown error stays internal",
			method: http.MethodDelete, path: "/admin/rewards/codes/7",
			body: ``,
			stub: rewardHandlerServiceStub{delete: func(context.Context, DeleteRewardCodeCommand) (DeletedRewardCode, error) {
				return DeletedRewardCode{}, errors.New("db connection refused")
			}},
			wantStatus: http.StatusInternalServerError, wantCode: "reward_operation_failed", wantMsg: "奖励操作失败",
		},
		{
			name:   "create malformed JSON",
			method: http.MethodPost, path: "/admin/rewards/codes",
			body: `{"display_name":`,
			stub: rewardHandlerServiceStub{create: func(context.Context, CreateRewardCodeCommand) (CreatedRewardCode, error) {
				t.Fatal("malformed request reached Rewards service")
				return CreatedRewardCode{}, nil
			}},
			wantStatus: http.StatusBadRequest, wantCode: "reward_invalid_input", wantMsg: "参数错误",
		},
		{
			name:   "update illegal Code id",
			method: http.MethodPatch, path: "/admin/rewards/codes/abc",
			body: `{"display_name":"Bad","credit_amount":1,"max_redemptions":0}`,
			stub: rewardHandlerServiceStub{update: func(context.Context, UpdateRewardCodeCommand) (UpdatedRewardCode, error) {
				t.Fatal("illegal id reached Rewards service")
				return UpdatedRewardCode{}, nil
			}},
			wantStatus: http.StatusBadRequest, wantCode: "reward_code_id_invalid", wantMsg: "兑换码 ID 无效",
		},
	}
	for _, testCase := range adminCommandCases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := NewAdminCommandHandler(testCase.stub)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
				c.Next()
			})
			router.POST("/admin/rewards/codes", handler.CreateRewardCode)
			router.PATCH("/admin/rewards/codes/:id", handler.UpdateRewardCode)
			router.DELETE("/admin/rewards/codes/:id", handler.DeleteRewardCode)

			recorder := performRawAdminRewardRequest(t, router, testCase.method, testCase.path, testCase.body)
			assertRewardMachineError(t, recorder, testCase.wantStatus, testCase.wantCode, testCase.wantMsg)
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}

	adminReadCases := []struct {
		name       string
		method     string
		path       string
		body       string
		handler    func(stub rewardHandlerServiceStub) http.Handler
		stub       rewardHandlerServiceStub
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:   "redemption not found",
			method: http.MethodGet, path: "/admin/rewards/codes/5/redemptions/9999", body: ``,
			handler: func(stub rewardHandlerServiceStub) http.Handler {
				return adminRedemptionDetailRouter(t, NewAdminReadHandler(stub, nil))
			},
			stub: rewardHandlerServiceStub{getRedemption: func(context.Context, GetAdminRedemptionInput) (AdminRedemptionDetail, error) {
				return AdminRedemptionDetail{}, ErrRedemptionNotFound
			}},
			wantStatus: http.StatusNotFound, wantCode: "reward_redemption_not_found", wantMsg: "兑换记录不存在",
		},
		{
			name:   "illegal redemption id",
			method: http.MethodGet, path: "/admin/rewards/codes/5/redemptions/0", body: ``,
			handler: func(stub rewardHandlerServiceStub) http.Handler {
				return adminRedemptionDetailRouter(t, NewAdminReadHandler(stub, nil))
			},
			stub:       rewardHandlerServiceStub{},
			wantStatus: http.StatusBadRequest, wantCode: "reward_redemption_id_invalid", wantMsg: "兑换记录 ID 无效",
		},
		{
			name:   "illegal pagination limit",
			method: http.MethodGet, path: "/admin/rewards/codes/5/redemptions?limit=101", body: ``,
			handler: func(stub rewardHandlerServiceStub) http.Handler {
				return adminRedemptionListRouter(t, NewAdminReadHandler(stub, nil))
			},
			stub:       rewardHandlerServiceStub{},
			wantStatus: http.StatusBadRequest, wantCode: "reward_pagination_invalid", wantMsg: "limit 无效",
		},
		{
			name:   "search rejects URL input",
			method: http.MethodPost, path: "/admin/rewards/codes/search?query=x", body: `{}`,
			handler: func(stub rewardHandlerServiceStub) http.Handler {
				return adminRewardSearchRouter(t, NewAdminReadHandler(stub, nil))
			},
			stub:       rewardHandlerServiceStub{},
			wantStatus: http.StatusBadRequest, wantCode: "reward_invalid_input", wantMsg: "参数错误",
		},
	}
	for _, testCase := range adminReadCases {
		t.Run(testCase.name, func(t *testing.T) {
			router := testCase.handler(testCase.stub)
			var recorder *httptest.ResponseRecorder
			if testCase.method == http.MethodGet {
				request := httptest.NewRequest(testCase.method, testCase.path, nil)
				recorder = httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
			} else {
				recorder = performRawAdminRewardRequest(t, router, testCase.method, testCase.path, testCase.body)
			}
			assertRewardMachineError(t, recorder, testCase.wantStatus, testCase.wantCode, testCase.wantMsg)
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}

	redeemCases := []struct {
		name       string
		redeemErr  error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{name: "invalid Code", redeemErr: ErrInvalidCommand, wantStatus: http.StatusUnprocessableEntity, wantCode: "reward_code_unavailable", wantMsg: "兑换码无效或已失效"},
		{name: "unavailable Code", redeemErr: ErrCodeUnavailable, wantStatus: http.StatusUnprocessableEntity, wantCode: "reward_code_unavailable", wantMsg: "兑换码无效或已失效"},
		{name: "frozen Wallet", redeemErr: rewardsdomain.ErrWalletFrozen, wantStatus: http.StatusConflict, wantCode: "reward_wallet_locked", wantMsg: "当前账户无法兑换奖励"},
		{name: "unknown failure", redeemErr: errors.New("credit grant timeout"), wantStatus: http.StatusInternalServerError, wantCode: "reward_redeem_failed", wantMsg: "网络错误，请重试"},
	}
	for _, testCase := range redeemCases {
		t.Run("redeem "+testCase.name, func(t *testing.T) {
			handler := NewUserHandler(rewardHandlerServiceStub{redeem: func(context.Context, RedeemCommand) (RedeemResult, error) {
				return RedeemResult{}, testCase.redeemErr
			}})
			router := rewardRedeemRouter(t, handler, 41)
			recorder := performRawRewardRedeem(t, router, `{"code":"MACHINE01"}`)
			assertRewardMachineError(t, recorder, testCase.wantStatus, testCase.wantCode, testCase.wantMsg)
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

func TestRewardUnavailableServiceRendersMachineCode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("user redeem without service", func(t *testing.T) {
		router := rewardRedeemRouter(t, NewUserHandler(nil), 41)
		recorder := performRawRewardRedeem(t, router, `{"code":"MACHINE01"}`)
		assertRewardMachineError(t, recorder, http.StatusServiceUnavailable, "reward_service_unavailable", "网络错误，请重试")
	})

	t.Run("admin command without service", func(t *testing.T) {
		handler := NewAdminCommandHandler(nil)
		router := gin.New()
		router.Use(func(c *gin.Context) {
			testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
			c.Next()
		})
		router.POST("/admin/rewards/codes", handler.CreateRewardCode)
		recorder := performRawAdminRewardRequest(t, router, http.MethodPost, "/admin/rewards/codes", `{"display_name":"x"}`)
		assertRewardMachineError(t, recorder, http.StatusServiceUnavailable, "reward_service_unavailable", "奖励服务暂不可用")
	})
}

func TestRewardRedeemThrottleRendersMachineCode(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	handler := newUserHandlerWithOptions(rewardHandlerServiceStub{redeem: func(context.Context, RedeemCommand) (RedeemResult, error) {
		return RedeemResult{CreditedAmount: 1}, nil
	}}, redemptionHandlerOptions{Now: func() time.Time { return now }})
	router := rewardRedeemRouter(t, handler, 41)
	for attempt := 1; attempt <= defaultRedemptionAttemptLimit; attempt++ {
		if recorder := performRawRewardRedeemWithKey(t, router, `{"code":"MACHINE01"}`, fmt.Sprintf("machine-key-%d", attempt)); recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d; body=%s", attempt, recorder.Code, recorder.Body.String())
		}
	}
	limited := performRawRewardRedeem(t, router, `{"code":"MACHINE01"}`)
	assertRewardMachineError(t, limited, http.StatusTooManyRequests, "reward_rate_limited", "操作频繁，请稍后重试")
}

func assertRewardMachineError(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode, wantMsg string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var payload struct {
		Code        string `json:"code"`
		Msg         string `json:"msg"`
		MessageSafe bool   `json:"message_safe"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode machine error: %v; body=%s", err, recorder.Body.String())
	}
	if payload.Code != wantCode || payload.Msg != wantMsg || !payload.MessageSafe {
		t.Fatalf("machine error = %+v, want code=%q msg=%q message_safe=true", payload, wantCode, wantMsg)
	}
}

func adminRedemptionListRouter(t *testing.T, handler *AdminReadHandler) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/rewards/codes/:id/redemptions", handler.ListAdminRedemptions)
	return router
}

func adminRedemptionDetailRouter(t *testing.T, handler *AdminReadHandler) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/rewards/codes/:id/redemptions/:redemption_id", handler.GetAdminRedemption)
	return router
}

func adminRewardSearchRouter(t *testing.T, handler *AdminReadHandler) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/admin/rewards/codes/search", handler.SearchRewardCodes)
	return router
}
