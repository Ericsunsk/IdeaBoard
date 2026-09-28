package worker

// Workspace task worker (#825, Phase 4b-01 of #809).
//
// This file is the REAL consumer for the River kind `workspace_task_execute`
// (args: WorkspaceTaskArgs in workspace_task_adapters.go). Before #825 the
// enqueue side (#821) inserted jobs into `drawing_default` with NO registered
// worker, so jobs waited and never dispatched to a provider.
//
// Loop (one River job = one attempt):
//
//	load -> drop dirty work (unknown id / terminal / cancelled / stale token)
//	-> validate workflow input (deterministic Fail)
//	-> resolve media refs read-only via the TaskAssets adapter (deterministic Fail)
//	-> ClaimTask with the lease token (pending -> running)
//	-> Generate through the provider seam (injected; production bridges the
//	   EXISTING gateway executor, never reimplements dialects)
//	-> terminal Complete / Fail via workspacetasks.Service (single-tx
//	   Capture/Release settlement, #826: state flip + outputs + billing
//	   commit atomically, billing failure reverts the flip)
//	-> transient provider failures consume the retry budget via Service.Retry
//	   (fresh token + new job); anything else terminal-fails.
//
// Settlement note: the worker shares ONE workspaceTaskHolder with the HTTP
// submit path (built once in task_wiring.go). The holder maps
// hold_transaction_id -> userID process-locally at Hold time, so a second
// holder instance would fail every Capture/Release with "unknown hold".
//
// Provider-creds boundary (deliberate): the production Generate bridge
// consumes the durable execution plan minted at submit (canonical params
// plus the submit-time resolved model) and calls
// providers.Executor.CallGatewayAPI (transport/auth/retries stay inside
// the gateway). Missing gateway credentials or model mappings fail the
// task deterministically with a clear message. Result materialization
// into workspace media assets runs inside the executor finalize seam
// (workspace result materializer) and through the WorkspaceTaskRenderer
// below: once a provider (real or fake) yields output refs, the renderer
// verifies asset refs (or ingests remote refs) and settlement persists
// the refs atomically.
// The worker never dials providers directly and never shapes provider bodies.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/workflow/paramkeys"

	"github.com/riverqueue/river"
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
// complete, River never retries a completed task).
type WorkspaceTaskWorkerDeps struct {
	Store        WorkspaceTaskWorkerStore
	Settler      WorkspaceTaskWorkerSettler
	Assets       workspacetasks.AssetReader
	Provider     WorkspaceTaskProvider
	Renderer     taskruntime.WorkspaceTaskRenderer
	Materializer taskruntime.WorkspaceTaskAutoMaterializer
}

// WorkspaceTaskWorker consumes WorkspaceTaskArgs jobs. It rides the existing
// drawing_default queue and worker concurrency (no new queues, pools, or
// periodic jobs); Timeout reuses the drawing execution budget.
type WorkspaceTaskWorker struct {
	river.WorkerDefaults[taskruntime.WorkspaceTaskArgs]
	store        WorkspaceTaskWorkerStore
	settler      WorkspaceTaskWorkerSettler
	assets       workspacetasks.AssetReader
	provider     WorkspaceTaskProvider
	renderer     taskruntime.WorkspaceTaskRenderer
	materializer taskruntime.WorkspaceTaskAutoMaterializer
}

var _ river.Worker[taskruntime.WorkspaceTaskArgs] = (*WorkspaceTaskWorker)(nil)

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
		materializer: deps.Materializer,
	}, nil
}

// Timeout bounds one workspace execution with the existing drawing budget.
func (w *WorkspaceTaskWorker) Timeout(*river.Job[taskruntime.WorkspaceTaskArgs]) time.Duration {
	return taskqueue.RiverJobTimeoutDrawing
}

// Work runs one attempt of the claim -> process -> complete loop. It returns
// nil once the task converges (completed/failed/requeued/dropped) and a
// non-nil error ONLY for transient infra failures where River backoff and
// redelivery is the correct recovery.
func (w *WorkspaceTaskWorker) Work(ctx context.Context, job *river.Job[taskruntime.WorkspaceTaskArgs]) error {
	if w == nil || w.store == nil || w.settler == nil {
		return errors.New("workspace task worker is not configured")
	}
	if job == nil {
		return errors.New("workspace task job is nil")
	}
	args := job.Args
	taskID := strings.TrimSpace(args.TaskID)
	workspaceID := strings.TrimSpace(args.WorkspaceID)
	if taskID == "" || workspaceID == "" {
		slog.Warn("workspace task job missing identity; dropping", "task_id", taskID, "workspace_id", workspaceID)
		return nil
	}

	task, err := w.store.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		if errors.Is(err, workspacetasks.ErrNotFound) {
			slog.Info("workspace task gone (rollback delete?); dropping job", "task_id", taskID)
			return nil
		}
		return fmt.Errorf("workspace task load: %w", err)
	}
	if task.IsTerminal() {
		if task.Status == workspacetasks.StatusCompleted {
			// Complete commits before auto-materialization. A redelivery in
			// this gap replays the idempotent trigger and repairs the link.
			w.materializeCompletedTask(ctx, task)
		}
		slog.Info("workspace task already terminal; dropping dirty job", "task_id", taskID, "status", task.Status)
		return nil
	}
	if task.Status == workspacetasks.StatusCancelled {
		slog.Info("workspace task cancelled; dropping job", "task_id", taskID)
		return nil
	}
	if token := strings.TrimSpace(args.ExecutionToken); token != "" && task.ExecutionToken != token {
		slog.Info("workspace task job carries a stale execution token; dropping",
			"task_id", taskID, "job_token", redactToken(token), "task_token", redactToken(task.ExecutionToken))
		return nil
	}

	if err := w.validateInput(ctx, task); err != nil {
		return w.failTerminal(ctx, task, err.Error())
	}
	if err := w.resolveMediaRefs(ctx, task); err != nil {
		return w.failTerminal(ctx, task, err.Error())
	}

	claimed, err := w.claim(ctx, args, task)
	if err != nil || claimed == nil {
		return err
	}

	outputs, genErr := w.generate(ctx, claimed)
	if genErr == nil {
		if w.renderer != nil {
			rendered, renderErr := w.renderer.RenderOutputs(ctx, claimed, outputs)
			if renderErr != nil {
				return w.failTerminal(ctx, claimed, workspaceFailureMessage(renderErr))
			}
			outputs = rendered
		}
		completed, err := w.settler.Complete(ctx, workspaceID, taskID, outputs)
		if err != nil {
			if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
				slog.Info("workspace task converged during completion; dropping", "task_id", taskID, "err", err)
				return nil
			}
			return fmt.Errorf("workspace task complete: %w", err)
		}
		// #828: auto-materialize AFTER the atomic Complete commit. The
		// task is complete regardless of the outcome; trigger failures
		// are logged and never retried through River.
		w.materializeCompletedTask(ctx, completed)
		slog.Info("workspace task completed", "task_id", taskID, "outputs", len(outputs))
		return nil
	}

	retryable := isWorkspaceProviderRetryable(genErr)
	if workspacetasks.ShouldRetry(claimed.RetryCount, retryable) {
		if _, err := w.settler.Retry(ctx, workspaceID, taskID, retryable); err != nil {
			if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
				slog.Info("workspace task converged during retry; dropping", "task_id", taskID, "err", err)
				return nil
			}
			return fmt.Errorf("workspace task retry: %w", err)
		}
		slog.Info("workspace task requeued after transient provider failure",
			"task_id", taskID, "retry_count", claimed.RetryCount, "err", genErr)
		return nil
	}
	return w.failTerminal(ctx, claimed, workspaceFailureMessage(genErr))
}

// validateInput enforces the discriminated input union before claiming, so
// malformed tasks fail from pending without lease churn.
func (w *WorkspaceTaskWorker) validateInput(_ context.Context, task *workspacetasks.WorkspaceTask) error {
	if _, err := workspacetasks.ValidateCreate(task.WorkflowKind, task.MediaAssetIDs, task.Params); err != nil {
		return fmt.Errorf(msgValidateInput, err)
	}
	return nil
}

// resolveMediaRefs checks every media_asset_ids entry read-only through the
// TaskAssets adapter. Invisible/archived/non-ready assets fail the task
// deterministically; the worker never mutates media.
func (w *WorkspaceTaskWorker) resolveMediaRefs(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if len(task.MediaAssetIDs) == 0 && task.Params == nil {
		return nil
	}
	if w.assets == nil && (len(task.MediaAssetIDs) > 0 || hasWorkspaceMaskParam(task)) {
		return errors.New(msgResolveMediaRefs)
	}
	for i, id := range task.MediaAssetIDs {
		if _, err := w.assets.GetAsset(ctx, task.WorkspaceID, id); err != nil {
			switch {
			case errors.Is(err, workspacetasks.ErrMediaArchived):
				return fmt.Errorf(msgResolveMediaRefs2, i, err)
			case errors.Is(err, workspacetasks.ErrMediaNotReady):
				return fmt.Errorf(msgResolveMediaRefs3, i, err)
			default:
				return fmt.Errorf(msgResolveMediaRefs4, i, workspacetasks.ErrMediaNotFound)
			}
		}
	}
	// The v1 mask reference (params.mask_asset_id) resolves through the
	// same read-only gate so a mask that vanished between submit and
	// dispatch fails deterministically instead of reaching the provider.
	if maskID, ok := workspacetasks.MaskAssetIDFromParams(task.Params); ok {
		if w.assets == nil {
			return errors.New(msgResolveMediaRefs5)
		}
		if _, err := w.assets.GetAsset(ctx, task.WorkspaceID, maskID); err != nil {
			switch {
			case errors.Is(err, workspacetasks.ErrMediaArchived):
				return fmt.Errorf(msgResolveMediaRefs6, err)
			case errors.Is(err, workspacetasks.ErrMediaNotReady):
				return fmt.Errorf(msgResolveMediaRefs7, err)
			default:
				return fmt.Errorf(msgResolveMediaRefs8, workspacetasks.ErrMediaNotFound)
			}
		}
	}
	return nil
}

func hasWorkspaceMaskParam(task *workspacetasks.WorkspaceTask) bool {
	if task == nil {
		return false
	}
	_, ok := workspacetasks.MaskAssetIDFromParams(task.Params)
	return ok
}

// claim fences pending -> running with the lease token. Lease-guarded
// conflicts reconverge: terminal/cancelled rows drop the job, anything else
// is transient and rides River backoff.
func (w *WorkspaceTaskWorker) claim(ctx context.Context, args taskruntime.WorkspaceTaskArgs, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	lease := strings.TrimSpace(args.ExecutionToken)
	if lease == "" {
		fresh, err := workspacetasks.NewExecutionToken()
		if err != nil {
			return nil, fmt.Errorf("workspace task mint lease: %w", err)
		}
		lease = fresh
	}
	claimed, err := w.store.ClaimTask(ctx, task.WorkspaceID, task.ID, lease)
	if err != nil {
		if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
			current, getErr := w.store.GetTask(ctx, task.WorkspaceID, task.ID)
			if getErr != nil {
				if errors.Is(getErr, workspacetasks.ErrNotFound) {
					return nil, nil
				}
				return nil, fmt.Errorf("workspace task reload after claim conflict: %w", getErr)
			}
			if current.IsTerminal() || current.Status == workspacetasks.StatusCancelled {
				slog.Info("workspace task converged before claim; dropping", "task_id", task.ID, "status", current.Status)
				return nil, nil
			}
			return nil, fmt.Errorf("workspace task claim conflict: %w", err)
		}
		return nil, fmt.Errorf("workspace task claim: %w", err)
	}
	return claimed, nil
}

// generate runs the provider seam. A nil provider fails closed with a clear
// message (tasks converge to failed, never strand).
func (w *WorkspaceTaskWorker) generate(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
	if w.provider == nil {
		return nil, errors.New(msgGenerate)
	}
	outputs, err := w.provider.Generate(ctx, task)
	if err != nil {
		return nil, err
	}
	for _, ref := range outputs {
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, fmt.Errorf(msgGenerate2, err)
		}
	}
	if len(outputs) > workspacetasks.MaxOutputsPerTask {
		return nil, fmt.Errorf(msgGenerate3, workspacetasks.MaxOutputsPerTask)
	}
	return outputs, nil
}

// materializeCompletedTask runs the #828 post-commit trigger. The task is
// already complete: failures are warn-logged and swallowed so a completed
// task never requeues through River.
func (w *WorkspaceTaskWorker) materializeCompletedTask(ctx context.Context, completed *workspacetasks.WorkspaceTask) {
	if w.materializer == nil || completed == nil {
		return
	}
	if err := w.materializer.MaterializeCompletedTask(ctx, completed); err != nil {
		slog.Warn("workspace task auto-materialize failed; task stays complete",
			"task_id", completed.ID, "err", err)
	}
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

// workspaceHTTPStatusCoder extracts an HTTP status from provider errors
// without importing the runtime gateway seam: the concrete upstream error
// implements HTTPStatus() int, so structural matching classifies it the
// same way runtimeports.ProviderHTTPStatus did.
type workspaceHTTPStatusCoder interface {
	HTTPStatus() int
}

// isWorkspaceProviderRetryable classifies gateway errors with the
// workspace-domain budget: HTTP 429/5xx/408 and transport timeouts may
// requeue; deterministic failures (4xx, validation, auth, quota) never do.
func isWorkspaceProviderRetryable(err error) bool {
	if err == nil {
		return false
	}
	status := 0
	var coder workspaceHTTPStatusCoder
	if errors.As(err, &coder) {
		status = coder.HTTPStatus()
	}
	return workspacetasks.IsRetryableProviderError(status, err)
}

// workspaceFailureMessage keeps the terminal error row inspectable without
// persisting unbounded provider bodies.
func workspaceFailureMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return msgWorkspaceFailureMessage
	}
	const maxLen = 500
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "...(truncated)"
	}
	return msg
}

func redactToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "***" + token[len(token)-4:]
}

// workspaceGatewayProvider bridges the EXISTING provider gateway seam for
// workspace tasks. It consumes the durable execution plan minted at submit
// (submit-time resolved model), synthesizes
// the legacy TaskPayload the executor requires, and maps provider outputs
// back to workspace OutputRefs. It never shapes provider bodies and never
// dials providers: transport/auth/retries stay inside the gateway client.
//
// Boundary: gateway credentials resolve per execution through the AI
// gateway config seam (system settings + env-held encryption key) and are
// never persisted; a missing gateway configuration or model mapping fails
// the task deterministically at Resolve/factory time. Provider image bytes
// land in workspace media through the workspace result materializer
// (executor finalize seam), so Generate yields workspace asset refs that
// the renderer verifies before the atomic Complete.
// WorkspaceGatewayCallInput carries one gateway execution's resolved
// inputs. It mirrors the runtime gateway seam with tasking-domain types so
// this package never imports the runtime layer: the model comes from the
// durable execution plan, the token fences terminal writes so stale
// workers lose.
type WorkspaceGatewayCallInput struct {
	Task           *tasks.TaskPayload
	Model          string
	ExecutionToken string
}

// WorkspaceGatewayCallOutput is the gateway result subset the workspace
// bridge consumes: provider outputs mapped onto the workspace contract.
type WorkspaceGatewayCallOutput struct {
	Outputs []tasks.TaskOutput
}

// WorkspaceGatewayCaller hides the gateway transport from the workspace
// task worker. The production implementation adapts the runtime provider
// gateway executor (see workspaceGatewayCallerAdapter in
// runtime/composition); tests inject deterministic fakes.
type WorkspaceGatewayCaller interface {
	CallGatewayAPI(ctx context.Context, input WorkspaceGatewayCallInput) (WorkspaceGatewayCallOutput, error)
}

type workspaceGatewayProvider struct {
	caller WorkspaceGatewayCaller
}

// NewWorkspaceGatewayProvider bridges a gateway caller for workspace
// tasks. A nil caller fails closed per execution with a clear message.
func NewWorkspaceGatewayProvider(caller WorkspaceGatewayCaller) WorkspaceTaskProvider {
	return &workspaceGatewayProvider{caller: caller}
}

// Generate implements WorkspaceTaskProvider.
func (p *workspaceGatewayProvider) Generate(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
	if p == nil || p.caller == nil {
		return nil, errors.New(msgGenerate4)
	}
	model := strings.TrimSpace(task.ResolvedModel)
	if model == "" {
		return nil, errors.New(msgGenerate5)
	}
	legacy, err := workspaceBridgeTaskPayload(task)
	if err != nil {
		return nil, err
	}
	// Scope the call for the workspace result materializer (executor
	// passes ctx through to it): the materializer needs the workspace
	// identity that the legacy materialize signature does not carry.
	ctx = taskruntime.WithWorkspaceTaskScope(ctx, strings.TrimSpace(task.WorkspaceID))
	output, err := p.caller.CallGatewayAPI(ctx, WorkspaceGatewayCallInput{
		Task:           legacy,
		Model:          model,
		ExecutionToken: strings.TrimSpace(task.ExecutionToken),
	})
	if err != nil {
		return nil, err
	}
	return mapWorkspaceProviderOutputs(output.Outputs)
}

// workspaceBridgeTaskPayload synthesizes the gateway payload from the
// durable canonical plan. The executor still owns dialect shaping and
// materialization; an absent or invalid canonical plan fails closed.
func workspaceBridgeTaskPayload(task *workspacetasks.WorkspaceTask) (*tasks.TaskPayload, error) {
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return nil, errors.New(msgWorkspaceBridgeTaskPayload)
	}
	raw := strings.TrimSpace(task.CanonicalParams)
	if raw == "" {
		return nil, errors.New(msgWorkspaceBridgeTaskPayload2)
	}
	typed, err := tasks.UnmarshalParams(raw)
	if err != nil {
		return nil, fmt.Errorf(msgWorkspaceBridgeTaskPayload3, err)
	}
	if typed.Extra == nil {
		typed.Extra = make(map[string]interface{})
	}
	typed.Extra[paramkeys.WorkspaceID] = strings.TrimSpace(task.WorkspaceID)
	encoded, err := json.Marshal(&typed)
	if err != nil {
		return nil, fmt.Errorf(msgWorkspaceBridgeTaskPayload4, err)
	}
	return &tasks.TaskPayload{
		TaskID:    strings.TrimSpace(task.ID),
		UserID:    task.OwnerUserID,
		Prompt:    workspaceBridgePrompt(typed),
		Params:    string(encoded),
		CreatedAt: task.CreatedAt.Unix(),
	}, nil
}

// workspaceBridgePrompt resolves the provider prompt from the canonical
// Generation snapshot stamped at submit time.
func workspaceBridgePrompt(typed *tasks.WorkflowTaskParams) string {
	if typed != nil && typed.Generation != nil {
		if prompt := strings.TrimSpace(typed.Generation.CompiledPrompt); prompt != "" {
			return prompt
		}
		if prompt := strings.TrimSpace(typed.Generation.UserPrompt); prompt != "" {
			return prompt
		}
	}
	return ""
}

// mapWorkspaceProviderOutputs projects legacy provider outputs onto the
// workspace output contract. Asset refs pass through by public id; remote
// refs pass through only when they satisfy the explicit remote contract
// (URL + checksum, no storage internals). Anything else is a deterministic
// finalize failure. Remote refs flow into the #827 WorkspaceTaskRenderer,
// which ingests their bytes into workspace media assets before the atomic
// Complete; asset refs are re-verified there.
func mapWorkspaceProviderOutputs(outputs []tasks.TaskOutput) ([]workspacetasks.OutputRef, error) {
	refs := make([]workspacetasks.OutputRef, 0, len(outputs))
	for _, out := range outputs {
		var ref workspacetasks.OutputRef
		switch {
		case strings.TrimSpace(out.AssetID) != "":
			ref = workspacetasks.OutputRef{
				Kind: workspacetasks.OutputKindAsset, AssetID: strings.TrimSpace(out.AssetID),
				Role: strings.TrimSpace(out.Role), ResultIndex: out.ResultIndex,
			}
		case strings.TrimSpace(out.URL) != "" || strings.TrimSpace(out.SourceURL) != "":
			url := firstNonEmptyWorkspaceURL(out.URL, out.SourceURL)
			ref = workspacetasks.OutputRef{
				Kind: workspacetasks.OutputKindRemote, URL: url,
				ChecksumSHA256: strings.TrimSpace(out.ChecksumSHA256),
				Role:           strings.TrimSpace(out.Role), ResultIndex: out.ResultIndex,
			}
		default:
			return nil, errors.New(msgMapWorkspaceProviderOutputs)
		}
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, fmt.Errorf(msgMapWorkspaceProviderOutputs2, err)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func firstNonEmptyWorkspaceURL(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
