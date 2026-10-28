package http

import (
	"ideaboard/api/internal/platform/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsServerExportsOnlyTheDedicatedMetricsPath(t *testing.T) {
	registry := prometheus.NewRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "ideaboard_test_metric_total"})
	registry.MustRegister(counter)
	counter.Inc()

	server := newMetricsServerWithGatherer(nil, registry)
	if server.Addr != "127.0.0.1:9091" {
		t.Fatalf("metrics server address = %q, want %q", server.Addr, "127.0.0.1:9091")
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

func TestListenersFollowTopology(t *testing.T) {
	for _, tt := range []struct{ env, host string }{
		{"development", "127.0.0.1"}, {"production", "0.0.0.0"},
	} {
		t.Run(tt.env, func(t *testing.T) {
			cfg := &config.Config{App: config.AppConfig{Env: tt.env}}
			if got := NewHTTPServerRunner(cfg, gin.New()).server.Addr; got != tt.host+":6000" {
				t.Fatalf("API listener = %q", got)
			}
			if got := newMetricsServerWithGatherer(cfg, prometheus.NewRegistry()).Addr; got != tt.host+":9091" {
				t.Fatalf("metrics listener = %q", got)
			}
		})
	}
}

func TestClientIPRejectsLANForwardedHeadersDuringHostDevelopment(t *testing.T) {
	for _, tt := range []struct{ env, peer, want string }{
		{"development", "192.168.1.10:1234", "192.168.1.10"},
		{"development", "127.0.0.1:1234", "203.0.113.10"},
		{"production", "172.18.0.1:1234", "203.0.113.10"},
	} {
		t.Run(tt.env+tt.peer, func(t *testing.T) {
			router := gin.New()
			if err := router.SetTrustedProxies((config.AppConfig{Env: tt.env}).TrustedProxies()); err != nil {
				t.Fatal(err)
			}
			router.GET("/client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
			req := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
			req.RemoteAddr = tt.peer
			req.Header.Set("X-Forwarded-For", "203.0.113.10")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if got := response.Body.String(); got != tt.want {
				t.Fatalf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}
