package worker

// Task settlement sweep worker tests.
//
// These pin the parts of the sweep that do not need a database: the worker
// wiring, the code-owned River budget, the liveness cutoff it hands the
// convergence, and the guarantee that a task-level failure inside a batch does
// not fail the whole River job (one un-releasable hold must never keep every
// other abandoned hold locked). The end-to-end convergence over the real store,
// ledger, and settle transaction is proven in workspacetasks
// (settle_stalled_test.go).

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/platform/riverpolicy"
	taskapp "ideaboard/api/internal/tasking/application/workspacetasks"
	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/riverqueue/river"
)

type sweepScanCall struct {
	stalledBefore time.Time
	limit         int
}

type sweepFakeStore struct {
	rows  []tasks.StalledSettlement
	err   error
	calls []sweepScanCall
}

func (s *sweepFakeStore) ListStalledHeldSettlements(_ context.Context, stalledBefore time.Time, limit int) ([]tasks.StalledSettlement, error) {
	s.calls = append(s.calls, sweepScanCall{stalledBefore: stalledBefore, limit: limit})
	return s.rows, s.err
}

type sweepFakeSettler struct {
	errs  map[string]error
	calls []string
}

func (f *sweepFakeSettler) Fail(_ context.Context, workspaceID, taskID, _ string) (*tasks.WorkspaceTask, error) {
	f.calls = append(f.calls, workspaceID+"/"+taskID)
	if err, ok := f.errs[taskID]; ok {
		return nil, err
	}
	return &tasks.WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, Status: tasks.StatusFailed}, nil
}

func newTestSweepWorker(store TaskSettlementSweepStore, settler TaskSettlementSweepSettler, now time.Time) *TaskSettlementSweepWorker {
	worker := NewTaskSettlementSweepWorker(store, settler)
	worker.now = func() time.Time { return now }
	return worker
}

// TestTaskSettlementSweepUsesCodeOwnedBudget pins that the sweep takes its
// budget from the matrices rather than inheriting the client-level drawing
// timeout, and that the budget stays below the periodic interval so a stuck run
// times out instead of piling up behind the next one.
func TestTaskSettlementSweepUsesCodeOwnedBudget(t *testing.T) {
	worker := newTestSweepWorker(&sweepFakeStore{}, &sweepFakeSettler{}, time.Now())
	if got := worker.Timeout(nil); got != riverpolicy.RiverJobTimeoutTaskSettlementSweep {
		t.Fatalf("Timeout = %s, want the matrix constant %s", got, riverpolicy.RiverJobTimeoutTaskSettlementSweep)
	}
	if got := worker.MaxAttempts(nil); got != riverpolicy.RiverMaxAttemptsTaskSettlementSweep {
		t.Fatalf("MaxAttempts = %d, want the matrix constant %d", got, riverpolicy.RiverMaxAttemptsTaskSettlementSweep)
	}
	if riverpolicy.RiverJobTimeoutTaskSettlementSweep >= taskSettlementSweepPeriod {
		t.Fatalf("sweep timeout %s must stay below the %s periodic interval", riverpolicy.RiverJobTimeoutTaskSettlementSweep, taskSettlementSweepPeriod)
	}
	if riverpolicy.RiverJobTimeoutTaskSettlementSweep >= riverpolicy.RiverJobTimeoutDrawing {
		t.Fatal("a settlement-only sweep must not carry the provider-sized drawing budget")
	}
}

// TestTaskSettlementSweepScansAtTheLivenessThreshold pins the cutoff: the
// sweep may only look at rows no worker has touched for the full threshold, so
// a healthy long-running task is never a candidate.
func TestTaskSettlementSweepScansAtTheLivenessThreshold(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	store := &sweepFakeStore{rows: []tasks.StalledSettlement{
		{WorkspaceID: "ws_1", TaskID: "task_1"},
	}}
	settler := &sweepFakeSettler{}
	worker := newTestSweepWorker(store, settler, now)

	if err := worker.Work(context.Background(), &river.Job[taskSettlementSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("scan calls = %d, want 1", len(store.calls))
	}
	wantCutoff := now.Add(-taskapp.StalledSettlementThreshold)
	if !store.calls[0].stalledBefore.Equal(wantCutoff) {
		t.Fatalf("cutoff = %s, want %s", store.calls[0].stalledBefore, wantCutoff)
	}
	if store.calls[0].limit != taskapp.StalledSettlementSweepBatch {
		t.Fatalf("limit = %d, want the kernel bound %d", store.calls[0].limit, taskapp.StalledSettlementSweepBatch)
	}
	if len(settler.calls) != 1 || settler.calls[0] != "ws_1/task_1" {
		t.Fatalf("Fail calls = %v, want the discovered pair", settler.calls)
	}
}

// TestTaskSettlementSweepIsolatesTaskFailures pins the job semantics: a task
// the sweep cannot converge is reported but never fails the River job, because
// the sweep is periodic and idempotent — the next run retries the same batch,
// while failing the job would burn the attempt budget on a poisoned row.
func TestTaskSettlementSweepIsolatesTaskFailures(t *testing.T) {
	ledgerDown := errors.New("ledger unavailable")
	store := &sweepFakeStore{rows: []tasks.StalledSettlement{
		{WorkspaceID: "ws_1", TaskID: "task_bad"},
		{WorkspaceID: "ws_1", TaskID: "task_good"},
	}}
	settler := &sweepFakeSettler{errs: map[string]error{"task_bad": ledgerDown}}
	worker := newTestSweepWorker(store, settler, time.Now())

	if err := worker.Work(context.Background(), &river.Job[taskSettlementSweepArgs]{}); err != nil {
		t.Fatalf("Work must not fail the job on a per-task failure: %v", err)
	}
	if len(settler.calls) != 2 {
		t.Fatalf("Fail calls = %v, want both tasks attempted", settler.calls)
	}
}

// TestTaskSettlementSweepFailsClosed pins the sweep-level failures: missing
// wiring and a discovery outage must surface (River retries, then the operator
// sees a failed job) instead of converging nothing quietly.
func TestTaskSettlementSweepFailsClosed(t *testing.T) {
	if err := (&TaskSettlementSweepWorker{}).Work(context.Background(), nil); err == nil {
		t.Fatal("an unwired sweep must fail closed")
	}
	discoveryDown := errors.New("database unavailable")
	worker := newTestSweepWorker(&sweepFakeStore{err: discoveryDown}, &sweepFakeSettler{}, time.Now())
	if err := worker.Work(context.Background(), nil); !errors.Is(err, discoveryDown) {
		t.Fatalf("Work err = %v, want the discovery cause surfaced", err)
	}
}

// TestTaskSettlementSweepPeriodicJobIsWired pins the schedule: one periodic
// job on its own maintenance queue with the matrix budget, and NOT on startup
// (there is nothing a boot-time sweep could converge that the next period
// would not).
func TestTaskSettlementSweepPeriodicJobIsWired(t *testing.T) {
	if jobs := TaskSettlementSweepPeriodicJobs(); len(jobs) != 1 {
		t.Fatalf("periodic jobs = %d, want 1", len(jobs))
	}
	args, opts := taskSettlementSweepInsert()
	if args.Kind() != "task_settlement_sweep" {
		t.Fatalf("kind = %q, want task_settlement_sweep", args.Kind())
	}
	if opts.Queue != riverpolicy.RiverQueueTaskSettlementSweep {
		t.Fatalf("queue = %q, want %q", opts.Queue, riverpolicy.RiverQueueTaskSettlementSweep)
	}
	if opts.MaxAttempts != riverpolicy.RiverMaxAttemptsTaskSettlementSweep {
		t.Fatalf("max attempts = %d, want %d", opts.MaxAttempts, riverpolicy.RiverMaxAttemptsTaskSettlementSweep)
	}
	start := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if got := (taskSettlementSweepSchedule{}).Next(start); !got.Equal(start.Add(taskSettlementSweepPeriod)) {
		t.Fatalf("next run = %s, want +%s", got, taskSettlementSweepPeriod)
	}
	if taskSettlementSweepPeriod >= taskapp.StalledSettlementThreshold {
		t.Fatalf("sweep period %s must stay far below the %s threshold", taskSettlementSweepPeriod, taskapp.StalledSettlementThreshold)
	}
}
