package http

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type metricObservationRecorder struct {
	observations []redemptionMetricObservation
}

func (r *metricObservationRecorder) RecordRedemption(observation redemptionMetricObservation) {
	r.observations = append(r.observations, observation)
}

func TestRedemptionMetricsNormalizesAndBoundsObservation(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	recorder := &metricObservationRecorder{}
	metrics := newRedemptionMetrics(func() time.Time { return now }, recorder)
	finish := metrics.Start()
	now = now.Add(-time.Second)
	finish(redemptionOutcome("unexpected"), -10, nil)
	if got := recorder.observations[0]; got != (redemptionMetricObservation{Outcome: redemptionOutcomeInternalError}) {
		t.Fatalf("observation = %+v", got)
	}
}

func TestRedemptionMetricsClassifiesDatabaseConflict(t *testing.T) {
	recorder := &metricObservationRecorder{}
	finish := newRedemptionMetrics(time.Now, recorder).Start()
	finish(redemptionOutcomeInternalError, 0, fmt.Errorf("redeem: %w", &pgconn.PgError{Code: "23505"}))
	if !recorder.observations[0].DatabaseConflict {
		t.Fatal("expected PostgreSQL unique violation to be classified as conflict")
	}
}

func TestPrometheusRedemptionMetricsExportsBoundedObservations(t *testing.T) {
	if _, err := newPrometheusRedemptionMetricsRecorder(nil); err == nil {
		t.Fatal("expected nil registerer error")
	}
	registry := prometheus.NewRegistry()
	recorder, err := newPrometheusRedemptionMetricsRecorder(registry)
	if err != nil {
		t.Fatalf("new recorder error = %v", err)
	}
	recorder.RecordRedemption(redemptionMetricObservation{
		Outcome:         redemptionOutcomeSuccess,
		CreditedCredits: 25,
		Latency:         1250 * time.Millisecond,
	})
	recorder.RecordRedemption(redemptionMetricObservation{
		Outcome:          redemptionOutcomeInternalError,
		DatabaseConflict: true,
		Latency:          250 * time.Millisecond,
	})

	if got := testutil.ToFloat64(recorder.redemptions.WithLabelValues("success")); got != 1 {
		t.Fatalf("success redemptions = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.creditedCredits); got != 25 {
		t.Fatalf("credited credits = %v, want 25", got)
	}
	if got := testutil.ToFloat64(recorder.databaseConflicts); got != 1 {
		t.Fatalf("database conflicts = %v, want 1", got)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				switch label.GetName() {
				case "user_id", "reward_code_id", "code", "digest":
					t.Fatalf("metric %q contains forbidden label %q", family.GetName(), label.GetName())
				}
			}
		}
	}
}
