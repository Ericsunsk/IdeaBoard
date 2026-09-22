package http

import (
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
)

type RedemptionOutcome string

const (
	RedemptionOutcomeSuccess       RedemptionOutcome = "success"
	RedemptionOutcomeReplayed      RedemptionOutcome = "replayed"
	RedemptionOutcomeInvalid       RedemptionOutcome = "invalid"
	RedemptionOutcomeRateLimited   RedemptionOutcome = "rate_limited"
	RedemptionOutcomeInternalError RedemptionOutcome = "internal_error"

	redemptionOutcomeSuccess       = RedemptionOutcomeSuccess
	redemptionOutcomeReplayed      = RedemptionOutcomeReplayed
	redemptionOutcomeInvalid       = RedemptionOutcomeInvalid
	redemptionOutcomeRateLimited   = RedemptionOutcomeRateLimited
	redemptionOutcomeInternalError = RedemptionOutcomeInternalError

	uniqueViolationSQLState      = "23505"
	serializationFailureSQLState = "40001"
	deadlockDetectedSQLState     = "40P01"
)

type RedemptionMetricObservation struct {
	Outcome          RedemptionOutcome
	CreditedCredits  int64
	Latency          time.Duration
	DatabaseConflict bool
}

type RedemptionMetricsRecorder interface {
	RecordRedemption(RedemptionMetricObservation)
}

type (
	redemptionOutcome           = RedemptionOutcome
	redemptionMetricObservation = RedemptionMetricObservation
	redemptionMetricsRecorder   = RedemptionMetricsRecorder
)

type noopRedemptionMetricsRecorder struct{}

func (noopRedemptionMetricsRecorder) RecordRedemption(redemptionMetricObservation) {}

type prometheusRedemptionMetricsRecorder struct {
	redemptions       *prometheus.CounterVec
	creditedCredits   prometheus.Counter
	latency           prometheus.Histogram
	databaseConflicts prometheus.Counter
}

func newPrometheusRedemptionMetricsRecorder(registerer prometheus.Registerer) (*prometheusRedemptionMetricsRecorder, error) {
	if registerer == nil {
		return nil, fmt.Errorf("register reward redemption metrics: registerer is required")
	}
	recorder := &prometheusRedemptionMetricsRecorder{
		redemptions:       prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "ideaboard", Subsystem: "rewards", Name: "redemptions_total", Help: "Reward redemption requests by outcome."}, []string{"outcome"}),
		creditedCredits:   prometheus.NewCounter(prometheus.CounterOpts{Namespace: "ideaboard", Subsystem: "rewards", Name: "credited_credits_total", Help: "Credits granted by successful reward redemptions."}),
		latency:           prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: "ideaboard", Subsystem: "rewards", Name: "redemption_latency_seconds", Help: "Reward redemption request latency in seconds."}),
		databaseConflicts: prometheus.NewCounter(prometheus.CounterOpts{Namespace: "ideaboard", Subsystem: "rewards", Name: "database_conflicts_total", Help: "PostgreSQL conflicts encountered by reward redemptions."}),
	}
	for _, collector := range []prometheus.Collector{recorder.redemptions, recorder.creditedCredits, recorder.latency, recorder.databaseConflicts} {
		if err := registerer.Register(collector); err != nil {
			return nil, fmt.Errorf("register reward redemption metrics: %w", err)
		}
	}
	return recorder, nil
}

func mustNewPrometheusRedemptionMetricsRecorder(registerer prometheus.Registerer) *prometheusRedemptionMetricsRecorder {
	recorder, err := newPrometheusRedemptionMetricsRecorder(registerer)
	if err != nil {
		panic(err)
	}
	return recorder
}

var defaultRedemptionMetricsRecorder = mustNewPrometheusRedemptionMetricsRecorder(prometheus.DefaultRegisterer)

func (r *prometheusRedemptionMetricsRecorder) RecordRedemption(observation redemptionMetricObservation) {
	if r == nil {
		return
	}
	r.redemptions.WithLabelValues(string(normalizeRedemptionOutcome(observation.Outcome))).Inc()
	if observation.CreditedCredits > 0 {
		r.creditedCredits.Add(float64(observation.CreditedCredits))
	}
	r.latency.Observe(observation.Latency.Seconds())
	if observation.DatabaseConflict {
		r.databaseConflicts.Inc()
	}
}

func isRedemptionDatabaseConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.SQLState() {
	case uniqueViolationSQLState, serializationFailureSQLState, deadlockDetectedSQLState:
		return true
	default:
		return false
	}
}

type redemptionMetrics struct {
	now      func() time.Time
	recorder redemptionMetricsRecorder
}

func newRedemptionMetrics(now func() time.Time, recorder redemptionMetricsRecorder) redemptionMetrics {
	if now == nil {
		now = time.Now
	}
	if recorder == nil {
		recorder = noopRedemptionMetricsRecorder{}
	}
	return redemptionMetrics{now: now, recorder: recorder}
}

func (m redemptionMetrics) Start() func(redemptionOutcome, int64, error) {
	if m.now == nil {
		m.now = time.Now
	}
	if m.recorder == nil {
		m.recorder = noopRedemptionMetricsRecorder{}
	}
	startedAt := m.now()
	return func(outcome redemptionOutcome, creditedCredits int64, err error) {
		latency := m.now().Sub(startedAt)
		if latency < 0 {
			latency = 0
		}
		outcome = normalizeRedemptionOutcome(outcome)
		if creditedCredits < 0 || outcome != redemptionOutcomeSuccess {
			creditedCredits = 0
		}
		m.recorder.RecordRedemption(redemptionMetricObservation{Outcome: outcome, CreditedCredits: creditedCredits, Latency: latency, DatabaseConflict: isRedemptionDatabaseConflict(err)})
	}
}

func normalizeRedemptionOutcome(outcome redemptionOutcome) redemptionOutcome {
	switch outcome {
	case redemptionOutcomeSuccess, redemptionOutcomeReplayed, redemptionOutcomeInvalid, redemptionOutcomeRateLimited, redemptionOutcomeInternalError:
		return outcome
	default:
		return redemptionOutcomeInternalError
	}
}
