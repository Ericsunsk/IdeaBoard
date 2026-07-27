package prompts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type eligibleModelLookupStub struct {
	eligible bool
}

func (s eligibleModelLookupStub) HasEligibleModel(context.Context, string, string) (bool, error) {
	return s.eligible, nil
}

func newAdminWorkflowFeatureTestRouter(t testing.TB, handler *WorkflowFeatureHandler) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
		c.Next()
	})
	router.PUT("/admin/workflow-features/:slug", handler.Save)
	return router
}

func TestSaveWorkflowFeatureRequiresCanonicalModel(t *testing.T) {
	handler := NewWorkflowFeatureHandler(NewWorkflowFeatureRepository(testdb.Open(t)), eligibleModelLookupStub{eligible: true})
	router := newAdminWorkflowFeatureTestRouter(t, handler)
	request := httptest.NewRequest(http.MethodPut, "/admin/workflow-features/style-variation", strings.NewReader(`{"canonical_model":" ","system_prompt":"test","enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSaveWorkflowFeatureRequiresEligibleAccountModel(t *testing.T) {
	handler := NewWorkflowFeatureHandler(NewWorkflowFeatureRepository(testdb.Open(t)), eligibleModelLookupStub{})
	router := newAdminWorkflowFeatureTestRouter(t, handler)
	request := httptest.NewRequest(http.MethodPut, "/admin/workflow-features/style-variation", strings.NewReader(`{"canonical_model":"image-a","system_prompt":"test","enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSaveDisabledWorkflowFeatureWithoutEligibleAccountModel(t *testing.T) {
	handler := NewWorkflowFeatureHandler(NewWorkflowFeatureRepository(testdb.Open(t)), eligibleModelLookupStub{})
	router := newAdminWorkflowFeatureTestRouter(t, handler)
	request := httptest.NewRequest(http.MethodPut, "/admin/workflow-features/style-variation", strings.NewReader(`{"canonical_model":"image-a","system_prompt":"test","enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSaveWorkflowFeaturePersistsCurrentConfiguration(t *testing.T) {
	db := testdb.Open(t)
	handler := NewWorkflowFeatureHandler(NewWorkflowFeatureRepository(db), eligibleModelLookupStub{eligible: true})
	router := newAdminWorkflowFeatureTestRouter(t, handler)
	request := httptest.NewRequest(http.MethodPut, "/admin/workflow-features/style-variation", strings.NewReader(`{"canonical_model":"image-a","system_prompt":"test","enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}
