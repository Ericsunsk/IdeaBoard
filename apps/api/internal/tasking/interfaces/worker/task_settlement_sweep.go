package worker

// Task settlement sweep (the "final timeout" leg of the settlement contract).
//
// The workspace task worker owns terminal convergence for every task it can
// actually reach: Complete -> Capture, Fail -> Release, Cancel -> Release, each
// in one transaction. What it cannot reach is a task whose worker died. A
// SIGKILL after the claim commits, followed by River exhausting the drawing
// queue's attempt budget and discarding the job, leaves tasks.status='running',
// task_billing_settlements.state='held', and credit_holds.status='held' with no
// remaining actor. The Credits stale-hold reconciler does not converge that
// leak (execute_credit_reconciliation only records a 'stale_hold' finding), so
// the sweep exists to give the hold an owner again.
//
// Why a periodic sweep and not an extra River worker kind:
//
//   - The convergence is SCAN-driven ("find every abandoned hold"), not
//     EVENT-driven ("converge the job I just discarded"). River has no discard
//     hook to enqueue from, so an event-driven final-failure kind would still
//     need something to notice the discard — the same scan, one layer deeper.
//   - The existing maintenance-queue precedent (credit_reconciliation,
//     payment_reconciliation, vision_billing) is exactly this shape: a
//     periodic job on its own bounded queue. No new deployment surface, no
//     per-task enqueue, no new failure mode per task.
//
// The sweep converges through workspacetasks.ConvergeStalledSettlements,
// which routes every task through the SAME StalledSettlementFailer seam the
// worker itself settles through. In production that seam is the settler event
// bridge (composition), so a swept task gets the identical single-transaction
// settlement AND the identical post-commit task.failed event on SSE/NATS.

import (
	"context"
	"log/slog"
	"time"

	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

// taskSettlementSweepPeriod is how often the sweep runs. The staleness
// threshold (workspacetasks.StalledSettlementThreshold, 24h) decides WHEN a
// task becomes a candidate; this only decides how long after that an operator
// waits. 15 minutes keeps the post-threshold window negligible while running a
// single indexed, LIMIT-bounded query per period.
const taskSettlementSweepPeriod = 15 * time.Minute

type taskSettlementSweepArgs struct{}

func (taskSettlementSweepArgs) Kind() string { return "task_settlement_sweep" }

// TaskSettlementSweepStore is the read-only discovery seam the sweep needs.
// *taskpostgres.Store satisfies it; the sweep never writes through it.
type TaskSettlementSweepStore interface {
	workspacetasks.StalledSettlementScanner
}

// TaskSettlementSweepSettler is the terminal seam the sweep converges through.
// It is the workspace worker's own settlement interface narrowed to Fail, so
// production wires the same settler event bridge instance the worker holds and
// a swept task publishes its task.failed event through the same post-commit
// path as any other failure.
type TaskSettlementSweepSettler interface {
	workspacetasks.StalledSettlementFailer
}

// TaskSettlementSweepWorker converges abandoned Credit Holds.
type TaskSettlementSweepWorker struct {
	river.WorkerDefaults[taskSettlementSweepArgs]
	store   TaskSettlementSweepStore
	settler TaskSettlementSweepSettler
	// now is injectable so tests can pin the liveness cutoff; production uses
	// time.Now.
	now func() time.Time
}

var _ river.Worker[taskSettlementSweepArgs] = (*TaskSettlementSweepWorker)(nil)

func NewTaskSettlementSweepWorker(store TaskSettlementSweepStore, settler TaskSettlementSweepSettler) *TaskSettlementSweepWorker {
	return &TaskSettlementSweepWorker{store: store, settler: settler, now: time.Now}
}

// Timeout pins the sweep's River budget from the code-owned matrix (it must
// never silently inherit the drawing budget, which is 4x larger).
func (*TaskSettlementSweepWorker) Timeout(*river.Job[taskSettlementSweepArgs]) time.Duration {
	return taskqueue.RiverJobTimeoutTaskSettlementSweep
}

// MaxAttempts pins the sweep's retry budget from the code-owned matrix.
func (*TaskSettlementSweepWorker) MaxAttempts(*river.Job[taskSettlementSweepArgs]) int {
	return taskqueue.RiverMaxAttemptsTaskSettlementSweep
}

// Work runs one bounded sweep.
//
// The job fails ONLY on a sweep-level fault (missing wiring, discovery
// failure, caller cancellation). A task that cannot be converged is isolated
// inside the sweep and reported as a failure entry: it must not abort the rest
// of the batch, because one un-releasable hold must not keep every other
// abandoned hold locked.
func (w *TaskSettlementSweepWorker) Work(ctx context.Context, _ *river.Job[taskSettlementSweepArgs]) error {
	if w == nil || w.store == nil || w.settler == nil {
		return workspacetasks.ErrSettlementUnavailable
	}
	report, err := workspacetasks.ConvergeStalledSettlements(ctx, workspacetasks.StalledConvergenceDeps{
		Scanner:       w.store,
		Failer:        w.settler,
		StalledBefore: w.nowTime().Add(-workspacetasks.StalledSettlementThreshold),
		Limit:         workspacetasks.StalledSettlementSweepBatch,
	})
	for _, failure := range report.Failures {
		// Logged at Error: this is the signal that a user's credits are still
		// locked and an operator has to look. The task is untouched.
		slog.Error("stalled task settlement convergence failed",
			"workspace_id", failure.Task.WorkspaceID,
			"task_id", failure.Task.TaskID,
			"err", failure.Err)
	}
	if report.Converged > 0 || len(report.Failures) > 0 || report.Skipped > 0 {
		slog.Info("stalled task settlement sweep completed",
			"scanned", report.Scanned, "converged", report.Converged,
			"skipped", report.Skipped, "failures", len(report.Failures),
			"stalled_after", workspacetasks.StalledSettlementThreshold)
	}
	return err
}

func (w *TaskSettlementSweepWorker) nowTime() time.Time {
	if w != nil && w.now != nil {
		return w.now()
	}
	return time.Now()
}

type taskSettlementSweepSchedule struct{}

func (taskSettlementSweepSchedule) Next(current time.Time) time.Time {
	return current.Add(taskSettlementSweepPeriod)
}

// taskSettlementSweepInsert is the periodic job constructor. It is a named
// seam rather than an inline closure so the wiring (queue name, retry budget,
// priority) stays assertable: river.PeriodicJob keeps its constructor and
// schedule in unexported fields, so a closure would be untestable and the
// matrix constants would only be pinned indirectly.
func taskSettlementSweepInsert() (river.JobArgs, *river.InsertOpts) {
	return taskSettlementSweepArgs{}, &river.InsertOpts{
		Queue:       taskqueue.RiverQueueTaskSettlementSweep,
		MaxAttempts: taskqueue.RiverMaxAttemptsTaskSettlementSweep,
		Priority:    4,
	}
}

// TaskSettlementSweepPeriodicJobs returns the sweep's periodic job.
//
// RunOnStart is false: the sweep converges holds abandoned a full day ago, so
// there is nothing to do at boot, and the next run is at most one period away.
func TaskSettlementSweepPeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(
		taskSettlementSweepSchedule{},
		taskSettlementSweepInsert,
		&river.PeriodicJobOpts{ID: "task-settlement-sweep", RunOnStart: false},
	)}
}
