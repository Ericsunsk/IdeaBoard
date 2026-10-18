package sse

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newSSETestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/events", nil)
	return c, recorder
}

func TestPrepareSetsEventStreamHeaders(t *testing.T) {
	c, recorder := newSSETestContext()
	// SetWriteDeadline needs a real connection; httptest returns
	// "feature not supported". Headers are set before that call.
	_ = Prepare(c)
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
}

func TestWriteEventAndIDEventFrameShape(t *testing.T) {
	c, recorder := newSSETestContext()
	if err := WriteEvent(c, "ping", map[string]string{"status": "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteIDEvent(c, "evt_1", "update", map[string]string{"status": "ok"}); err != nil {
		t.Fatal(err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: ping\ndata: ") {
		t.Fatalf("missing ping frame in %q", body)
	}
	if !strings.Contains(body, "id: evt_1\nevent: update\ndata: ") {
		t.Fatalf("missing id frame in %q", body)
	}
}
