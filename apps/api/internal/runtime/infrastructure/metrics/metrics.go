// Package metrics is the RED minimal metric set for the API process
// (issue #765): per-route HTTP latency/error counts, River queue depth and
// head lag, and AI gateway request/retry counts. All collectors are exposed
// on the dedicated /metrics endpoint through the default Prometheus
// registry; tests build an isolated Set with NewSet(prometheus.NewRegistry()).
//
// Label values stay closed: route is the Gin route template (or "unmatched"),
// status is the numeric HTTP status, feature is the workflow feature slug
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

// UnknownFeature labels gateway observations whose feature slug is missing.
const UnknownFeature = "unknown"

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
	gatewayRequests   *prometheus.CounterVec
	gatewayRetries    *prometheus.CounterVec
	riverQueueDepth   *prometheus.GaugeVec
	riverQueueLag     *prometheus.GaugeVec
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
		gatewayRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ideaboard_gateway_requests_total",
			Help: "Cloudflare AI gateway calls by feature and final outcome (success/error).",
		}, []string{"feature", "outcome"}),
		gatewayRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ideaboard_gateway_retries_total",
			Help: "Worker-level task requeues after retryable gateway failures, by feature.",
		}, []string{"feature"}),
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
		s.gatewayRequests = registerCounterVec(reg, s.gatewayRequests)
		s.gatewayRetries = registerCounterVec(reg, s.gatewayRetries)
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

// normalizeFeature keeps the feature label closed to known slugs.
func normalizeFeature(feature string) string {
	if trimmed := strings.TrimSpace(feature); trimmed != "" {
		return trimmed
	}
	return UnknownFeature
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

// ObserveGatewayRequest records one finished gateway call. err == nil means
// success; any error means outcome "error".
func (s *Set) ObserveGatewayRequest(feature string, err error) {
	if s == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.gatewayRequests.WithLabelValues(normalizeFeature(feature), outcome).Inc()
}

// IncGatewayRetry records one worker-level requeue after a retryable
// gateway failure (gateway-internal retries exhausted, whole task requeued
// under River backoff).
func (s *Set) IncGatewayRetry(feature string) {
	if s == nil {
		return
	}
	s.gatewayRetries.WithLabelValues(normalizeFeature(feature)).Inc()
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

// Refresher periodically publishes River queue depth/lag gauges. It is a
// runtime Runner owned by the composition root: scrape stays cheap (gauges
// only) while freshness comes from the tick, not from blocking /metrics on
// the database.
type Refresher struct {
	set      *Set
	queue    string
	stats    RiverQueueStats
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewRefresher builds the River queue metrics poller. Interval <= 0 keeps
// DefaultRefreshInterval. Nil set or stats disables refresh (Start no-ops).
func NewRefresher(set *Set, queue string, stats RiverQueueStats, interval time.Duration) *Refresher {
	if interval <= 0 {
		interval = DefaultRefreshInterval
	}
	return &Refresher{set: set, queue: queue, stats: stats, interval: interval}
}

// Name implements ports.Runner.
func (r *Refresher) Name() string { return "river-queue-metrics" }

// Kind implements ports.Runner.
func (r *Refresher) Kind() domain.ComponentKind { return domain.ComponentKindRiver }

// Start implements ports.Runner: it refreshes once, then ticks until Stop.
func (r *Refresher) Start(ctx context.Context) error {
	if r == nil || r.set == nil || r.stats == nil {
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
		r.set.RefreshRiverQueue(refreshCtx, r.queue, r.stats)
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ticker.C:
				r.set.RefreshRiverQueue(refreshCtx, r.queue, r.stats)
			}
		}
	}()
	_ = ctx
	return nil
}

// Stop implements ports.Runner.
func (r *Refresher) Stop(ctx context.Context) error {
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

var (
	_ ports.Runner = (*Refresher)(nil)
	_ ports.Runner = (*MultiRefresher)(nil)
)

// MultiRefresher periodically publishes River depth/lag gauges for every
// known queue (see taskqueue.RiverRetryMatrix). It shares the Refresher
// semantics: one blocking-safe snapshot per queue per tick, failures keep
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
