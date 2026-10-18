package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/rewards"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRewardRedeemDoesNotFailAfterCommittedResultWhenIdentityIsUnavailable(t *testing.T) {
	for _, replayed := range []bool{false, true} {
		t.Run(fmt.Sprintf("replayed=%t", replayed), func(t *testing.T) {
			handler := rewardhttp.NewUserHandler(rewardHandlerServiceStub{redeem: func(context.Context, ports.RedeemCommand) (ports.RedeemResult, error) {
				return ports.RedeemResult{
					Redemption:       ports.Redemption{ID: 9, UserID: 41, CreditAmount: 25},
					CreditedAmount:   25,
					AvailableBalance: 125,
					Replayed:         replayed,
				}, nil
			}})
			router := rewardRedeemRouter(t, handler, 41)

			recorder := performRawRewardRedeem(t, router, `{"code":"SECURE01"}`)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
		})
	}
}

type rewardHandlerServiceStub struct {
	create          func(context.Context, ports.CreateRewardCodeCommand) (ports.CreatedRewardCode, error)
	setEnabled      func(context.Context, ports.SetRewardCodeEnabledCommand) (ports.RewardCode, error)
	delete          func(context.Context, ports.DeleteRewardCodeCommand) (ports.DeletedRewardCode, error)
	redeem          func(context.Context, ports.RedeemCommand) (ports.RedeemResult, error)
	search          func(context.Context, ports.SearchRewardCodesInput) (ports.RewardCodePage, error)
	update          func(context.Context, ports.UpdateRewardCodeCommand) (ports.UpdatedRewardCode, error)
	listRedemptions func(context.Context, ports.ListAdminRedemptionsInput) (ports.AdminRedemptionPage, error)
	getRedemption   func(context.Context, ports.GetAdminRedemptionInput) (ports.AdminRedemptionDetail, error)
	listUser        func(context.Context, ports.ListRedemptionsInput) (ports.RedemptionHistoryPage, error)
}

func (s rewardHandlerServiceStub) CreateRewardCode(ctx context.Context, input ports.CreateRewardCodeCommand) (ports.CreatedRewardCode, error) {
	if s.create == nil {
		return ports.CreatedRewardCode{}, nil
	}
	return s.create(ctx, input)
}

func (s rewardHandlerServiceStub) SetRewardCodeEnabled(ctx context.Context, input ports.SetRewardCodeEnabledCommand) (ports.RewardCode, error) {
	if s.setEnabled == nil {
		return ports.RewardCode{}, nil
	}
	return s.setEnabled(ctx, input)
}

func (s rewardHandlerServiceStub) DeleteRewardCode(ctx context.Context, input ports.DeleteRewardCodeCommand) (ports.DeletedRewardCode, error) {
	if s.delete == nil {
		return ports.DeletedRewardCode{}, nil
	}
	return s.delete(ctx, input)
}

func (s rewardHandlerServiceStub) SearchRewardCodes(
	ctx context.Context,
	input ports.SearchRewardCodesInput,
) (ports.RewardCodePage, error) {
	if s.search == nil {
		return ports.RewardCodePage{}, nil
	}
	return s.search(ctx, input)
}

func (s rewardHandlerServiceStub) UpdateRewardCode(
	ctx context.Context,
	command ports.UpdateRewardCodeCommand,
) (ports.UpdatedRewardCode, error) {
	if s.update == nil {
		return ports.UpdatedRewardCode{}, nil
	}
	return s.update(ctx, command)
}

func (s rewardHandlerServiceStub) Redeem(
	ctx context.Context,
	command ports.RedeemCommand,
) (ports.RedeemResult, error) {
	if s.redeem == nil {
		return ports.RedeemResult{}, nil
	}
	return s.redeem(ctx, command)
}

func (s rewardHandlerServiceStub) ListAdminRedemptions(
	ctx context.Context,
	input ports.ListAdminRedemptionsInput,
) (ports.AdminRedemptionPage, error) {
	if s.listRedemptions == nil {
		return ports.AdminRedemptionPage{}, nil
	}
	return s.listRedemptions(ctx, input)
}

func (s rewardHandlerServiceStub) GetAdminRedemption(ctx context.Context, input ports.GetAdminRedemptionInput) (ports.AdminRedemptionDetail, error) {
	if s.getRedemption == nil {
		return ports.AdminRedemptionDetail{}, nil
	}
	return s.getRedemption(ctx, input)
}

func (s rewardHandlerServiceStub) ListRedemptions(ctx context.Context, input ports.ListRedemptionsInput) (ports.RedemptionHistoryPage, error) {
	if s.listUser == nil {
		return ports.RedemptionHistoryPage{}, nil
	}
	return s.listUser(ctx, input)
}

type recordingIdentityProjection struct {
	identities  map[int64]rewards.IdentityProjection
	singleCalls int
	batchCalls  int
}

func (p *recordingIdentityProjection) ResolveIdentity(_ context.Context, userID int64) (rewards.IdentityProjection, error) {
	p.singleCalls++
	identity, ok := p.identities[userID]
	if !ok {
		return rewards.IdentityProjection{}, errors.New("Identity mapping missing")
	}
	return identity, nil
}

func (p *recordingIdentityProjection) ResolveIdentities(
	_ context.Context,
	_ []int64,
) (map[int64]rewards.IdentityProjection, error) {
	p.batchCalls++
	return p.identities, nil
}

func TestAdminRedemptionListUsesOneBatchIdentityProjection(t *testing.T) {
	identities := map[int64]rewards.IdentityProjection{
		3: {PublicID: uuid.MustParse("00000000-0000-4000-8000-000000000003"), DisplayName: "User 3"},
		7: {PublicID: uuid.MustParse("00000000-0000-4000-8000-000000000007"), DisplayName: "User 7"},
		8: {PublicID: uuid.MustParse("00000000-0000-4000-8000-000000000008"), DisplayName: "User 8"},
	}
	projection := &recordingIdentityProjection{identities: identities}
	service := rewardHandlerServiceStub{listRedemptions: func(
		context.Context,
		ports.ListAdminRedemptionsInput,
	) (ports.AdminRedemptionPage, error) {
		return ports.AdminRedemptionPage{Redemptions: []ports.AdminRedemptionSummary{
			{ID: 13, RewardCodeID: 5, UserID: 3},
			{ID: 12, RewardCodeID: 5, UserID: 7},
			{ID: 11, RewardCodeID: 5, UserID: 8},
		}}, nil
	}}
	handler := rewardhttp.NewAdminReadHandler(service, projection)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/rewards/codes/:id/redemptions", handler.ListAdminRedemptions)

	page, _ := performRewardRequestWithRecorder[ports.AdminRedemptionPage](
		t, router, http.MethodGet, "/admin/rewards/codes/5/redemptions", nil, http.StatusOK,
	)

	if projection.batchCalls != 1 || projection.singleCalls != 0 {
		t.Fatalf("Identity projection calls: batch=%d single=%d, want 1 and 0",
			projection.batchCalls, projection.singleCalls)
	}
	if len(page.Redemptions) != 3 {
		t.Fatalf("Redemptions = %d, want 3", len(page.Redemptions))
	}
	for index, userID := range []int64{3, 7, 8} {
		redemption := page.Redemptions[index]
		identity := identities[userID]
		if redemption.UserPublicID != identity.PublicID || redemption.DisplayName != identity.DisplayName {
			t.Fatalf("Redemption projection = %+v, want %+v", redemption, identity)
		}
	}
}

func TestAdminRedemptionListFailsClosedForMissingIdentityMapping(t *testing.T) {
	projection := &recordingIdentityProjection{identities: map[int64]rewards.IdentityProjection{}}
	service := rewardHandlerServiceStub{listRedemptions: func(
		context.Context,
		ports.ListAdminRedemptionsInput,
	) (ports.AdminRedemptionPage, error) {
		return ports.AdminRedemptionPage{Redemptions: []ports.AdminRedemptionSummary{{ID: 13, RewardCodeID: 5, UserID: 3}}}, nil
	}}
	handler := rewardhttp.NewAdminReadHandler(service, projection)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.GET("/admin/rewards/codes/:id/redemptions", handler.ListAdminRedemptions)

	_, recorder := performRewardRequestWithRecorder[struct{}](
		t, router, http.MethodGet, "/admin/rewards/codes/5/redemptions", nil, http.StatusInternalServerError,
	)

	if projection.batchCalls != 1 || projection.singleCalls != 0 ||
		!strings.Contains(recorder.Body.String(), "奖励操作失败") {
		t.Fatalf("Identity projection calls: batch=%d single=%d body=%s",
			projection.batchCalls, projection.singleCalls, recorder.Body.String())
	}
}

func TestRewardRedeemRequiresAuthenticationAndDisablesCaching(t *testing.T) {
	handler := rewardhttp.NewUserHandler(rewardHandlerServiceStub{redeem: func(context.Context, ports.RedeemCommand) (ports.RedeemResult, error) {
		t.Fatal("unauthenticated request reached Rewards service")
		return ports.RedeemResult{}, nil
	}})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/user/rewards/redeem", handler.Redeem)

	recorder := performRawRewardRedeem(t, router, `{"code":"SECURE01"}`)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestRewardRedeemCannotTargetAnotherUserFromRequestBody(t *testing.T) {
	handler := rewardhttp.NewUserHandler(rewardHandlerServiceStub{redeem: func(context.Context, ports.RedeemCommand) (ports.RedeemResult, error) {
		t.Fatal("request containing a target user reached Rewards service")
		return ports.RedeemResult{}, nil
	}})
	router := rewardRedeemRouter(t, handler, 41)

	recorder := performRawRewardRedeem(t, router, `{"code":"SECURE01","user_id":42}`)

	assertRewardErrorResponse(t, recorder, http.StatusBadRequest, "参数错误")
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestRewardRedeemLimitsEachAuthenticatedUser(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	serviceCalls := 0
	service := rewardHandlerServiceStub{redeem: func(context.Context, ports.RedeemCommand) (ports.RedeemResult, error) {
		serviceCalls++
		return ports.RedeemResult{CreditedAmount: 1}, nil
	}}
	handler := rewardhttp.NewUserHandlerWithOptions(service, rewards.RedemptionHandlerOptions{
		Now: func() time.Time { return now },
	})

	user41 := rewardRedeemRouter(t, handler, 41)
	for attempt := 1; attempt <= rewards.DefaultRedemptionAttemptLimit; attempt++ {
		recorder := performRawRewardRedeemWithKey(t, user41, `{"code":"SECURE01"}`, fmt.Sprintf("secure-key-%d", attempt))
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d; body=%s", attempt, recorder.Code, recorder.Body.String())
		}
	}
	limited := performRawRewardRedeemWithKey(t, user41, `{"code":"SECURE01"}`, "secure-key-limited")
	assertRewardErrorResponse(t, limited, http.StatusTooManyRequests, "操作过于频繁，请稍后重试")
	if got := limited.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("rate-limited Cache-Control = %q, want no-store", got)
	}
	if serviceCalls != rewards.DefaultRedemptionAttemptLimit {
		t.Fatalf("service calls = %d, want %d", serviceCalls, rewards.DefaultRedemptionAttemptLimit)
	}

	user42 := rewardRedeemRouter(t, handler, 42)
	if recorder := performRawRewardRedeem(t, user42, `{"code":"SECURE01"}`); recorder.Code != http.StatusOK {
		t.Fatalf("other user status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRewardRedeemDoesNotLogOrReturnSensitiveCodeMaterial(t *testing.T) {
	const rawCode = "SECRET294"
	const fullDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	requestBody := `{"code":"` + rawCode + `"}`
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	service := rewardHandlerServiceStub{redeem: func(context.Context, ports.RedeemCommand) (ports.RedeemResult, error) {
		return ports.RedeemResult{}, errors.New("lookup failed code=" + rawCode + " digest=" + fullDigest + " body=" + requestBody)
	}}
	handler := rewardhttp.NewUserHandler(service)
	router := rewardRedeemRouter(t, handler, 41)

	responseRecorder := performRawRewardRedeem(t, router, requestBody)
	assertRewardErrorResponse(t, responseRecorder, http.StatusInternalServerError, "网络错误，请重试")

	observable := logs.String() + responseRecorder.Body.String()
	for _, forbidden := range []string{rawCode, fullDigest, requestBody} {
		if strings.Contains(observable, forbidden) {
			t.Fatalf("logs or response exposed sensitive value %q: %s", forbidden, observable)
		}
	}
}

func TestRewardCodeUpdateWithCodeRequiresAdminAndDoesNotExposeSensitiveMaterial(t *testing.T) {
	const rawCode = "ADMIN-SECRET-294"
	const fullDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	requestBody := `{"code":"` + rawCode + `"}`
	serviceCalls := 0
	service := rewardHandlerServiceStub{update: func(context.Context, ports.UpdateRewardCodeCommand) (ports.UpdatedRewardCode, error) {
		serviceCalls++
		return ports.UpdatedRewardCode{}, errors.New("rotation failed code=" + rawCode + " digest=" + fullDigest)
	}}
	handler := rewardhttp.NewAdminCommandHandler(service)
	gin.SetMode(gin.TestMode)
	unauthenticated := gin.New()
	unauthenticated.PATCH("/admin/rewards/codes/:id", handler.UpdateRewardCode)
	responseRecorder := performRawAdminRewardRequest(t, unauthenticated, http.MethodPatch,
		"/admin/rewards/codes/1", requestBody)
	if responseRecorder.Code != http.StatusUnauthorized || serviceCalls != 0 {
		t.Fatalf("unauthenticated rotate status=%d calls=%d", responseRecorder.Code, serviceCalls)
	}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	admin := gin.New()
	admin.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	admin.PATCH("/admin/rewards/codes/:id", handler.UpdateRewardCode)
	responseRecorder = performRawAdminRewardRequest(t, admin, http.MethodPatch,
		"/admin/rewards/codes/1", `{"display_name":"Secure","credit_amount":1,"max_redemptions":0,"code":"`+rawCode+`"}`)
	assertRewardErrorResponse(t, responseRecorder, http.StatusInternalServerError, "奖励操作失败")
	if responseRecorder.Header().Get("Cache-Control") != "no-store" || serviceCalls != 1 {
		t.Fatalf("admin rotate Cache-Control=%q calls=%d", responseRecorder.Header().Get("Cache-Control"), serviceCalls)
	}
	observable := logs.String() + responseRecorder.Body.String()
	for _, forbidden := range []string{rawCode, fullDigest, requestBody} {
		if strings.Contains(observable, forbidden) {
			t.Fatalf("rotate logs or response exposed sensitive value %q: %s", forbidden, observable)
		}
	}
}

func TestRewardCodeSearchDoesNotLogOrReturnSensitiveCodeMaterial(t *testing.T) {
	const rawCode = "ADMIN-LOOKUP-294"
	const fullDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	service := rewardHandlerServiceStub{search: func(_ context.Context, input ports.SearchRewardCodesInput) (ports.RewardCodePage, error) {
		if input.Query != rawCode {
			t.Fatalf("search query = %q, want request body Code", input.Query)
		}
		return ports.RewardCodePage{}, errors.New("lookup failed code=" + rawCode + " digest=" + fullDigest)
	}}
	handler := rewardhttp.NewAdminReadHandler(service, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/admin/rewards/codes/search", handler.SearchRewardCodes)
	responseRecorder := performRawAdminRewardRequest(
		t, router, http.MethodPost, "/admin/rewards/codes/search", `{"query":"`+rawCode+`"}`,
	)
	assertRewardErrorResponse(t, responseRecorder, http.StatusInternalServerError, "奖励操作失败")
	if responseRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("search Cache-Control=%q", responseRecorder.Header().Get("Cache-Control"))
	}
	observable := logs.String() + responseRecorder.Body.String()
	for _, forbidden := range []string{rawCode, fullDigest} {
		if strings.Contains(observable, forbidden) {
			t.Fatalf("search logs or response exposed sensitive value %q: %s", forbidden, observable)
		}
	}
}

func TestRewardCodeManagementRejectsUnknownJSONFields(t *testing.T) {
	service := rewardHandlerServiceStub{update: func(context.Context, ports.UpdateRewardCodeCommand) (ports.UpdatedRewardCode, error) {
		t.Fatal("invalid management request reached Rewards service")
		return ports.UpdatedRewardCode{}, nil
	}}
	handler := rewardhttp.NewAdminCommandHandler(service)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.PATCH("/admin/rewards/codes/:id", handler.UpdateRewardCode)
	recorder := performRawAdminRewardRequest(t, router, http.MethodPatch,
		"/admin/rewards/codes/1", `{"display_name":"Secure","credit_amount":1,"max_redemptions":0,"code":"SECURE01","user_id":42}`)
	assertRewardErrorResponse(t, recorder, http.StatusBadRequest, "参数错误")
}

func rewardRedeemRouter(t *testing.T, handler *rewards.UserHandler, userID int64) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, userID, principal.RoleUser)
		c.Next()
	})
	router.POST("/user/rewards/redeem", handler.Redeem)
	return router
}

func performRawRewardRedeem(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	return performRawRewardRedeemWithKey(t, router, body, "security-test-key")
}

func performRawRewardRedeemWithKey(t *testing.T, router http.Handler, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/user/rewards/redeem", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func performRawAdminRewardRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	// Header-only idempotency: admin create requires the Idempotency-Key
	// header on the wire; the body carries no key.
	request.Header.Set("Idempotency-Key", "test-admin-key")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertRewardErrorResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantMessage string,
) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var payload struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if payload.Msg != wantMessage {
		t.Fatalf("message = %q, want %q", payload.Msg, wantMessage)
	}
}
