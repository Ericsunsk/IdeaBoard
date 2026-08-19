package searchcatalog

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type SearchObservation struct {
	Duration       time.Duration
	Page           string
	Outcome        string
	ResultCount    int
	ProviderStatus []ProviderStatus
}

type SearchMetrics interface {
	RecordSearch(SearchObservation)
	RecordImageFailure(Provider)
}

type noopSearchMetrics struct{}

func (noopSearchMetrics) RecordSearch(SearchObservation) {}
func (noopSearchMetrics) RecordImageFailure(Provider)    {}

type prometheusSearchMetrics struct {
	requests        *prometheus.CounterVec
	duration        prometheus.Histogram
	providerRequest *prometheus.CounterVec
	providerLatency *prometheus.HistogramVec
	resultCount     *prometheus.HistogramVec
	imageFailures   *prometheus.CounterVec
}

func NewPrometheusMetrics(registerer prometheus.Registerer) (SearchMetrics, error) {
	if registerer == nil {
		return nil, fmt.Errorf("register search metrics: registerer is required")
	}
	metrics := &prometheusSearchMetrics{}
	var err error
	if metrics.requests, err = registerCounterVec(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ideaboard",
		Subsystem: "search",
		Name:      "requests_total",
		Help:      "Search requests by outcome.",
	}, []string{"outcome"})); err != nil {
		return nil, err
	}
	if metrics.duration, err = registerHistogram(registerer, prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "ideaboard",
		Subsystem: "search",
		Name:      "duration_seconds",
		Help:      "Search request duration in seconds.",
	})); err != nil {
		return nil, err
	}
	if metrics.providerRequest, err = registerCounterVec(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ideaboard",
		Subsystem: "search",
		Name:      "provider_requests_total",
		Help:      "Search provider requests by result status and failure category.",
	}, []string{"provider", "status", "failure_category"})); err != nil {
		return nil, err
	}
	if metrics.providerLatency, err = registerHistogramVec(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "ideaboard",
		Subsystem: "search",
		Name:      "provider_latency_seconds",
		Help:      "Search provider request latency in seconds.",
	}, []string{"provider"})); err != nil {
		return nil, err
	}
	if metrics.resultCount, err = registerHistogramVec(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "ideaboard",
		Subsystem: "search",
		Name:      "result_count",
		Help:      "Search result count by page type.",
	}, []string{"page"})); err != nil {
		return nil, err
	}
	if metrics.imageFailures, err = registerCounterVec(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ideaboard",
		Subsystem: "search",
		Name:      "image_failures_total",
		Help:      "Search gallery image load failures by provider.",
	}, []string{"provider"})); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (m *prometheusSearchMetrics) RecordSearch(observation SearchObservation) {
	if m == nil {
		return
	}
	outcome := observation.Outcome
	if outcome == "" {
		outcome = "success"
	}
	m.requests.WithLabelValues(outcome).Inc()
	m.duration.Observe(observation.Duration.Seconds())
	page := observation.Page
	if page == "" {
		page = "first"
	}
	m.resultCount.WithLabelValues(page).Observe(float64(observation.ResultCount))
	for _, status := range observation.ProviderStatus {
		failureCategory := string(status.FailureCategory)
		if failureCategory == "" {
			failureCategory = "none"
		}
		m.providerRequest.WithLabelValues(string(status.Provider), status.Status, failureCategory).Inc()
		m.providerLatency.WithLabelValues(string(status.Provider)).Observe(float64(status.LatencyMS) / 1000)
	}
}

func (m *prometheusSearchMetrics) RecordImageFailure(provider Provider) {
	if m == nil || !isProvider(provider) {
		return
	}
	m.imageFailures.WithLabelValues(string(provider)).Inc()
}

func registerCounterVec(registerer prometheus.Registerer, collector *prometheus.CounterVec) (*prometheus.CounterVec, error) {
	if err := registerer.Register(collector); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if typed, ok := existing.ExistingCollector.(*prometheus.CounterVec); ok {
				return typed, nil
			}
		}
		return nil, fmt.Errorf("register search counter: %w", err)
	}
	return collector, nil
}

func registerHistogram(registerer prometheus.Registerer, collector prometheus.Histogram) (prometheus.Histogram, error) {
	if err := registerer.Register(collector); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if typed, ok := existing.ExistingCollector.(prometheus.Histogram); ok {
				return typed, nil
			}
		}
		return nil, fmt.Errorf("register search histogram: %w", err)
	}
	return collector, nil
}

func registerHistogramVec(registerer prometheus.Registerer, collector *prometheus.HistogramVec) (*prometheus.HistogramVec, error) {
	if err := registerer.Register(collector); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if typed, ok := existing.ExistingCollector.(*prometheus.HistogramVec); ok {
				return typed, nil
			}
		}
		return nil, fmt.Errorf("register search histogram: %w", err)
	}
	return collector, nil
}
