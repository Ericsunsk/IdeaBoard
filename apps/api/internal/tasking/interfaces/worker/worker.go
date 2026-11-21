package worker

// Workspace task worker (#825, Phase 4b-01 of #809).
//
// This file is the REAL consumer for the River kind `workspace_task_execute`
// (args: WorkspaceTaskArgs in the workspacetasks kernel). Before #825 the
// enqueue side (#821) inserted jobs into `drawing_default` with NO registered
// worker, so jobs waited and never dispatched to a provider.
//
// Work runs one River job = one attempt through three phases with clear
// ownership (issue #860):
//
//	claim      (claim.go) — load, drop dirty work (unknown
//	           id / terminal / cancelled / stale token), validate workflow
//	           input (deterministic Fail), resolve media refs read-only via
//	           the TaskAssets adapter (deterministic Fail), resolve the
//	           pinned RoutingSnapshot (missing/corrupt fails closed
//	           deterministically: hold released, no retry burn, the live
//	           pool is never consulted), settle the send's model + retry
//	           layer, then ClaimTaskWithAttempt: pending -> running WITH
//	           the send's attempt fact, in ONE statement (the lease token
//	           IS the attempt's execution token).
//	execution  (execution.go) — Generate through the provider
//	           seam against the snapshot's current model (injected;
//	           production bridges the sole Worker executor, never
//	           shapes provider bodies); on a retryable failure with an
//	           unspent retry candidate: ONE bounded model fallback in the
//	           same claim (model_fallback attempt fact first, then the
//	           second send; no second charge, no duplicate task).
//	retry+settlement (settle.go) — terminal
//	           Complete / Fail via workspacetasks.Service (single-tx
//	           Capture/Release settlement, #826: state flip + outputs +
//	           billing commit atomically, billing failure reverts the
//	           flip); transient provider failures consume the retry budget
//	           via Service.Retry (fresh token + new job); anything else
//	           terminal-fails.
//
// Every send opens/closes a durable attempt fact (retry_layer +
// gateway_retry_count); the primary send's row is written by the claim
// statement itself, and gateway request retries stay inside the gateway
// and never look like model fallback.
//
// Settlement note: the worker shares ONE workspaceTaskHolder with the HTTP
// submit path (built once in task_wiring.go). The holder maps
// hold_transaction_id -> userID process-locally at Hold time, so a second
// holder instance would fail every Capture/Release with "unknown hold".
//
// Provider-creds boundary (deliberate): the production Generate bridge
// consumes the canonical execution plan and calls
// providers.Executor.CallWorkerAPI (transport/auth/retries stay inside
// the Worker). Missing Worker configuration fails the task
// deterministically with a clear message. The execution model is the
// snapshot's pinned selection, stamped onto the transient claimed copy
// by Work: the bridge consumes only that field and never performs a
// live pool or mapping lookup. Result materialization
// into workspace media assets runs inside the executor finalize seam
// (workspace result materializer) and through the WorkspaceTaskRenderer:
// once a provider (real or fake) yields output refs, the renderer
// verifies asset refs (or ingests remote refs) and settlement persists
// the refs atomically.
// The worker never dials providers directly and never shapes provider bodies.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/platform/riverpolicy"
	"ideaboard/api/internal/platform/telemetry"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskports "ideaboard/api/internal/tasking/ports/tasks"

	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel/attribute"
)

// WorkspaceTaskWorkerStore is the narrow persistence seam the worker needs:
// one read plus the kernel lease-fence claim. It embeds TaskClaimStore so
// the claim contract is owned once by the tasks kernel; GetTask is
// redeclared (not the full TaskReadStore) because the worker must not
// observe List/Count. *taskpostgres.Store and
// *taskmemory.InMemoryStore both satisfy it.
type WorkspaceTaskWorkerStore interface {
	GetTask(ctx context.Context, workspaceID, taskID string) (*tasks.WorkspaceTask, error)
	taskports.TaskClaimStore
}

// WorkspaceTaskAttemptClaimStore is the merged claim seam (DESIGN.md
// contract 4): the pending -> running lease flip and the durable
// task_execution_attempts row for the send that follows commit in ONE
// statement, so no running lease can exist without the attempt fact that
// explains it.
//
// The attempt row's execution_token is the lease token the claim fences with,
// which is why start.ExecutionToken is ignored: one token per execution, not
// the two the split claim/StartAttempt pair used to mint.
//
// started is false when a River redelivery re-acquires a lease that already
// owns an attempt row; that is the SAME send resuming, not a second attempt,
// and the caller keeps the existing attempt fact.
type WorkspaceTaskAttemptClaimStore interface {
	ClaimTaskWithAttempt(ctx context.Context, workspaceID, taskID, leaseToken string, start tasks.AttemptStart) (*tasks.WorkspaceTask, bool, error)
}

// WorkspaceTaskWorkerSettler is the terminal settlement seam. The worker
// settles ONLY through it so billing Capture/Release stays compensating and
// centralized. *taskapp.Service satisfies it.
type WorkspaceTaskWorkerSettler interface {
	Complete(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error)
	Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error)
	Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*tasks.WorkspaceTask, error)
}

// WorkspaceTaskProvider generates persisted output refs for one claimed task.
// Production bridges the sole Worker executor (see
// NewWorkspaceWorkerProvider); tests inject deterministic fakes. A nil
// provider fails closed: tasks converge to failed, never strand.
// executionCtx bounds sends; ctx retains the parent budget and cancellation
// for materializing accepted outputs after the execution deadline.
type WorkspaceTaskProvider interface {
	Generate(ctx context.Context, executionCtx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error)
}

// WorkspaceTaskGenerateFunc adapts a function to WorkspaceTaskProvider.
type WorkspaceTaskGenerateFunc func(ctx context.Context, executionCtx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error)

// Generate implements WorkspaceTaskProvider.
func (f WorkspaceTaskGenerateFunc) Generate(ctx context.Context, executionCtx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
	return f(ctx, executionCtx, task)
}

// WorkspaceTaskWorkerDeps wires the worker. Store and Settler are required;
// Assets may be nil only when no task carries media inputs (any input ref
// with a nil reader fails the task deterministically); Provider nil fails
// every task closed with a clear message. Renderer is optional: nil keeps
// the previous passthrough (provider refs settle as-is); set, remote
// results are ingested into workspace media assets (#827) before the
// atomic Complete. LibrarySaver is optional: nil keeps completion bare;
// set, settled outputs are saved in library Quick Saves
// AFTER the atomic Complete, best-effort (failures logged, task stays
// complete, River never retries a completed task). Attempts is optional:
// nil disables durable attempt facts (unit paths); production always
// wires the postgres store so retry_layer/gateway_retry_count facts
// distinguish the gateway, model_fallback, and river_task layers (#848).
// Metrics is optional: nil disables model-fallback observations (unit
// paths); production wires the RED set so bounded fallback sends land in
// ideaboard_model_fallback_total (#852).
type WorkspaceTaskWorkerDeps struct {
	Store        WorkspaceTaskWorkerStore
	Settler      WorkspaceTaskWorkerSettler
	Assets       taskports.AssetReader
	Provider     WorkspaceTaskProvider
	Renderer     taskruntime.WorkspaceTaskRenderer
	LibrarySaver taskruntime.WorkspaceTaskAutoSaver
	Attempts     taskports.AttemptRecorder
	Metrics      ModelFallbackObserver
}

// ModelFallbackObserver receives one observation per completed
// model-fallback send (nil error means the fallback send succeeded).
// The production implementation is the RED metrics set; the narrow
// interface keeps the tasking worker free of the runtime metrics
// package. Implementations must be nil-receiver safe or nil.
type ModelFallbackObserver interface {
	IncModelFallback(service string, err error)
}

// WorkspaceTaskWorker consumes WorkspaceTaskArgs jobs. It rides the existing
// drawing_default queue and worker concurrency (no new queues, pools, or
// periodic jobs); Timeout pins the River drawing budget.
type WorkspaceTaskWorker struct {
	river.WorkerDefaults[taskqueue.WorkspaceTaskArgs]
	store        WorkspaceTaskWorkerStore
	settler      WorkspaceTaskWorkerSettler
	assets       taskports.AssetReader
	provider     WorkspaceTaskProvider
	renderer     taskruntime.WorkspaceTaskRenderer
	librarySaver taskruntime.WorkspaceTaskAutoSaver
	attempts     taskports.AttemptRecorder
	metrics      ModelFallbackObserver
	claims       WorkspaceTaskAttemptClaimStore
}

var _ river.Worker[taskqueue.WorkspaceTaskArgs] = (*WorkspaceTaskWorker)(nil)

// NewWorkspaceTaskWorker builds the consumer. Nil store/settler fail fast:
// a worker that cannot settle must never claim. A store that cannot claim
// through the MERGED statement fails fast too: falling back to the split
// claim-then-record path would reintroduce the "token without an attempt row"
// intermediate state the contract forbids, so a partial store is a
// configuration error, not a degraded mode.
func NewWorkspaceTaskWorker(deps WorkspaceTaskWorkerDeps) (*WorkspaceTaskWorker, error) {
	if deps.Store == nil {
		return nil, errors.New("workspace task worker: store is required")
	}
	if deps.Settler == nil {
		return nil, errors.New("workspace task worker: settler is required")
	}
	claims, ok := deps.Store.(WorkspaceTaskAttemptClaimStore)
	if !ok {
		return nil, errors.New("workspace task worker: store cannot claim with an attempt")
	}
	return &WorkspaceTaskWorker{
		store: deps.Store, settler: deps.Settler,
		assets: deps.Assets, provider: deps.Provider, renderer: deps.Renderer,
		librarySaver: deps.LibrarySaver, attempts: deps.Attempts,
		metrics: deps.Metrics, claims: claims,
	}, nil
}

// Timeout pins the outer River job budget. Provider sends share the shorter
// execution context created by Work, leaving the parent alive for settlement.
func (w *WorkspaceTaskWorker) Timeout(*river.Job[taskqueue.WorkspaceTaskArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutDrawing
}

// Work runs one attempt of the claim -> process -> complete loop. It returns
// nil once the task converges (completed/failed/requeued/dropped) and a
// non-nil error ONLY for transient infra failures where River backoff and
// redelivery is the correct recovery.
func (w *WorkspaceTaskWorker) Work(ctx context.Context, job *river.Job[taskqueue.WorkspaceTaskArgs]) error {
	if w == nil || w.store == nil || w.settler == nil {
		return errors.New("workspace task worker is not configured")
	}
	if job == nil {
		return errors.New("workspace task job is nil")
	}
	args := job.Args
	attemptNumber := 0
	if job.JobRow != nil {
		attemptNumber = job.Attempt
	}
	ctx, workerSpan := telemetry.StartLinked(ctx, args.TraceParent, "generation.worker.handoff",
		attribute.String("service", args.Service), attribute.String("queue", "drawing_default"),
		attribute.Int("attempt_number", attemptNumber))
	defer telemetry.End(workerSpan, nil)
	taskID := strings.TrimSpace(args.TaskID)
	workspaceID := strings.TrimSpace(args.WorkspaceID)
	if taskID == "" || workspaceID == "" {
		slog.Warn("workspace task job missing identity; dropping", "task_id", taskID, "workspace_id", workspaceID)
		return nil
	}

	task, err := w.loadDispatchableTask(ctx, args)
	if err != nil {
		return err
	}
	if task == nil {
		// Dirty work (unknown id / terminal / cancelled / stale token)
		// drops without touching the task or the provider.
		return nil
	}

	if err := w.validateInput(ctx, task); err != nil {
		return w.failTerminal(ctx, task, err.Error())
	}
	if err := w.resolveMediaRefs(ctx, task); err != nil {
		return w.failTerminal(ctx, task, err.Error())
	}
	// Snapshot-only execution (#848, ADR-0012 rule 9): the pinned
	// submit-time snapshot is the sole routing fact. Missing or corrupt
	// snapshots fail closed here, before claiming, so the hold is
	// released with no retry burn and the live pool is never consulted.
	snap, err := resolveExecutionSnapshot(task)
	if err != nil {
		return w.failTerminal(ctx, task, err.Error())
	}

	// The attempt fact is written BY the claim statement, so the send's
	// model and retry layer must be settled BEFORE claiming. Both only
	// need the task id, which loadDispatchableTask already resolved. The
	// execution's Worker target comes from the verified snapshot alone: a
	// redelivery that already consumed the fallback runs the retry
	// candidate directly and never falls back again.
	tracker := newWorkspaceSendTracker(w.attempts, task.ID, workspaceRiverJobID(job), snap.Service)
	model, allowFallback := catalog.SelectExecutionModel(snap, tracker.fallbackConsumed(ctx))
	layer := workspaceAttemptLayer(int32(task.RetryCount), attemptNumber)

	claimed, send, err := w.claim(ctx, args, task, tracker, model, layer)
	if err != nil || claimed == nil {
		return err
	}

	// Stamp the snapshot-derived target onto the transient claimed copy
	// (never persisted): the provider bridge consumes only this field,
	// so stamping is what keeps execution snapshot-only. The legacy
	// envelope value is ignored even when set.
	claimed.ResolvedModel = model
	ctx = taskruntime.WithResultIngestionScope(ctx, claimed)
	// The execution budget bounds every provider send in this claim
	// (primary + at most one bounded fallback) without touching settlement:
	// an over-budget send without accepted outputs is retryable. Materialization,
	// Complete, Fail, and Retry retain the parent (River) context.
	execCtx, cancel := context.WithTimeout(ctx, riverpolicy.WorkspaceExecutionBudget())
	defer cancel()
	outputs, genErr := w.generate(ctx, execCtx, claimed, send.layer, send.attemptNo)
	if genErr == nil {
		return w.completeExecution(ctx, send, claimed, outputs, model)
	}

	failure := routing.ClassifyError(genErr)
	if failure.Retryable() && allowFallback {
		if result, done := w.executeModelFallback(ctx, execCtx, tracker, send, snap, claimed, model, genErr); done {
			return result
		}
		// The fallback row could not be persisted: at-most-once cannot
		// be proven, so no second send. Converge the original failure
		// through the river/terminal policy below.
	}
	return w.convergeSendFailure(ctx, send, claimed, failure, genErr, false)
}

// workspaceRiverJobID extracts the River job id for attempt facts.
// Synthetic jobs in unit tests carry no JobRow: they record 0, which
// the column accepts (river_job_id >= 0).
func workspaceRiverJobID(job *river.Job[taskqueue.WorkspaceTaskArgs]) int64 {
	if job == nil || job.JobRow == nil {
		return 0
	}
	return job.ID
}
