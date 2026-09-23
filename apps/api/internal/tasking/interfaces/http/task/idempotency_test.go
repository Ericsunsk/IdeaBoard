package taskhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	tasks "ideaboard/api/internal/tasking/domain"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
	taskbillingports "ideaboard/api/internal/tasking/ports/billing"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

func submitWithIdempotencyKey(t *testing.T, router *gin.Engine, body map[string]interface{}, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := newJSONRequest(t, http.MethodPost, "/submit", body)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func fusionSubmitBody() map[string]interface{} {
	return map[string]interface{}{
		"prompt":     "融合但保留主体",
		"feature":    workflow.FeatureSlugFusion,
		"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
		"params": map[string]interface{}{
			"model":        "gpt-image-1",
			"output_count": 4,
			"fusion": map[string]interface{}{
				"input_weights": []float64{3, 1},
			},
		},
	}
}

func newFusionSubmitRouter(t testing.TB, store *recordingSubmissionStore) (*gin.Engine, *recordingSubmissionStore) {
	t.Helper()
	service := newSubmitServiceForTest(submitTestServiceDeps{
		ModelStore:      &fakeIntakeModelStore{},
		SubmissionStore: store,
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})
	return newSubmitHandlerTestRouter(t, service, 42), store
}

func TestSubmitTaskHandlerRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, store := newFusionSubmitRouter(t, &recordingSubmissionStore{})

	rec := submitWithIdempotencyKey(t, router, fusionSubmitBody(), "")
	assertMachineErrorResponse(t, rec, http.StatusBadRequest, "idempotency_key_required", "缺少幂等键，请重试此次操作")
	if len(store.calls) != 0 {
		t.Fatalf("submission calls=%d, want 0 (missing key must not execute)", len(store.calls))
	}
}

func TestSubmitTaskHandlerRejectsConflictingIdempotencyKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, store := newFusionSubmitRouter(t, &recordingSubmissionStore{})

	body := fusionSubmitBody()
	body["client_action_id"] = "body-action"
	rec := submitWithIdempotencyKey(t, router, body, "header-action")
	assertMachineErrorResponse(t, rec, http.StatusConflict, "idempotency_conflict", "幂等请求与原请求不一致")
	if len(store.calls) != 0 {
		t.Fatalf("submission calls=%d, want 0 (conflict must not execute)", len(store.calls))
	}
}

func TestSubmitTaskHandlerAcceptsHeaderOnlyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, store := newFusionSubmitRouter(t, &recordingSubmissionStore{})

	rec := submitWithIdempotencyKey(t, router, fusionSubmitBody(), "header-only-action")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(store.calls) != 1 || store.calls[0].canonical.ClientActionID() != "header-only-action" {
		t.Fatalf("canonical action=%+v, want header key", store.calls)
	}
}

func TestSubmitTaskHandlerAcceptsMatchingHeaderAndBodyKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, store := newFusionSubmitRouter(t, &recordingSubmissionStore{})

	body := fusionSubmitBody()
	body["client_action_id"] = "same-action"
	rec := submitWithIdempotencyKey(t, router, body, "same-action")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(store.calls) != 1 || store.calls[0].canonical.ClientActionID() != "same-action" {
		t.Fatalf("canonical action=%+v", store.calls)
	}
}

func TestSubmitTaskHandlerRejectsOversizedIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, _ := newFusionSubmitRouter(t, &recordingSubmissionStore{})

	rec := submitWithIdempotencyKey(t, router, fusionSubmitBody(), strings.Repeat("k", 129))
	assertMachineErrorResponse(t, rec, http.StatusBadRequest, "idempotency_key_required", "幂等键无效，请重试此次操作")
}

func TestSubmitTaskHandlerMapsBillingIdempotencyConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newSubmitServiceForTest(submitTestServiceDeps{
		ModelStore:      &fakeIntakeModelStore{},
		SubmissionStore: &recordingSubmissionStore{err: taskbillingports.ErrIdempotencyConflict},
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	body := fusionSubmitBody()
	body["client_action_id"] = "conflict-action"
	rec := submitWithIdempotencyKey(t, router, body, "conflict-action")
	assertMachineErrorResponse(t, rec, http.StatusConflict, "idempotency_conflict", "幂等请求与原请求不一致")
}

func newUserCancelTestRouter(t testing.TB, service *Service, userID int64) *gin.Engine {
	t.Helper()
	handler := NewHandler(HandlerDeps{Commands: service, AdminQueries: service, UserQueries: service})
	router := gin.New()
	router.POST("/tasks/:task_id/cancel", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, userID, principal.RoleUser)
		handler.CancelUserTask(c)
	})
	return router
}

func TestCancelUserTaskRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	task := &tasks.Task{
		TaskID:     "user-handler-cancel-key",
		UserID:     42,
		UserPrompt: "test",
		Prompt:     "test",
		Status:     tasks.StatusPending,
		Params:     "{}",
	}
	if err := createTaskServiceTestTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	service := NewService(ServiceDeps{TaskRepo: taskRepo, CancelStore: &fakeCancelStore{}}, ServiceOptions{})
	router := newUserCancelTestRouter(t, service, 42)

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/tasks/"+task.TaskID+"/cancel", nil))
	assertMachineErrorResponse(t, missing, http.StatusBadRequest, "idempotency_key_required", "缺少幂等键，请重试此次操作")

	keyed := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/tasks/"+task.TaskID+"/cancel", nil)
		req.Header.Set("Idempotency-Key", "user-cancel-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	first, replay := keyed(), keyed()
	if first.Code != http.StatusOK || replay.Code != http.StatusOK {
		t.Fatalf("statuses=(%d,%d), want both 200; first=%s replay=%s",
			first.Code, replay.Code, first.Body.String(), replay.Body.String())
	}
	if first.Body.String() != replay.Body.String() {
		t.Fatalf("replay body=%s, want first=%s", replay.Body.String(), first.Body.String())
	}
	conflictRequest := httptest.NewRequest(http.MethodPost, "/tasks/another-task/cancel", nil)
	conflictRequest.Header.Set("Idempotency-Key", "user-cancel-key")
	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, conflictRequest)
	assertMachineErrorResponse(t, conflict, http.StatusConflict, "idempotency_conflict", "幂等请求与原请求不一致")
}
