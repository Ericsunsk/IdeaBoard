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
