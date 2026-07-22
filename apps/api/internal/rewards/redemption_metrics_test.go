package rewards

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRedemptionMetricsRecordsLowCardinalityOutcomes(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	recorder := &memoryRedemptionMetricsRecorder{}
	metrics := newRedemptionMetrics(func() time.Time { return now }, recorder)
	outcomes := []redemptionOutcome{
		redemptionOutcomeSuccess,
		redemptionOutcomeReplayed,
		redemptionOutcomeInvalid,
		redemptionOutcomeAlreadyRedeemed,
		redemptionOutcomeExhausted,
		redemptionOutcomeRateLimited,
		redemptionOutcomeInternalError,
	}

	for index, outcome := range outcomes {
		finish := metrics.Start()
		now = now.Add(time.Duration(index+1) * time.Millisecond)
		finish(outcome, int64(index+1), nil)
	}

	observations := recorder.Observations()
	if len(observations) != len(outcomes) {
		t.Fatalf("observation count = %d, want %d", len(observations), len(outcomes))
	}
	for index, outcome := range outcomes {
		observation := observations[index]
		if observation.Outcome != outcome {
			t.Fatalf("observation %d outcome = %q, want %q", index, observation.Outcome, outcome)
		}
		if observation.Latency != time.Duration(index+1)*time.Millisecond {
			t.Fatalf("observation %d latency = %s", index, observation.Latency)
		}
		wantCredits := int64(0)
		if outcome == redemptionOutcomeSuccess {
			wantCredits = int64(index + 1)
		}
		if observation.CreditedCredits != wantCredits {
			t.Fatalf("observation %d credited Credits = %d, want %d", index, observation.CreditedCredits, wantCredits)
		}
	}
}

func TestRedemptionMetricsDefaultsToSafeObservationValues(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	recorder := &memoryRedemptionMetricsRecorder{}
	metrics := newRedemptionMetrics(func() time.Time { return now }, recorder)
	finish := metrics.Start()

	now = now.Add(-time.Second)
	finish(redemptionOutcome("unexpected-sensitive-value"), -10, nil)

	observation := recorder.Observations()[0]
	if observation != (redemptionMetricObservation{Outcome: redemptionOutcomeInternalError}) {
		t.Fatalf("safe observation = %+v", observation)
	}
}

func TestRedemptionMetricsUsesNoopRecorderByDefault(t *testing.T) {
	metrics := newRedemptionMetrics(nil, nil)
	finish := metrics.Start()
	finish(redemptionOutcomeSuccess, 10, nil)
}

func TestPrometheusRedemptionMetricsRecorderExportsProductionCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder, err := newPrometheusRedemptionMetricsRecorder(registry)
	if err != nil {
		t.Fatalf("newPrometheusRedemptionMetricsRecorder() error = %v", err)
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

	want := `
# HELP ideaboard_rewards_credited_credits_total Credits granted by successful reward redemptions.
# TYPE ideaboard_rewards_credited_credits_total counter
ideaboard_rewards_credited_credits_total 25
# HELP ideaboard_rewards_database_conflicts_total PostgreSQL conflicts encountered by reward redemptions.
# TYPE ideaboard_rewards_database_conflicts_total counter
ideaboard_rewards_database_conflicts_total 1
# HELP ideaboard_rewards_redemption_latency_seconds Reward redemption request latency in seconds.
# TYPE ideaboard_rewards_redemption_latency_seconds histogram
ideaboard_rewards_redemption_latency_seconds_bucket{le="0.005"} 0
ideaboard_rewards_redemption_latency_seconds_bucket{le="0.01"} 0
ideaboard_rewards_redemption_latency_seconds_bucket{le="0.025"} 0
ideaboard_rewards_redemption_latency_seconds_bucket{le="0.05"} 0
ideaboard_rewards_redemption_latency_seconds_bucket{le="0.1"} 0
ideaboard_rewards_redemption_latency_seconds_bucket{le="0.25"} 1
ideaboard_rewards_redemption_latency_seconds_bucket{le="0.5"} 1
ideaboard_rewards_redemption_latency_seconds_bucket{le="1"} 1
ideaboard_rewards_redemption_latency_seconds_bucket{le="2.5"} 2
ideaboard_rewards_redemption_latency_seconds_bucket{le="5"} 2
ideaboard_rewards_redemption_latency_seconds_bucket{le="10"} 2
ideaboard_rewards_redemption_latency_seconds_bucket{le="+Inf"} 2
ideaboard_rewards_redemption_latency_seconds_sum 1.5
ideaboard_rewards_redemption_latency_seconds_count 2
# HELP ideaboard_rewards_redemptions_total Reward redemption requests by outcome.
# TYPE ideaboard_rewards_redemptions_total counter
ideaboard_rewards_redemptions_total{outcome="internal_error"} 1
ideaboard_rewards_redemptions_total{outcome="success"} 1
`
	if err := testutil.GatherAndCompare(
		registry,
		strings.NewReader(want),
		"ideaboard_rewards_credited_credits_total",
		"ideaboard_rewards_database_conflicts_total",
		"ideaboard_rewards_redemption_latency_seconds",
		"ideaboard_rewards_redemptions_total",
	); err != nil {
		t.Fatal(err)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
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

func TestRedemptionMetricsClassifiesWrappedPostgresConflicts(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{code: "23505", want: true},
		{code: "40001", want: true},
		{code: "40P01", want: true},
		{code: "23503", want: false},
		{code: "22001", want: false},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			recorder := &memoryRedemptionMetricsRecorder{}
			finish := newRedemptionMetrics(time.Now, recorder).Start()
			err := fmt.Errorf("redeem: %w", &pgconn.PgError{Code: test.code})
			finish(redemptionOutcomeInternalError, 0, err)
			if got := recorder.Observations()[0].DatabaseConflict; got != test.want {
				t.Fatalf("database conflict for SQLSTATE %q = %v, want %v", test.code, got, test.want)
			}
		})
	}
	recorder := &memoryRedemptionMetricsRecorder{}
	finish := newRedemptionMetrics(time.Now, recorder).Start()
	finish(redemptionOutcomeInternalError, 0, errors.New("database unavailable"))
	if recorder.Observations()[0].DatabaseConflict {
		t.Fatal("non-PostgreSQL error classified as database conflict")
	}
}

type memoryRedemptionMetricsRecorder struct {
	mu           sync.Mutex
	observations []redemptionMetricObservation
}

func (r *memoryRedemptionMetricsRecorder) RecordRedemption(observation redemptionMetricObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observations = append(r.observations, observation)
}

func (r *memoryRedemptionMetricsRecorder) Observations() []redemptionMetricObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]redemptionMetricObservation(nil), r.observations...)
}
