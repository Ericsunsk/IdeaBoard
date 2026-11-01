// Package metrics is the RED minimal metric set for the API process
// (issue #765): per-route HTTP latency/error counts, River queue depth and
// head lag, and Worker egress request/retry counts. All collectors are exposed
// on the dedicated /metrics endpoint through the default Prometheus
// registry; tests build an isolated Set with NewSet(prometheus.NewRegistry()).
//
// Label values stay closed: route is the Gin route template (or "unmatched"),
// status is the numeric HTTP status, service is the service slug
// (or "probe"/"vision-search"/"unknown"), queue is the River queue name.
// No user IDs, task IDs, URLs, or error text ever enter a label.
package metrics

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/runtime/domain"
	"ideaboard/api/internal/runtime/ports"

	"github.com/prometheus/client_golang/prometheus"
)

// UnmatchedRoute labels requests that hit no registered Gin route (404s and
// method mismatches). It keeps cardinality bounded instead of recording raw
// paths.
const UnmatchedRoute = "unmatched"

// UnknownService labels Worker egress observations whose service slug is missing.
const UnknownService = "unknown"

// DefaultRefreshInterval paces the River queue refresher: fresh enough for
// lag alerting, cheap enough for one short SQL statement per tick.
const DefaultRefreshInterval = 15 * time.Second

// RefreshTimeout bounds one River queue stats query so a stuck database
// never blocks the refresher tick or a scrape.
const RefreshTimeout = 5 * time.Second

// Set carries the RED collectors. Use DefaultSet in production (registered
// on the default registry scraped by /metrics); tests build an isolated
// instance with NewSet(prometheus.NewRegistry()).
type Set struct {
	httpRequestsTotal *prometheus.CounterVec
	httpRequestTime   *prometheus.HistogramVec
	workerRequests    *prometheus.CounterVec
	workerRetries     *prometheus.CounterVec
	// workerPerModelLimited counts sends rejected before egress by the
	// per-model guards (reason=concurrency_limited|breaker_open). Rejections
	// surface as retryable 429s so the worker requeues with backoff.
	workerPerModelLimited *prometheus.CounterVec
	// workerBreakerOpen reports the per-model breaker state (1=open,
	// 0=closed/half-open probe admitted). Model labels stay bounded: models
	// come from the admin catalog/mapping, never from user input.
	workerBreakerOpen *prometheus.GaugeVec
	// modelFallbacks counts bounded model-fallback sends (#852,
	// ADR-0012 rule 6): exactly one retry-candidate send per task after
	// a retryable upstream failure. Labels mirror workerRequests
	// ({service, outcome}) so fallback success/error splits stay
	// comparable with worker outcomes; retry_layer stays out of the
	// labels (one counter per layer would triple series for no new
	// signal: workerRetries and this counter already separate them).
	modelFallbacks  *prometheus.CounterVec
	riverQueueDepth *prometheus.GaugeVec
	riverQueueLag   *prometheus.GaugeVec
}

// NewSet registers the RED collectors on reg. A nil reg builds an
// unregistered Set (observations still work, nothing is scraped).
func NewSet(reg prometheus.Registerer) *Set {
	s := &Set{
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ideaboard_http_requests_total",
			Help: "HTTP requests by method, route template and status code.",
		}, []string{"method", "route", "status"}),
		httpRequestTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ideaboard_http_request_duration_seconds",
			Help:    "HTTP request latency by method, route template and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		workerRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ideaboard_gateway_requests_total",
			Help: "Worker egress calls by service and final outcome (success/error).",
		}, []string{"service", "outcome"}),
		workerRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ideaboard_gateway_retries_total",
			Help: "Worker-level task requeues after retryable worker failures, by service.",
		}, []string{"service"}),
		workerPerModelLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ideaboard_gateway_per_model_limited_total",
			Help: "Worker sends rejected before egress by per-model guards, by model and reason (concurrency_limited|breaker_open).",
		}, []string{"model", "reason"}),
		workerBreakerOpen: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ideaboard_gateway_breaker_open",
			Help: "Per-model worker breaker state by model (1=open, 0=closed).",
		}, []string{"model"}),
		modelFallbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ideaboard_model_fallback_total",
			Help: "Bounded model-fallback sends to the snapshot retry candidate, by service and fallback-send outcome (success/error).",
		}, []string{"service", "outcome"}),
		riverQueueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ideaboard_river_queue_depth",
			Help: "River jobs waiting for execution (available/scheduled/retryable), by queue.",
		}, []string{"queue"}),
		riverQueueLag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ideaboard_river_queue_lag_seconds",
			Help: "Age of the oldest waiting River job, by queue.",
		}, []string{"queue"}),
	}
	if reg != nil {
		s.httpRequestsTotal = registerCounterVec(reg, s.httpRequestsTotal)
		s.httpRequestTime = registerHistogramVec(reg, s.httpRequestTime)
		s.workerRequests = registerCounterVec(reg, s.workerRequests)
		s.workerRetries = registerCounterVec(reg, s.workerRetries)
		s.workerPerModelLimited = registerCounterVec(reg, s.workerPerModelLimited)
		s.workerBreakerOpen = registerGaugeVec(reg, s.workerBreakerOpen)
		s.modelFallbacks = registerCounterVec(reg, s.modelFallbacks)
		s.riverQueueDepth = registerGaugeVec(reg, s.riverQueueDepth)
		s.riverQueueLag = registerGaugeVec(reg, s.riverQueueLag)
	}
	return s
}

// registerCounterVec registers collector, reusing the already-registered
// instance when the process builds the Set twice (e.g. repeated container
// construction in tests) so observations never sink into a dropped copy.
func registerCounterVec(reg prometheus.Registerer, collector *prometheus.CounterVec) *prometheus.CounterVec {
	if existing := registerOrReuse(reg, collector); existing != nil {
		if vec, ok := existing.(*prometheus.CounterVec); ok {
			return vec
		}
	}
	return collector
}

func registerHistogramVec(reg prometheus.Registerer, collector *prometheus.HistogramVec) *prometheus.HistogramVec {
	if existing := registerOrReuse(reg, collector); existing != nil {
		if vec, ok := existing.(*prometheus.HistogramVec); ok {
			return vec
		}
	}
	return collector
}

func registerGaugeVec(reg prometheus.Registerer, collector *prometheus.GaugeVec) *prometheus.GaugeVec {
	if existing := registerOrReuse(reg, collector); existing != nil {
		if vec, ok := existing.(*prometheus.GaugeVec); ok {
			return vec
		}
	}
	return collector
}

func registerOrReuse(reg prometheus.Registerer, collector prometheus.Collector) prometheus.Collector {
	if err := reg.Register(collector); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			return already.ExistingCollector
		}
		slog.Warn("RED metrics registration failed", "err", err)
	}
	return nil
}

var defaultSetOnce sync.Once
var defaultSetValue *Set

// DefaultSet feeds the process-wide /metrics endpoint. The composition root
// owns when it is created; call sites only observe through it.
func DefaultSet() *Set {
	defaultSetOnce.Do(func() {
		defaultSetValue = NewSet(prometheus.DefaultRegisterer)
	})
	return defaultSetValue
}

// normalizeRoute keeps the route label closed to registered templates.
func normalizeRoute(route string) string {
	if strings.TrimSpace(route) == "" {
		return UnmatchedRoute
	}
	return route
}

// normalizeService keeps the service label closed to known slugs.
func normalizeService(service string) string {
	if trimmed := strings.TrimSpace(service); trimmed != "" {
		return trimmed
	}
	return UnknownService
}

// ObserveHTTP records one finished HTTP request. A nil Set is a no-op so
// uninitialized call paths never crash.
func (s *Set) ObserveHTTP(method, route string, status int, latency time.Duration) {
	if s == nil {
		return
	}
	if status < 100 {
		status = 200
	}
	labels := []string{strings.ToUpper(strings.TrimSpace(method)), normalizeRoute(route), strconv.Itoa(status)}
	s.httpRequestsTotal.WithLabelValues(labels...).Inc()
	s.httpRequestTime.WithLabelValues(labels...).Observe(latency.Seconds())
}

// ObserveWorkerRequest records one finished Worker egress call. err == nil means
// success; non-nil errors (including guard rejections) count as failures in
// RED. The Prometheus metric name stays gateway-dimensioned for dashboard
// continuity.
func (s *Set) ObserveWorkerRequest(service string, err error) {
	if s == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.workerRequests.WithLabelValues(normalizeService(service), outcome).Inc()
}

// IncWorkerRetry records one worker-level requeue after a retryable
// worker failure under River backoff.
func (s *Set) IncWorkerRetry(service string) {
	if s == nil {
		return
	}
	s.workerRetries.WithLabelValues(normalizeService(service)).Inc()
}

// Worker per-model guard rejection reasons (closed label set).
const (
	WorkerLimitedConcurrency = "concurrency_limited"
	WorkerLimitedBreakerOpen = "breaker_open"
)

// normalizeWorkerModel keeps the breaker/limiter model label closed to
// admin-controlled values: empty reads as unknown, case folds.
func normalizeWorkerModel(model string) string {
	if trimmed := strings.ToLower(strings.TrimSpace(model)); trimmed != "" {
		return trimmed
	}
	return UnknownService
}

// IncWorkerPerModelLimited records one send rejected before egress by the
// per-model guards. A nil Set is a no-op. Reason must be one of the
// WorkerLimited* constants; anything else reads as concurrency_limited so
// the label set stays closed.
func (s *Set) IncWorkerPerModelLimited(model, reason string) {
	if s == nil {
		return
	}
	if reason != WorkerLimitedConcurrency && reason != WorkerLimitedBreakerOpen {
		reason = WorkerLimitedConcurrency
	}
	s.workerPerModelLimited.WithLabelValues(normalizeWorkerModel(model), reason).Inc()
}

// SetWorkerBreakerOpen publishes the per-model breaker state (1=open,
// 0=closed). A nil Set is a no-op.
func (s *Set) SetWorkerBreakerOpen(model string, open bool) {
	if s == nil {
		return
	}
	value := 0.0
	if open {
		value = 1.0
	}
	s.workerBreakerOpen.WithLabelValues(normalizeWorkerModel(model)).Set(value)
}

// IncModelFallback records one bounded model-fallback send to the
// snapshot's retry candidate (#852, ADR-0012 rule 6). err == nil means
// the fallback send succeeded; any error means outcome "error". The
// observation fires once per fallback send (at most once per task), at
// send completion when the outcome is known — never at fallback
// decision time, so decided-but-unrecorded fallbacks (missing
// model_fallback row) never inflate the counter.
func (s *Set) IncModelFallback(service string, err error) {
	if s == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.modelFallbacks.WithLabelValues(normalizeService(service), outcome).Inc()
}

// SetRiverQueue publishes the latest River queue snapshot for one queue.
func (s *Set) SetRiverQueue(queue string, depth int64, lagSeconds float64) {
	if s == nil {
		return
	}
	name := strings.TrimSpace(queue)
	if name == "" {
		name = "unknown"
	}
	if depth < 0 {
		depth = 0
	}
	if lagSeconds < 0 {
		lagSeconds = 0
	}
	s.riverQueueDepth.WithLabelValues(name).Set(float64(depth))
	s.riverQueueLag.WithLabelValues(name).Set(lagSeconds)
}

// RiverQueueStats reads queue depth (waiting jobs) and head lag (oldest
// waiting job age in seconds). ctx must come first per repository rules; the
// refresher always bounds it with RefreshTimeout.
type RiverQueueStats func(ctx context.Context) (depth int64, lagSeconds float64, err error)

// RefreshRiverQueue pulls one snapshot through stats and publishes it. Query
// failures keep the last gauges and only log, so a stuck database degrades
// freshness instead of breaking the /metrics scrape.
func (s *Set) RefreshRiverQueue(ctx context.Context, queue string, stats RiverQueueStats) {
	if s == nil || stats == nil {
		return
	}
	refreshCtx, cancel := context.WithTimeout(ctx, RefreshTimeout)
	defer cancel()
	depth, lag, err := stats(refreshCtx)
	if err != nil {
		slog.Warn("river queue metrics refresh failed", "queue", queue, "err", err)
		return
	}
	s.SetRiverQueue(queue, depth, lag)
}

var _ ports.Runner = (*MultiRefresher)(nil)

// MultiRefresher periodically publishes River depth/lag gauges for every
// known queue (see riverpolicy.RiverRetryMatrix). It takes one blocking-safe
// snapshot per queue per tick; failures keep
// the last gauges and only log, and nil set/empty stats disables refresh.
type MultiRefresher struct {
	set      *Set
	queues   map[string]RiverQueueStats
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewMultiRefresher builds the all-queues River metrics poller.
// Interval <= 0 keeps DefaultRefreshInterval. Entries with a nil stats func
// are dropped; a nil set or an empty map disables refresh (Start no-ops).
func NewMultiRefresher(set *Set, queues map[string]RiverQueueStats, interval time.Duration) *MultiRefresher {
	if interval <= 0 {
		interval = DefaultRefreshInterval
	}
	kept := make(map[string]RiverQueueStats, len(queues))
	for name, stats := range queues {
		if strings.TrimSpace(name) == "" || stats == nil {
			continue
		}
		kept[name] = stats
	}
	return &MultiRefresher{set: set, queues: kept, interval: interval}
}

// Queues returns the queue names this refresher polls, for wiring tests.
func (r *MultiRefresher) Queues() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.queues))
	for name := range r.queues {
		names = append(names, name)
	}
	return names
}

// Name implements ports.Runner.
func (r *MultiRefresher) Name() string { return "river-queue-metrics" }

// Kind implements ports.Runner.
func (r *MultiRefresher) Kind() domain.ComponentKind { return domain.ComponentKindRiver }

// Start implements ports.Runner: it refreshes every queue once, then ticks
// until Stop.
func (r *MultiRefresher) Start(ctx context.Context) error {
	if r == nil || r.set == nil || len(r.queues) == 0 {
		return nil
	}
	r.mu.Lock()
	if r.done != nil {
		r.mu.Unlock()
		return nil
	}
	refreshCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	done := r.done
	r.mu.Unlock()
	go func() {
		defer close(done)
		r.refreshAll(refreshCtx)
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ticker.C:
				r.refreshAll(refreshCtx)
			}
		}
	}()
	_ = ctx
	return nil
}

// Stop implements ports.Runner.
func (r *MultiRefresher) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return nil
}

func (r *MultiRefresher) refreshAll(ctx context.Context) {
	if r == nil || r.set == nil {
		return
	}
	for queue, stats := range r.queues {
		r.set.RefreshRiverQueue(ctx, queue, stats)
	}
}
