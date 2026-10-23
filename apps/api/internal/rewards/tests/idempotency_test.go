package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/rewards"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type idempotencyMachineErrorResponse struct {
	Code string `json:"code"`
}

// performRewardWriteWithKey mirrors performRewardRequestWithRecorder and
// attaches an Idempotency-Key header for POST write contract coverage.
func performRewardWriteWithKey[T any](
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body any,
	key string,
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
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, recorder.Code, wantStatus, recorder.Body.String())
	}
	if wantStatus < 200 || wantStatus >= 300 {
		var machine idempotencyMachineErrorResponse
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
	return response.Data, recorder
}

func assertRewardMachineCode(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var machine idempotencyMachineErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &machine); err != nil {
		t.Fatalf("decode machine error: %v; body=%s", err, recorder.Body.String())
	}
	if machine.Code != wantCode {
		t.Fatalf("code = %q, want %q; body=%s", machine.Code, wantCode, recorder.Body.String())
	}
}

func newIdempotencyAdminRouter(t *testing.T, handler *rewards.AdminCommandHandler) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/admin/rewards/codes", handler.CreateRewardCode)
	return router
}

func TestRewardCreateRequiresIdempotencyKey(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	router := newIdempotencyAdminRouter(t, rewardhttp.NewAdminCommandHandler(service))

	_, recorder := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, router, http.MethodPost, "/admin/rewards/codes", map[string]any{
			"display_name": "Missing key", "code": "MISSING-KEY-2026",
			"credit_amount": 5, "max_redemptions": 0,
		}, "", http.StatusBadRequest,
	)
	assertRewardMachineCode(t, recorder, http.StatusBadRequest, "reward_invalid_input")
}

func TestRewardCreateRejectsBodyIdempotencyKey(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	router := newIdempotencyAdminRouter(t, rewardhttp.NewAdminCommandHandler(service))

	// Header-only: a body idempotency_key is not a fallback — strict
	// decoding rejects it as an unknown field even with a valid header.
	_, recorder := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, router, http.MethodPost, "/admin/rewards/codes", map[string]any{
			"display_name": "Body key", "code": "BODY-KEY-2026",
			"credit_amount": 5, "max_redemptions": 0, "idempotency_key": "body-key",
		}, "header-key", http.StatusBadRequest,
	)
	assertRewardMachineCode(t, recorder, http.StatusBadRequest, "reward_invalid_input")

	// The same payload without the body key succeeds on the header alone.
	created, _ := performRewardWriteWithKey[ports.CreatedRewardCode](
		t, router, http.MethodPost, "/admin/rewards/codes", map[string]any{
			"display_name": "Body key", "code": "BODY-KEY-2026",
			"credit_amount": 5, "max_redemptions": 0,
		}, "header-key", http.StatusOK,
	)
	if created.RewardCode.ID <= 0 {
		t.Fatalf("header-only create id=%d, want positive", created.RewardCode.ID)
	}
}

func TestRewardCreateReplaysWithHeaderKey(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	router := newIdempotencyAdminRouter(t, rewardhttp.NewAdminCommandHandler(service))

	create := func(key string) (ports.CreatedRewardCode, *httptest.ResponseRecorder) {
		return performRewardWriteWithKey[ports.CreatedRewardCode](
			t, router, http.MethodPost, "/admin/rewards/codes", map[string]any{
				"display_name": "Header replay", "code": "HEADER-REPLAY-2026",
				"credit_amount": 7, "max_redemptions": 0,
			}, key, http.StatusOK,
		)
	}
	first, _ := create("header-replay-key")
	replay, _ := create("header-replay-key")
	if replay.RewardCode.ID != first.RewardCode.ID {
		t.Fatalf("replay id=%d, want first id=%d", replay.RewardCode.ID, first.RewardCode.ID)
	}
	if first.Replayed || !replay.Replayed {
		t.Fatalf("replayed flags first=%v replay=%v, want false/true", first.Replayed, replay.Replayed)
	}
}
