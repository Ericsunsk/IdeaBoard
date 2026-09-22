package outboundhttp

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric label values stay closed: target is a caller-chosen egress name
// (never a URL or user id), outcome and reason come from fixed sets.
const (
	outcomeSuccess      = "success"
	outcomeTimeout      = "timeout"
	outcomeTransportErr = "transport_error"
	outcomeClientError  = "http_4xx"
	outcomeServerError  = "http_5xx"
	outcomeCanceled     = "canceled"
)

// Outcome names exported for non-HTTP recorders (Stripe SDK, minio data
// plane) so every egress shares one closed outcome set.
const (
	OutcomeSuccess      = outcomeSuccess
	OutcomeTimeout      = outcomeTimeout
	OutcomeTransportErr = outcomeTransportErr
	OutcomeClientError  = outcomeClientError
	OutcomeServerError  = outcomeServerError
	OutcomeCanceled     = outcomeCanceled
)

// Metrics carries the outbound counters. Use DefaultMetrics in production
// (registered on the default registry scraped by /metrics); tests build an
// isolated instance with NewMetrics(prometheus.NewRegistry()).
type Metrics struct {
	requests       *prometheus.CounterVec
	retries        *prometheus.CounterVec
	retryAfterHits *prometheus.CounterVec
	durations      *prometheus.HistogramVec
}

// NewMetrics registers outbound counters on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbound_requests_total",
			Help: "Standard outbound client calls by target and final outcome.",
		}, []string{"target", "outcome"}),
		retries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbound_retries_total",
			Help: "Standard outbound client scheduled retries by target and reason.",
		}, []string{"target", "reason"}),
		retryAfterHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbound_retry_after_honored_total",
			Help: "Upstream Retry-After delays honored instead of backoff, by target.",
		}, []string{"target"}),
		durations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "outbound_request_duration_seconds",
			Help: "Outbound call latency by target and final outcome.",
		}, []string{"target", "outcome"}),
	}
	if reg != nil {
		reg.MustRegister(m.requests, m.retries, m.retryAfterHits, m.durations)
	}
	return m
}

// DefaultMetrics feeds the process-wide /metrics endpoint.
var DefaultMetrics = NewMetrics(prometheus.DefaultRegisterer)

func (m *Metrics) observeRequest(target, outcome string) {
	if m == nil {
		return
	}
	m.requests.WithLabelValues(target, outcome).Inc()
}

// Observe records one finished non-HTTP egress call (Stripe SDK, minio data
// plane) in the shared outbound RED set. Outcome must come from the closed
// Outcome* set; target is a fixed egress name (never a URL or user id).
// A nil receiver is a no-op so uninitialized call paths never crash.
func (m *Metrics) Observe(target, outcome string, latency time.Duration) {
	if m == nil {
		return
	}
	if outcome == "" {
		outcome = outcomeTransportErr
	}
	m.requests.WithLabelValues(target, outcome).Inc()
	m.durations.WithLabelValues(target, outcome).Observe(latency.Seconds())
}

// ObserveDuration records latency for an HTTP-client outcome already counted
// by observeRequest. Kept separate so the hot Do path counts once.
func (m *Metrics) observeDuration(target, outcome string, latency time.Duration) {
	if m == nil {
		return
	}
	m.durations.WithLabelValues(target, outcome).Observe(latency.Seconds())
}

// OutcomeForStatus maps an HTTP status to the closed outcome set.
func OutcomeForStatus(status int) string {
	return outcomeForStatus(status)
}

func (m *Metrics) observeRetry(target, reason string) {
	if m == nil {
		return
	}
	m.retries.WithLabelValues(target, reason).Inc()
}

func (m *Metrics) observeRetryAfter(target string) {
	if m == nil {
		return
	}
	m.retryAfterHits.WithLabelValues(target).Inc()
}
