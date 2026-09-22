package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestREDMetricsMiddlewareCountsByRouteAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := prometheus.NewRegistry()
	set := redmetrics.NewSet(registry)
	router := gin.New()
	router.Use(REDMetricsMiddleware(set))
	router.GET("/api/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	router.GET("/api/boom", func(c *gin.Context) { c.String(http.StatusInternalServerError, "boom") })

	serve := func(path string) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	}
	serve("/api/health")
	serve("/api/health")
	serve("/api/boom")
	serve("/no-such-route")

	expected := `
# HELP ideaboard_http_requests_total HTTP requests by method, route template and status code.
# TYPE ideaboard_http_requests_total counter
ideaboard_http_requests_total{method="GET",route="/api/boom",status="500"} 1
ideaboard_http_requests_total{method="GET",route="/api/health",status="200"} 2
ideaboard_http_requests_total{method="GET",route="unmatched",status="404"} 1
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected), "ideaboard_http_requests_total"); err != nil {
		t.Fatalf("RED http counters mismatch: %v", err)
	}
	got, err := testutil.GatherAndCount(registry, "ideaboard_http_request_duration_seconds")
	if err != nil {
		t.Fatalf("gather histogram: %v", err)
	}
	if got != 3 {
		t.Fatalf("latency histogram series = %d, want 3", got)
	}
}

func TestREDMetricsMiddlewareNilSetIsPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(REDMetricsMiddleware(nil))
	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
