package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func TestSearchHandlersFailClosedWithoutDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("search service", func(t *testing.T) {
		router := gin.New()
		router.GET("/search", NewSearchHandler(nil, nil).Search)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/search?q=test", nil))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
		}
	})

	for _, method := range []string{"update", "test"} {
		t.Run("provider "+method, func(t *testing.T) {
			router := gin.New()
			router.POST("/providers/:provider/"+method, func(c *gin.Context) {
				testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
				if method == "update" {
					NewHandler(nil).Update(c)
					return
				}
				NewHandler(nil).Test(c)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/providers/unsplash/"+method, strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
		})
	}
}
