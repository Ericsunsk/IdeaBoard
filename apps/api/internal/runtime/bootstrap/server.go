package bootstrap

import (
	"net"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/config"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newHTTPServer(cfg *config.Config, handler *gin.Engine) *http.Server {
	return &http.Server{
		Addr:              listenerAddr(cfg.App.Host, cfg.App.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func newMetricsServer(cfg *config.Config) *http.Server {
	return newMetricsServerWithGatherer(cfg, prometheus.DefaultGatherer)
}

func newMetricsServerWithGatherer(cfg *config.Config, gatherer prometheus.Gatherer) *http.Server {
	addr := "127.0.0.1:9091"
	if cfg != nil && strings.TrimSpace(cfg.App.MetricsAddr) != "" {
		addr = strings.TrimSpace(cfg.App.MetricsAddr)
	}
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func listenerAddr(host, port string) string {
	if strings.TrimSpace(host) == "" {
		return "127.0.0.1:" + port
	}
	return net.JoinHostPort(host, port)
}
