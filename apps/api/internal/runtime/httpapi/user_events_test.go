package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type testUserEvents struct{ unsubscribed atomic.Int32 }

func (s *testUserEvents) Subscribe(int64) (<-chan events.UserEvent, func(), bool) {
	return make(chan events.UserEvent), func() { s.unsubscribed.Add(1) }, true
}

func TestUserEventStreamIsScopedAndCleansUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userEvents := &testUserEvents{}
	handler := NewUserEventStreamHandler(userEvents)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		c.Next()
	})
	router.GET("/user/events", handler.StreamUserEvents)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/user/events", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream response=%d headers=%v", recorder.Code, recorder.Header())
	}
	if got := recorder.Body.String(); got != ":connected\n\n" {
		t.Fatalf("initial SSE frame=%q, want connected frame", got)
	}
	if userEvents.unsubscribed.Load() != 1 {
		t.Fatalf("subscriptions were not cleaned up")
	}
}
