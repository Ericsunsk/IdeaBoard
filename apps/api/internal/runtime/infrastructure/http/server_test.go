package httpinfra

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsServerExportsOnlyTheDedicatedMetricsPath(t *testing.T) {
	registry := prometheus.NewRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "ideaboard_test_metric_total"})
	registry.MustRegister(counter)
	counter.Inc()

	server := newMetricsServerWithGatherer(&config.Config{
		App: config.AppConfig{MetricsAddr: "127.0.0.1:19091"},
	}, registry)
	if server.Addr != "127.0.0.1:19091" {
		t.Fatalf("metrics server address = %q", server.Addr)
	}

	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ideaboard_test_metric_total 1") {
		t.Fatalf("metrics response status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	notFound := httptest.NewRecorder()
	server.Handler.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("non-metrics path status = %d, want 404", notFound.Code)
	}
}
