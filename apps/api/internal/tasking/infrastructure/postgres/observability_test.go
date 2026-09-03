package postgres

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	taskbilling "ideaboard/api/internal/tasking/ports/billing"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestTaskstoreMetricsClassifiesBoundaryAndDatabaseConflicts(t *testing.T) {
	leaseErr := fmt.Errorf("release: %w", runtimecontracts.ErrTaskLeaseStateConflict)
	idempotencyErr := fmt.Errorf("claim: %w", taskbilling.ErrIdempotencyConflict)
	databaseErr := fmt.Errorf("insert: %w", &pgconn.PgError{Code: "40001"})
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"lease", leaseErr, taskstoreOutcomeConflict},
		{"idempotency", idempotencyErr, taskstoreOutcomeConflict},
		{"database", databaseErr, taskstoreOutcomeConflict},
		{"cancel", context.Canceled, taskstoreOutcomeCanceled},
		{"success", nil, taskstoreOutcomeSuccess},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyTaskstoreOutcome(taskstoreOutcomeSuccess, tc.err); got != tc.want {
				t.Fatalf("classifyTaskstoreOutcome()=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestTaskstoreMetricsRecorderUsesBoundedLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder, err := newPrometheusTaskstoreMetricsRecorder(registry)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	recorder.RecordTransaction(taskstoreTransactionObservation{Operation: taskstoreOperationCreate, Outcome: taskstoreOutcomeSuccess, Latency: 25 * time.Millisecond})
	recorder.RecordTransaction(taskstoreTransactionObservation{Operation: taskstoreOperationSubmissionClaim, Outcome: taskstoreOutcomeReplayed, Latency: 50 * time.Millisecond})
	recorder.RecordTransaction(taskstoreTransactionObservation{Operation: taskstoreOperationConvergence, Outcome: taskstoreOutcomeError, Err: fmt.Errorf("%w", runtimecontracts.ErrTaskLeaseStateConflict), Latency: 10 * time.Millisecond})
	recorder.RecordLock(taskstoreLockObservation{Lock: taskstoreLockSubmissionClaim, Outcome: taskstoreOutcomeSuccess, Wait: 5 * time.Millisecond})

	if err := testutil.GatherAndCompare(registry, strings.NewReader(`
# HELP ideaboard_taskstore_transactions_total Taskstore transactional boundary operations by outcome.
# TYPE ideaboard_taskstore_transactions_total counter
ideaboard_taskstore_transactions_total{operation="create",outcome="success"} 1
ideaboard_taskstore_transactions_total{operation="submission_claim",outcome="replayed"} 1
ideaboard_taskstore_transactions_total{operation="convergence",outcome="conflict"} 1
`), "ideaboard_taskstore_transactions_total"); err != nil {
		t.Fatal(err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "task_id" || label.GetName() == "user_id" || label.GetName() == "client_action_id" {
					t.Fatalf("high-cardinality label %q found on %s", label.GetName(), family.GetName())
				}
			}
		}
	}
}

func TestTaskstoreMetricsStartClampsNegativeLatency(t *testing.T) {
	clock := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	recorder := &memoryTaskstoreMetricsRecorder{}
	metrics := newTaskstoreMetrics(func() time.Time { return clock }, recorder)
	finish := metrics.StartTransaction(taskstoreOperationConvergence)
	clock = clock.Add(-time.Second)
	finish(taskstoreOutcomeSuccess, nil)
	if got := recorder.transactions[0].Latency; got != 0 {
		t.Fatalf("negative latency=%s, want 0", got)
	}
}

type memoryTaskstoreMetricsRecorder struct {
	mu           sync.Mutex
	transactions []taskstoreTransactionObservation
	locks        []taskstoreLockObservation
}

func (r *memoryTaskstoreMetricsRecorder) RecordTransaction(observation taskstoreTransactionObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transactions = append(r.transactions, observation)
}

func (r *memoryTaskstoreMetricsRecorder) RecordLock(observation taskstoreLockObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.locks = append(r.locks, observation)
}
