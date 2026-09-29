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
//	claim      (workspace_task_claim.go) — load, drop dirty work (unknown
//	           id / terminal / cancelled / stale token), validate workflow
//	           input (deterministic Fail), resolve media refs read-only via
//	           the TaskAssets adapter (deterministic Fail), resolve the
//	           pinned RoutingSnapshot (missing/corrupt fails closed
//	           deterministically: hold released, no retry burn, the live
//	           pool is never consulted), then ClaimTask with the lease
//	           token (pending -> running).
//	execution  (workspace_task_execution.go) — Generate through the provider
//	           seam against the snapshot's current model (injected;
//	           production bridges the EXISTING gateway executor, never
//	           reimplements dialects); on a retryable failure with an
//	           unspent retry candidate: ONE bounded model fallback in the
//	           same claim (model_fallback attempt fact first, then the
//	           second send; no second charge, no duplicate task).
//	retry+settlement (workspace_task_retry_settlement.go) — terminal
//	           Complete / Fail via workspacetasks.Service (single-tx
//	           Capture/Release settlement, #826: state flip + outputs +
//	           billing commit atomically, billing failure reverts the
//	           flip); transient provider failures consume the retry budget
//	           via Service.Retry (fresh token + new job); anything else
//	           terminal-fails.
//
// Every send opens/closes a durable attempt fact (retry_layer +
// gateway_retry_count); gateway request retries stay inside the gateway
// and never look like model fallback.
//
// Settlement note: the worker shares ONE workspaceTaskHolder with the HTTP
// submit path (built once in task_wiring.go). The holder maps
// hold_transaction_id -> userID process-locally at Hold time, so a second
// holder instance would fail every Capture/Release with "unknown hold".
//
// Provider-creds boundary (deliberate): the production Generate bridge
// consumes the canonical execution plan and calls
// providers.Executor.CallGatewayAPI (transport/auth/retries stay inside
// the gateway). Missing gateway credentials fail the task
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

	"ideaboard/api/internal/platform/telemetry"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel/attribute"
)

// WorkspaceTaskWorkerStore is the narrow persistence seam the worker needs:
// lease-guarded claim plus reads. *taskpostgres.Store satisfies it; the
// in-memory sibling satisfies it once ClaimTask lands there (test backend).
type WorkspaceTaskWorkerStore interface {
	GetTask(ctx context.Context, workspaceID, taskID string) (*workspacetasks.WorkspaceTask, error)
	ClaimTask(ctx context.Context, workspaceID, taskID, leaseToken string) (*workspacetasks.WorkspaceTask, error)
}

// WorkspaceTaskWorkerSettler is the terminal settlement seam. The worker
// settles ONLY through it so billing Capture/Release stays compensating and
// centralized. *workspacetasks.Service satisfies it.
type WorkspaceTaskWorkerSettler interface {
	Complete(ctx context.Context, workspaceID, taskID string, outputs []workspacetasks.OutputRef) (*workspacetasks.WorkspaceTask, error)
	Fail(ctx context.Context, workspaceID, taskID, errorMsg string) (*workspacetasks.WorkspaceTask, error)
	Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*workspacetasks.WorkspaceTask, error)
}

// WorkspaceTaskProvider generates persisted output refs for one claimed task.
// Production bridges the existing gateway executor (see
// NewWorkspaceGatewayProvider); tests inject deterministic fakes. A nil
// provider fails closed: tasks converge to failed, never strand.
type WorkspaceTaskProvider interface {
	Generate(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error)
}

// WorkspaceTaskGenerateFunc adapts a function to WorkspaceTaskProvider.
type WorkspaceTaskGenerateFunc func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error)

// Generate implements WorkspaceTaskProvider.
func (f WorkspaceTaskGenerateFunc) Generate(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
	return f(ctx, task)
}

// WorkspaceTaskWorkerDeps wires the worker. Store and Settler are required;
// Assets may be nil only when no task carries media inputs (any input ref
// with a nil reader fails the task deterministically); Provider nil fails
// every task closed with a clear message. Renderer is optional: nil keeps
// the previous passthrough (provider refs settle as-is); set, remote
// results are ingested into workspace media assets (#827) before the
// atomic Complete. Materializer is optional: nil keeps completion bare;
// set, settled outputs auto-materialize into library Quick Saves (#828)
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
	Assets       workspacetasks.AssetReader
	Provider     WorkspaceTaskProvider
	Renderer     taskruntime.WorkspaceTaskRenderer
	Materializer taskruntime.WorkspaceTaskAutoMaterializer
	Attempts     workspacetasks.AttemptRecorder
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
// periodic jobs); Timeout reuses the drawing execution budget.
type WorkspaceTaskWorker struct {
	river.WorkerDefaults[workspacetasks.WorkspaceTaskArgs]
	store        WorkspaceTaskWorkerStore
	settler      WorkspaceTaskWorkerSettler
	assets       workspacetasks.AssetReader
	provider     WorkspaceTaskProvider
	renderer     taskruntime.WorkspaceTaskRenderer
	materializer taskruntime.WorkspaceTaskAutoMaterializer
	attempts     workspacetasks.AttemptRecorder
	metrics      ModelFallbackObserver
}

var _ river.Worker[workspacetasks.WorkspaceTaskArgs] = (*WorkspaceTaskWorker)(nil)

// NewWorkspaceTaskWorker builds the consumer. Nil store/settler fail fast:
// a worker that cannot settle must never claim.
func NewWorkspaceTaskWorker(deps WorkspaceTaskWorkerDeps) (*WorkspaceTaskWorker, error) {
	if deps.Store == nil {
		return nil, errors.New("workspace task worker: store is required")
	}
	if deps.Settler == nil {
		return nil, errors.New("workspace task worker: settler is required")
	}
	return &WorkspaceTaskWorker{
		store: deps.Store, settler: deps.Settler,
		assets: deps.Assets, provider: deps.Provider, renderer: deps.Renderer,
		materializer: deps.Materializer, attempts: deps.Attempts,
		metrics: deps.Metrics,
	}, nil
}

// Timeout bounds one workspace execution with the existing drawing budget.
func (w *WorkspaceTaskWorker) Timeout(*river.Job[workspacetasks.WorkspaceTaskArgs]) time.Duration {
	return taskqueue.RiverJobTimeoutDrawing
}

// Work runs one attempt of the claim -> process -> complete loop. It returns
// nil once the task converges (completed/failed/requeued/dropped) and a
// non-nil error ONLY for transient infra failures where River backoff and
// redelivery is the correct recovery.
func (w *WorkspaceTaskWorker) Work(ctx context.Context, job *river.Job[workspacetasks.WorkspaceTaskArgs]) error {
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

	claimed, err := w.claim(ctx, args, task)
	if err != nil || claimed == nil {
		return err
	}

	// The execution's gateway target comes from the verified snapshot
	// alone. A redelivery that already consumed the fallback runs the
	// retry candidate directly and never falls back again.
	tracker := newWorkspaceSendTracker(w.attempts, claimed.ID, workspaceRiverJobID(job), snap.Service)
	layer := workspacetasks.AttemptRetryLayerGateway
	if claimed.RetryCount > 0 {
		layer = workspacetasks.AttemptRetryLayerRiverTask
	}
	model, allowFallback := selectExecutionModel(snap, tracker.fallbackConsumed(ctx))

	send := tracker.startSend(ctx, model, layer)
	// Stamp the snapshot-derived target onto the transient claimed copy
	// (never persisted): the provider bridge consumes only this field,
	// so stamping is what keeps execution snapshot-only. The legacy
	// envelope value is ignored even when set.
	claimed.ResolvedModel = model
	outputs, genErr := w.generate(ctx, claimed, send.layer, send.attemptNo)
	if genErr == nil {
		return w.completeExecution(ctx, send, claimed, outputs, model)
	}

	failure := workspaceAttemptFailure(genErr)
	if failure.Retryable() && allowFallback {
		if result, done := w.executeModelFallback(ctx, tracker, send, snap, claimed, model, genErr); done {
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
func workspaceRiverJobID(job *river.Job[workspacetasks.WorkspaceTaskArgs]) int64 {
	if job == nil || job.JobRow == nil {
		return 0
	}
	return job.JobRow.ID
}
