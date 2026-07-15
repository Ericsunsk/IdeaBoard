package rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type rewardHandlerServiceStub struct {
	RewardService
	redeem func(context.Context, RedeemCommand) (RedeemResult, error)
}

func (s rewardHandlerServiceStub) Redeem(
	ctx context.Context,
	command RedeemCommand,
) (RedeemResult, error) {
	return s.redeem(ctx, command)
}

type captureRedemptionMetrics struct {
	mu           sync.Mutex
	observations []redemptionMetricObservation
}

func (c *captureRedemptionMetrics) RecordRedemption(observation redemptionMetricObservation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observations = append(c.observations, observation)
}

func TestRewardRedeemRequiresAuthenticationAndDisablesCaching(t *testing.T) {
	handler := NewHandler(rewardHandlerServiceStub{redeem: func(context.Context, RedeemCommand) (RedeemResult, error) {
		t.Fatal("unauthenticated request reached Rewards service")
		return RedeemResult{}, nil
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
	handler := NewHandler(rewardHandlerServiceStub{redeem: func(context.Context, RedeemCommand) (RedeemResult, error) {
		t.Fatal("request containing a target user reached Rewards service")
		return RedeemResult{}, nil
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
	metricsRecorder := &captureRedemptionMetrics{}
	serviceCalls := 0
	service := rewardHandlerServiceStub{redeem: func(context.Context, RedeemCommand) (RedeemResult, error) {
		serviceCalls++
		return RedeemResult{CreditedAmount: 1}, nil
	}}
	handler := newHandlerWithOptions(service, redemptionHandlerOptions{
		Now:             func() time.Time { return now },
		MetricsRecorder: metricsRecorder,
	})

	user41 := rewardRedeemRouter(t, handler, 41)
	for attempt := 1; attempt <= defaultRedemptionAttemptLimit; attempt++ {
		recorder := performRawRewardRedeem(t, user41, `{"code":"SECURE01"}`)
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d; body=%s", attempt, recorder.Code, recorder.Body.String())
		}
	}
	limited := performRawRewardRedeem(t, user41, `{"code":"SECURE01"}`)
	assertRewardErrorResponse(t, limited, http.StatusTooManyRequests, "操作频繁，请稍后重试")
	if got := limited.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("rate-limited Cache-Control = %q, want no-store", got)
	}
	if serviceCalls != defaultRedemptionAttemptLimit {
		t.Fatalf("service calls = %d, want %d", serviceCalls, defaultRedemptionAttemptLimit)
	}
	if got := metricsRecorder.observations[defaultRedemptionAttemptLimit].Outcome; got != redemptionOutcomeRateLimited {
		t.Fatalf("limited request outcome = %q, want %q", got, redemptionOutcomeRateLimited)
	}

	user42 := rewardRedeemRouter(t, handler, 42)
	if recorder := performRawRewardRedeem(t, user42, `{"code":"SECURE01"}`); recorder.Code != http.StatusOK {
		t.Fatalf("other user status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRewardRedeemRecordsLowCardinalityOutcomes(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	recorder := &captureRedemptionMetrics{}
	service := rewardHandlerServiceStub{redeem: func(_ context.Context, command RedeemCommand) (RedeemResult, error) {
		now = now.Add(25 * time.Millisecond)
		switch command.Code {
		case "SUCCESS1":
			return RedeemResult{CreditedAmount: 25}, nil
		case "REPLAY01":
			return RedeemResult{CreditedAmount: 25, Replayed: true}, nil
		case "INVALID1":
			return RedeemResult{}, ErrCodeUnavailable
		case "BADINPUT":
			return RedeemResult{}, ErrInvalidCommand
		case "ALREADY1":
			return RedeemResult{}, ErrAlreadyRedeemed
		case "EXHAUST1":
			return RedeemResult{}, ErrCampaignExhausted
		default:
			return RedeemResult{}, errors.New("database unavailable")
		}
	}}
	handler := newHandlerWithOptions(service, redemptionHandlerOptions{
		Now:             func() time.Time { return now },
		MetricsRecorder: recorder,
	})
	router := rewardRedeemRouter(t, handler, 41)

	for _, code := range []string{"SUCCESS1", "REPLAY01", "INVALID1", "BADINPUT", "ALREADY1", "EXHAUST1", "INTERNAL"} {
		performRawRewardRedeem(t, router, `{"code":"`+code+`"}`)
	}

	wantOutcomes := []redemptionOutcome{
		redemptionOutcomeSuccess,
		redemptionOutcomeReplayed,
		redemptionOutcomeInvalid,
		redemptionOutcomeInvalid,
		redemptionOutcomeAlreadyRedeemed,
		redemptionOutcomeExhausted,
		redemptionOutcomeInternalError,
	}
	if len(recorder.observations) != len(wantOutcomes) {
		t.Fatalf("observations = %+v, want %d", recorder.observations, len(wantOutcomes))
	}
	for index, want := range wantOutcomes {
		observation := recorder.observations[index]
		if observation.Outcome != want || observation.Latency != 25*time.Millisecond {
			t.Fatalf("observation %d = %+v, want outcome=%q latency=25ms", index, observation, want)
		}
	}
	if recorder.observations[0].CreditedCredits != 25 {
		t.Fatalf("success credited Credits = %d, want 25", recorder.observations[0].CreditedCredits)
	}
	if recorder.observations[1].CreditedCredits != 0 {
		t.Fatalf("replay credited Credits = %d, want 0", recorder.observations[1].CreditedCredits)
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

	service := rewardHandlerServiceStub{redeem: func(context.Context, RedeemCommand) (RedeemResult, error) {
		return RedeemResult{}, errors.New("lookup failed code=" + rawCode + " digest=" + fullDigest + " body=" + requestBody)
	}}
	handler := NewHandler(service)
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

func rewardRedeemRouter(t *testing.T, handler *Handler, userID int64) http.Handler {
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
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/user/rewards/redeem", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
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
