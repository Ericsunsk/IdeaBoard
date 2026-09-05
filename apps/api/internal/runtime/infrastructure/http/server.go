package httpinfra

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServerRunner wraps net/http.Server as a managed runtime ports.Runner.
type ServerRunner struct {
	name     string
	kind     domain.ComponentKind
	server   *http.Server
	mu       sync.RWMutex
	listener net.Listener
	started  bool
}

func (s *ServerRunner) Name() string               { return s.name }
func (s *ServerRunner) Kind() domain.ComponentKind { return s.kind }

func (s *ServerRunner) Start(context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.listener = listener
	s.started = true
	s.mu.Unlock()
	go func() {
		slog.Info(s.name+" server starting", "addr", s.server.Addr)
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("failed to start "+s.name+" server", "err", err)
		}
	}()
	return nil
}

func (s *ServerRunner) Stop(ctx context.Context) error {
	err := s.server.Shutdown(ctx)
	s.mu.Lock()
	s.started = false
	s.listener = nil
	s.mu.Unlock()
	return err
}

func (s *ServerRunner) CheckHealth(context.Context) domain.ComponentStatus {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()
	return domain.ComponentStatus{Name: s.name, Kind: s.kind, Ready: started, Required: true}
}

var _ ports.Runner = (*ServerRunner)(nil)
var _ ports.HealthChecker = (*ServerRunner)(nil)

// NewHTTPServerRunner constructs a ports.Runner for main HTTP API traffic.
func NewHTTPServerRunner(cfg *config.Config, handler *gin.Engine) *ServerRunner {
	srv := &http.Server{
		Addr:              ListenerAddr(cfg.App.Host, cfg.App.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return &ServerRunner{
		name:   "HTTP",
		kind:   domain.ComponentKindHTTP,
		server: srv,
	}
}

// NewMetricsServerRunner constructs a ports.Runner for Prometheus metrics scrapers.
func NewMetricsServerRunner(cfg *config.Config) *ServerRunner {
	return NewMetricsServerRunnerWithGatherer(cfg, prometheus.DefaultGatherer)
}

func newMetricsServerWithGatherer(cfg *config.Config, gatherer prometheus.Gatherer) *http.Server {
	return NewMetricsServerRunnerWithGatherer(cfg, gatherer).server
}

// NewMetricsServerRunnerWithGatherer constructs a metrics runner with a custom gatherer.
func NewMetricsServerRunnerWithGatherer(cfg *config.Config, gatherer prometheus.Gatherer) *ServerRunner {
	addr := "127.0.0.1:9091"
	if cfg != nil && strings.TrimSpace(cfg.App.MetricsAddr) != "" {
		addr = strings.TrimSpace(cfg.App.MetricsAddr)
	}
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return &ServerRunner{
		name:   "metrics",
		kind:   domain.ComponentKindHTTP,
		server: srv,
	}
}

// ListenerAddr formats host and port for network binding.
func ListenerAddr(host, port string) string {
	if strings.TrimSpace(host) == "" {
		return "127.0.0.1:" + port
	}
	return net.JoinHostPort(host, port)
}
