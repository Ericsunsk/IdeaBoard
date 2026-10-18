package worker

// Retry+settlement phase (#860): converge one failed send and own every
// terminal write.
//
//   - executeModelFallback runs the single bounded model fallback (#848,
//     ADR-0012 rule 6): exactly one more send to the snapshot's retry
//     candidate, in the same claim, with no second charge (billing captures
//     once at Complete) and no duplicate task. The model_fallback attempt
//     row is inserted BEFORE the second send; a missing row fails the
//     fallback itself so a later redelivery can never spend the budget
//     twice.
//   - convergeSendFailure converges one failed send: transient provider
//     failures consume the retry budget via Service.Retry (fresh token +
//     new job, recorded as a requeued attempt fact); anything else
//     terminal-fails via failTerminal.
//   - failTerminal converges one deterministic failure (hold released, no
//     retry burn). Converged rows (a cancel landing mid-flight) drop the
//     job instead of erroring.
//
// Retry layers stay distinct: provider request retries live inside the
// gateway client, the model fallback is the single same-claim second send
// above, and River/task retries are fresh-token requeues bounded by
// tasks.retry_count. Attempt facts record which layer each send used
// (gateway / model_fallback / river_task); the primary send's row is
// written by the claim statement, keyed by the lease token.
//
// Budget scope (ADR-0010 rule 5): tasks.retry_count is spent ONLY here,
// on a transient PROVIDER failure that requeues. A settlement fault
// after a billed send never reaches this file's paths: it converges
// terminally instead (execution.go), because the only
// recovery a River retry could offer would be another billed call.

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

// executeModelFallback runs the single bounded model fallback (#848,
// ADR-0012 rule 6): exactly one more send to the snapshot's retry
// candidate, in the same claim, with no second charge (billing captures
// once at Complete) and no duplicate task (no enqueue here).
//
// The model_fallback attempt row is inserted BEFORE the second send; a
// missing row fails the fallback itself (ok=false) so a later
// redelivery can never spend the budget twice. A retryable fallback
// failure still rides the river_task requeue budget; a deterministic
// one goes terminal directly, never further.
func (w *WorkspaceTaskWorker) executeModelFallback(
	ctx context.Context,
	tracker *workspaceSendTracker,
	first *workspaceSend,
	snap routing.RoutingSnapshot,
	claimed *workspacetasks.WorkspaceTask,
	model string,
	firstErr error,
) (error, bool) {
	stage, category := workspaceFailureStageCategory(routing.ClassifyError(firstErr))
	first.finishFailed(stage, category, workspaceFailureMessage(firstErr))
	fallback, ok := tracker.startFallbackSend(ctx, snap.RetryCandidate)
	if !ok {
		slog.Warn("workspace task model fallback unavailable; converging without it",
			"task_id", claimed.ID, "model", model)
		return nil, false
	}
	slog.Info("workspace task trying model fallback",
		"task_id", claimed.ID, "from_model", model, "to_model", snap.RetryCandidate)
	claimed.ResolvedModel = snap.RetryCandidate
	outputs, genErr := w.generate(ctx, claimed, fallback.layer, fallback.attemptNo)
	// The fallback observation fires at send completion when the
	// outcome is known (#852): decided-but-unrecorded fallbacks (the
	// ok=false branch above) never inflate the counter. Nil metrics
	// keeps unit paths uninstrumented.
	if w.metrics != nil {
		w.metrics.IncModelFallback(tracker.service, genErr)
	}
	if genErr == nil {
		return w.completeExecution(ctx, fallback, claimed, outputs, snap.RetryCandidate), true
	}
	failure := routing.ClassifyError(genErr)
	stage, category = workspaceFailureStageCategory(failure)
	fallback.finishFailed(stage, category, workspaceFailureMessage(genErr))
	return w.convergeSendFailure(ctx, fallback, claimed, failure, genErr, true), true
}

// convergeSendFailure converges one failed send: transient provider
// failures consume the retry budget via Service.Retry (fresh token +
// new job, recorded as a requeued attempt fact); anything else
// terminal-fails. fallbackUsed only routes logging today; the persisted
// model_fallback row (not this flag) enforces at-most-once across
// redeliveries.
func (w *WorkspaceTaskWorker) convergeSendFailure(
	ctx context.Context,
	send *workspaceSend,
	claimed *workspacetasks.WorkspaceTask,
	failure routing.AttemptFailure,
	genErr error,
	fallbackUsed bool,
) error {
	workspaceID, taskID := claimed.WorkspaceID, claimed.ID
	retryable := failure.Retryable()
	if workspacetasks.ShouldRetry(claimed.RetryCount, retryable) {
		stage, category := workspaceFailureStageCategory(failure)
		send.finishRequeued(stage, category, workspaceFailureMessage(genErr))
		if _, err := w.settler.Retry(ctx, workspaceID, taskID, retryable); err != nil {
			if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
				slog.Info("workspace task converged during retry; dropping", "task_id", taskID, "err", err)
				return nil
			}
			return fmt.Errorf("workspace task retry: %w", err)
		}
		slog.Info("workspace task requeued after transient provider failure",
			"task_id", taskID, "retry_count", claimed.RetryCount,
			"retry_layer", send.layer, "fallback_used", fallbackUsed, "err", genErr)
		return nil
	}
	stage, category := workspaceFailureStageCategory(failure)
	send.finishFailed(stage, category, workspaceFailureMessage(genErr))
	return w.failTerminal(ctx, claimed, workspaceFailureMessage(genErr))
}

// failTerminal converges one deterministic failure. Converged rows (a cancel
// landing mid-flight) drop the job instead of erroring.
func (w *WorkspaceTaskWorker) failTerminal(ctx context.Context, task *workspacetasks.WorkspaceTask, reason string) error {
	if _, err := w.settler.Fail(ctx, task.WorkspaceID, task.ID, reason); err != nil {
		if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
			slog.Info("workspace task converged before failure writeback; dropping", "task_id", task.ID, "err", err)
			return nil
		}
		return fmt.Errorf("workspace task fail: %w", err)
	}
	slog.Warn("workspace task failed", "task_id", task.ID, "reason", reason)
	return nil
}

// workspaceFailureMessage keeps the terminal error row inspectable without
// persisting unbounded provider bodies.
func workspaceFailureMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return errorrender.MsgWorkspaceFailureMessage
	}
	const maxLen = 500
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "...(truncated)"
	}
	return msg
}

// --- stalled-settlement sweep (from task_settlement_sweep.go): scan-driven final convergence ---

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
	return riverpolicy.RiverJobTimeoutTaskSettlementSweep
}

// MaxAttempts pins the sweep's retry budget from the code-owned matrix.
func (*TaskSettlementSweepWorker) MaxAttempts(*river.Job[taskSettlementSweepArgs]) int {
	return riverpolicy.RiverMaxAttemptsTaskSettlementSweep
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
		Queue:       riverpolicy.RiverQueueTaskSettlementSweep,
		MaxAttempts: riverpolicy.RiverMaxAttemptsTaskSettlementSweep,
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
