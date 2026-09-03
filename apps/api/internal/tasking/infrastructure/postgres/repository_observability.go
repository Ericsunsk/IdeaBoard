package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	taskbilling "ideaboard/api/internal/tasking/ports/billing"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
)

// Taskstore metrics deliberately use only operation and outcome labels. Task,
// user, lease, and client-action identifiers are high-cardinality facts and
// must remain available through traces and admin diagnostics rather than
// Prometheus labels.
type taskstoreTransactionObservation struct {
	Operation string
	Outcome   string
	Latency   time.Duration
	Err       error
}

type taskstoreLockObservation struct {
	Lock    string
	Outcome string
	Wait    time.Duration
}

type taskstoreMetricsRecorder interface {
	RecordTransaction(taskstoreTransactionObservation)
	RecordLock(taskstoreLockObservation)
}

type noopTaskstoreMetricsRecorder struct{}

func (noopTaskstoreMetricsRecorder) RecordTransaction(taskstoreTransactionObservation) {}
func (noopTaskstoreMetricsRecorder) RecordLock(taskstoreLockObservation)               {}

type prometheusTaskstoreMetricsRecorder struct {
	transactions       *prometheus.CounterVec
	transactionLatency *prometheus.HistogramVec
	lockWait           *prometheus.HistogramVec
	boundaryConflicts  *prometheus.CounterVec
	databaseConflicts  *prometheus.CounterVec
}

func newPrometheusTaskstoreMetricsRecorder(registerer prometheus.Registerer) (*prometheusTaskstoreMetricsRecorder, error) {
	if registerer == nil {
		return nil, fmt.Errorf("register taskstore metrics: registerer is required")
	}
	recorder := &prometheusTaskstoreMetricsRecorder{
		transactions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ideaboard", Subsystem: "taskstore", Name: "transactions_total",
			Help: "Taskstore transactional boundary operations by outcome.",
		}, []string{"operation", "outcome"}),
		transactionLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "ideaboard", Subsystem: "taskstore", Name: "transaction_latency_seconds",
			Help: "Taskstore transactional boundary latency by operation.",
		}, []string{"operation"}),
		lockWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "ideaboard", Subsystem: "taskstore", Name: "lock_wait_seconds",
			Help: "PostgreSQL advisory lock acquisition wait by logical lock.",
		}, []string{"lock", "outcome"}),
		boundaryConflicts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ideaboard", Subsystem: "taskstore", Name: "boundary_conflicts_total",
			Help: "Taskstore lease and idempotency boundary conflicts.",
		}, []string{"boundary"}),
		databaseConflicts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ideaboard", Subsystem: "taskstore", Name: "database_conflicts_total",
			Help: "PostgreSQL serialization, deadlock, and unique conflicts by operation.",
		}, []string{"operation"}),
	}
	for _, collector := range []prometheus.Collector{
		recorder.transactions,
		recorder.transactionLatency,
		recorder.lockWait,
		recorder.boundaryConflicts,
		recorder.databaseConflicts,
	} {
		if err := registerer.Register(collector); err != nil {
			return nil, fmt.Errorf("register taskstore metrics: %w", err)
		}
	}
	return recorder, nil
}

func mustNewPrometheusTaskstoreMetricsRecorder(registerer prometheus.Registerer) *prometheusTaskstoreMetricsRecorder {
	recorder, err := newPrometheusTaskstoreMetricsRecorder(registerer)
	if err != nil {
		panic(err)
	}
	return recorder
}

var defaultTaskstoreMetricsRecorder = mustNewPrometheusTaskstoreMetricsRecorder(prometheus.DefaultRegisterer)

func (r *prometheusTaskstoreMetricsRecorder) RecordTransaction(observation taskstoreTransactionObservation) {
	if r == nil {
		return
	}
	operation := normalizeTaskstoreOperation(observation.Operation)
	outcome := classifyTaskstoreOutcome(observation.Outcome, observation.Err)
	r.transactions.WithLabelValues(operation, outcome).Inc()
	r.transactionLatency.WithLabelValues(operation).Observe(nonNegativeDuration(observation.Latency).Seconds())
	if isTaskstoreBoundaryConflict(observation.Err) {
		r.boundaryConflicts.WithLabelValues(taskstoreConflictBoundary(observation.Err)).Inc()
	}
	if isTaskstoreDatabaseConflict(observation.Err) {
		r.databaseConflicts.WithLabelValues(operation).Inc()
	}
}

func (r *prometheusTaskstoreMetricsRecorder) RecordLock(observation taskstoreLockObservation) {
	if r == nil {
		return
	}
	r.lockWait.WithLabelValues(normalizeTaskstoreLock(observation.Lock), classifyTaskstoreOutcome(observation.Outcome, nil)).Observe(nonNegativeDuration(observation.Wait).Seconds())
}

type taskstoreMetrics struct {
	now      func() time.Time
	recorder taskstoreMetricsRecorder
}

func newTaskstoreMetrics(now func() time.Time, recorder taskstoreMetricsRecorder) taskstoreMetrics {
	if now == nil {
		now = time.Now
	}
	if recorder == nil {
		recorder = noopTaskstoreMetricsRecorder{}
	}
	return taskstoreMetrics{now: now, recorder: recorder}
}

func (m taskstoreMetrics) StartTransaction(operation string) func(string, error) {
	if m.now == nil {
		m.now = time.Now
	}
	if m.recorder == nil {
		m.recorder = noopTaskstoreMetricsRecorder{}
	}
	startedAt := m.now()
	return func(outcome string, err error) {
		m.recorder.RecordTransaction(taskstoreTransactionObservation{
			Operation: operation,
			Outcome:   outcome,
			Latency:   nonNegativeDuration(m.now().Sub(startedAt)),
			Err:       err,
		})
	}
}

func (m taskstoreMetrics) RecordLock(lock string, startedAt time.Time, err error) {
	if m.now == nil {
		m.now = time.Now
	}
	if m.recorder == nil {
		m.recorder = noopTaskstoreMetricsRecorder{}
	}
	m.recorder.RecordLock(taskstoreLockObservation{
		Lock: lock, Outcome: classifyTaskstoreOutcome(taskstoreOutcomeSuccess, err), Wait: m.now().Sub(startedAt),
	})
}

const (
	taskstoreOperationCreate          = "create"
	taskstoreOperationSubmissionClaim = "submission_claim"
	taskstoreOperationConvergence     = "convergence"
	taskstoreLockSubmissionClaim      = "submission_claim"

	taskstoreOutcomeSuccess  = "success"
	taskstoreOutcomeReplayed = "replayed"
	taskstoreOutcomeConflict = "conflict"
	taskstoreOutcomeCanceled = "canceled"
	taskstoreOutcomeError    = "error"
)

func classifyTaskstoreOutcome(outcome string, err error) string {
	if isTaskstoreBoundaryConflict(err) || isTaskstoreDatabaseConflict(err) {
		return taskstoreOutcomeConflict
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return taskstoreOutcomeCanceled
	}
	if err != nil {
		return taskstoreOutcomeError
	}
	if outcome == taskstoreOutcomeSuccess || outcome == taskstoreOutcomeReplayed {
		return outcome
	}
	return taskstoreOutcomeError
}

func isTaskstoreBoundaryConflict(err error) bool {
	return errors.Is(err, runtimecontracts.ErrTaskLeaseStateConflict) || errors.Is(err, taskbilling.ErrIdempotencyConflict)
}

func taskstoreConflictBoundary(err error) string {
	if errors.Is(err, runtimecontracts.ErrTaskLeaseStateConflict) {
		return "lease"
	}
	return "idempotency"
}

func isTaskstoreDatabaseConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.SQLState() {
	case "23505", "40001", "40P01":
		return true
	default:
		return false
	}
}

func normalizeTaskstoreOperation(operation string) string {
	switch operation {
	case taskstoreOperationCreate, taskstoreOperationSubmissionClaim, taskstoreOperationConvergence:
		return operation
	default:
		return taskstoreOperationConvergence
	}
}

func normalizeTaskstoreLock(lock string) string {
	if lock == taskstoreLockSubmissionClaim {
		return lock
	}
	return taskstoreLockSubmissionClaim
}

func nonNegativeDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}
	return duration
}
