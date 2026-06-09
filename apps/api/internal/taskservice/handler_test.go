package taskservice

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminListTasksRejectsInvalidIDFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		query   string
		message string
	}{
		{name: "user id zero", query: "user_id=0", message: "user_id 必须大于 0"},
		{name: "account id negative", query: "account_id=-1", message: "account_id 必须大于 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/admin/tasks", NewHandler(nil).ListTasks)

			req := httptest.NewRequest(http.MethodGet, "/admin/tasks?"+tt.query, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.message) {
				t.Fatalf("body = %s, want %q", rec.Body.String(), tt.message)
			}
		})
	}
}
