package safehttp

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveRecordsNonHTTPRED(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())
	metrics.Observe("stripe", OutcomeSuccess, 25*time.Millisecond)
	metrics.Observe("stripe", OutcomeServerError, 120*time.Millisecond)

	if got := testutil.ToFloat64(metrics.requests.WithLabelValues("stripe", OutcomeSuccess)); got != 1 {
		t.Fatalf("stripe success = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.requests.WithLabelValues("stripe", OutcomeServerError)); got != 1 {
		t.Fatalf("stripe server error = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(metrics.durations); got != 2 {
		t.Fatalf("duration series = %d, want 2", got)
	}
}

func TestObserveNormalizesEmptyOutcome(t *testing.T) {
	metrics := NewMetrics(prometheus.NewRegistry())
	metrics.Observe("s3-storage", "", time.Millisecond)
	if got := testutil.ToFloat64(metrics.requests.WithLabelValues("s3-storage", OutcomeTransportErr)); got != 1 {
		t.Fatalf("empty outcome = %v, want normalized transport_error 1", got)
	}
}

func TestNilMetricsObserveNeverCrashes(t *testing.T) {
	var metrics *Metrics
	metrics.Observe("stripe", OutcomeSuccess, time.Millisecond)
}
