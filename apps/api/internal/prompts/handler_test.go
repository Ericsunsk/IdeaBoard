package prompts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
)

func TestSetPromptRequiresPromptField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(db))
	router := gin.New()
	router.PUT("/admin/feature-prompts/:slug", handler.SetPrompt)

	req := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/partial-edit", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestSetPromptRejectsInvalidModelDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(db))
	router := gin.New()
	router.PUT("/admin/feature-prompts/:slug", handler.SetPrompt)

	cases := []struct {
		name string
		body string
	}{
		{name: "empty model", body: `{"prompt":"hello","model_defaults":{"model":""}}`},
		{name: "blank model", body: `{"prompt":"hello","model_defaults":{"model":"   "}}`},
		{name: "non string model", body: `{"prompt":"hello","model_defaults":{"model":123}}`},
		{name: "unknown key", body: `{"prompt":"hello","model_defaults":{"foo":"bar"}}`},
		{name: "non object", body: `{"prompt":"hello","model_defaults":[]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/partial-edit", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestSetPromptAcceptsTypedModelDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(db))
	router := gin.New()
	router.PUT("/admin/feature-prompts/:slug", handler.SetPrompt)

	req := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/partial-edit", strings.NewReader(`{"prompt":"hello","model_defaults":{"model":" image-alpha "}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestSetPromptBlocksFailingPromptEval(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(db))
	router := gin.New()
	router.PUT("/admin/feature-prompts/:slug", handler.SetPrompt)

	req := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/pattern-gen", strings.NewReader(`{"prompt":"Create {{missing_token}}","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"passed":false`) {
		t.Fatalf("body = %s, want failing eval report", rec.Body.String())
	}
}

func TestRollbackPromptMovesCurrentRevisionThroughCommandPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	repo := NewFeaturePromptRepository(db)
	handler := NewFeaturePromptHandler(repo)
	router := gin.New()
	router.POST("/admin/feature-prompts/:slug/rollback", handler.RollbackPrompt)

	first, err := repo.SetPromptConfig(context.Background(), workflow.FeatureSlugImageExtend, FeaturePromptSaveInput{
		Prompt:    "prompt v1 {{user_prompt}}",
		Enabled:   true,
		Variables: promptVariables("{{user_prompt}}"),
	}, nil)
	if err != nil {
		t.Fatalf("SetPromptConfig first returned error: %v", err)
	}
	if _, err := repo.SetPromptConfig(context.Background(), workflow.FeatureSlugImageExtend, FeaturePromptSaveInput{
		Prompt:    "prompt v2 {{user_prompt}}",
		Enabled:   true,
		Variables: promptVariables("{{user_prompt}}"),
	}, nil); err != nil {
		t.Fatalf("SetPromptConfig second returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/feature-prompts/image-extend/rollback", strings.NewReader(`{"revision_id":`+int64String(first.CurrentRevisionID)+`}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	config, err := repo.GetPromptConfig(context.Background(), workflow.FeatureSlugImageExtend)
	if err != nil {
		t.Fatalf("GetPromptConfig returned error: %v", err)
	}
	if config.CurrentRevisionID != first.CurrentRevisionID || config.Prompt != first.Prompt {
		t.Fatalf("config after rollback = %#v, want first revision active", config)
	}
}

func TestRollbackPromptReturnsNotFoundForInvalidTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(db))
	router := gin.New()
	router.POST("/admin/feature-prompts/:slug/rollback", handler.RollbackPrompt)

	req := httptest.NewRequest(http.MethodPost, "/admin/feature-prompts/image-extend/rollback", strings.NewReader(`{"revision_id":999999}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}
