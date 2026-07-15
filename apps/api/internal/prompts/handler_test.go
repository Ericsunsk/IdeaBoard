package prompts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func newAdminPromptTestRouter(t testing.TB, handler *FeaturePromptHandler) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
		c.Next()
	})
	router.PUT("/admin/feature-prompts/:slug", handler.SetPrompt)
	return router
}

func TestSetPromptRequiresAdminPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name       string
		setup      func(*gin.Context)
		wantStatus int
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "legacy key", setup: func(c *gin.Context) { c.Set("user_id", int64(7)) }, wantStatus: http.StatusUnauthorized},
		{name: "user role", setup: func(c *gin.Context) { testsupport.SetPrincipal(t, c, 7, principal.RoleUser) }, wantStatus: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.PUT("/admin/feature-prompts/:slug", func(c *gin.Context) {
				if tt.setup != nil {
					tt.setup(c)
				}
				NewFeaturePromptHandler(nil).SetPrompt(c)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/partial-edit", strings.NewReader(`{"prompt":"test"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSetPromptRequiresPromptField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(db))
	router := newAdminPromptTestRouter(t, handler)

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
	router := newAdminPromptTestRouter(t, handler)

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
	router := newAdminPromptTestRouter(t, handler)

	req := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/partial-edit", strings.NewReader(`{"prompt":"hello","model_defaults":{"model":" image-alpha "}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestSetPromptAllowsPatternGenWithoutPublishGate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(db))
	router := newAdminPromptTestRouter(t, handler)

	req := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/pattern-gen", strings.NewReader(`{"prompt":"Create {{missing_token}}","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
