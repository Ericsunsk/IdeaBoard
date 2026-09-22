package idempotency

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRejectionsSplitByOutcome(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())
	metrics.Observe(OutcomeMissing)
	metrics.Observe(OutcomeConflict)
	metrics.Observe(OutcomeReplay)
	metrics.Observe(OutcomeReplay)

	if got := testutil.ToFloat64(metrics.rejections.WithLabelValues(OutcomeMissing)); got != 1 {
		t.Fatalf("missing = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.rejections.WithLabelValues(OutcomeConflict)); got != 1 {
		t.Fatalf("conflict = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.rejections.WithLabelValues(OutcomeReplay)); got != 2 {
		t.Fatalf("replay = %v, want 2", got)
	}
}

func TestObserveErrorClassifiesMissingVsConflict(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())
	metrics.ObserveError(MissingError())
	metrics.ObserveError(InvalidError())
	metrics.ObserveError(ConflictError())

	if got := testutil.ToFloat64(metrics.rejections.WithLabelValues(OutcomeMissing)); got != 2 {
		t.Fatalf("missing = %v, want 2 (missing + invalid)", got)
	}
	if got := testutil.ToFloat64(metrics.rejections.WithLabelValues(OutcomeConflict)); got != 1 {
		t.Fatalf("conflict = %v, want 1", got)
	}
}

func TestNilMetricsNeverCrash(t *testing.T) {
	var metrics *Metrics
	metrics.Observe(OutcomeMissing)
	metrics.ObserveError(ConflictError())
	metrics.ObserveReplay()
}
