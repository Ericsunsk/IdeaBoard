package idempotency

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Rejection outcomes stay closed so missing keys, key-reuse conflicts and
// first-result replays are independently alertable (idempotency storm vs
// client misintegration vs healthy retry).
const (
	OutcomeMissing  = "missing"
	OutcomeConflict = "conflict"
	OutcomeReplay   = "replay"
)

// Metrics carries the idempotency rejection counter. Use DefaultMetrics in
// production (registered on the default registry scraped by /metrics);
// tests build an isolated instance with NewMetrics(prometheus.NewRegistry()).
type Metrics struct {
	rejections *prometheus.CounterVec
}

// NewMetrics registers the idempotency counters on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		rejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "idempotency_rejections_total",
			Help: "POST write idempotency rejections by outcome (missing/conflict/replay).",
		}, []string{"outcome"}),
	}
	if reg != nil {
		reg.MustRegister(m.rejections)
	}
	return m
}

// DefaultMetrics feeds the process-wide /metrics endpoint.
var DefaultMetrics = NewMetrics(prometheus.DefaultRegisterer)

// Observe records one idempotency rejection. Outcome must be one of the
// closed Outcome* values; unknown values normalize to missing. A nil
// receiver is a no-op so uninitialized call paths never crash.
func (m *Metrics) Observe(outcome string) {
	if m == nil {
		return
	}
	switch outcome {
	case OutcomeMissing, OutcomeConflict, OutcomeReplay:
		m.rejections.WithLabelValues(outcome).Inc()
	default:
		m.rejections.WithLabelValues(OutcomeMissing).Inc()
	}
}

// ObserveError classifies a Parse/Resolve/Lookup error into missing vs
// conflict. Replay is recorded separately via ObserveReplay since a replay
// is not an error.
func (m *Metrics) ObserveError(err error) {
	if m == nil || err == nil {
		return
	}
	if IsConflict(err) {
		m.Observe(OutcomeConflict)
		return
	}
	m.Observe(OutcomeMissing)
}

// ObserveReplay records one first-result replay served for a reused key.
func (m *Metrics) ObserveReplay() {
	m.Observe(OutcomeReplay)
}
