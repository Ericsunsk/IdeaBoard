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

func newAdminFeaturePromptTestRouter(t testing.TB, handler *FeaturePromptHandler) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
		c.Next()
	})
	router.PUT("/admin/feature-prompts/:slug", handler.Save)
	return router
}

func TestSaveFeaturePromptAcceptsEmptyPrompt(t *testing.T) {
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(testdb.Open(t)))
	router := newAdminFeaturePromptTestRouter(t, handler)
	request := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/style-variation", strings.NewReader(`{"prompt":""}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSaveFeaturePromptRejectsRuntimePolicyFields(t *testing.T) {
	handler := NewFeaturePromptHandler(NewFeaturePromptRepository(testdb.Open(t)))
	router := newAdminFeaturePromptTestRouter(t, handler)
	request := httptest.NewRequest(http.MethodPut, "/admin/feature-prompts/style-variation", strings.NewReader(`{"prompt":"x","canonical_model":"legacy"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}
