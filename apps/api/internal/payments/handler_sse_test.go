package payments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func TestUserPaymentEventStreamIsUserScopedNoStoreAndCleansUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := NewEventHub()
	handler := NewHandler(NewService(ServiceDependencies{Events: hub}), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.GET("/user/payments/events", handler.StreamUserPaymentEvents)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/user/payments/events", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream response=%d headers=%v", recorder.Code, recorder.Header())
	}
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	if len(hub.subscribers) != 0 {
		t.Fatalf("subscription was not cleaned up: %+v", hub.subscribers)
	}
}
