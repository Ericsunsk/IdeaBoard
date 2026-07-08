package prompts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"

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
