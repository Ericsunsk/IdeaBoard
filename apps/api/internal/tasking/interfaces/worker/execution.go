package worker

// Execution phase (#860): run one provider send against the
// snapshot-derived model and converge success.
//
// generate runs the provider seam (nil provider fails closed: tasks
// converge to failed, never strand). completeExecution renders (when
// configured) and terminally completes one successful send through the
// retry+settlement phase; renderer failures are deterministic finalize
// failures (failed, hold released, no retry burn). A settlement failure
// AFTER the provider returned is NOT retryable (defect 2): that send is
// already billed, so it converges terminally on a detached context and
// never returns the error whose River redelivery would re-enter the
// provider. The post-commit Library save is best-effort:
// the task is complete regardless of its outcome.
//
// The gateway bridge below consumes the canonical execution plan and calls
// the EXISTING provider gateway seam. It never shapes provider bodies and
// never dials providers: transport/auth/retries stay inside the gateway.
// Gateway credentials resolve per execution through the AI gateway config
// seam and are never persisted.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/platform/telemetry"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/workflow/paramkeys"

	"go.opentelemetry.io/otel/attribute"
)

// generate runs the provider seam. A nil provider fails closed with a clear
// message (tasks converge to failed, never strand).
func (w *WorkspaceTaskWorker) generate(ctx context.Context, task *workspacetasks.WorkspaceTask, retryLayer string, attemptNumber int32) ([]workspacetasks.OutputRef, error) {
	if attemptNumber <= 0 {
		attemptNumber = int32(task.RetryCount + 1)
	}
	ctx, span := telemetry.Start(ctx, "generation.execution_attempt",
		attribute.String("service", task.Service), attribute.String("selected_model", task.ResolvedModel),
		attribute.Int("attempt_number", int(attemptNumber)), attribute.String("retry_layer", retryLayer))
	defer telemetry.End(span, nil)
	if w.provider == nil {
		return nil, errors.New(errorrender.MsgGenerate)
	}
	outputs, err := w.provider.Generate(ctx, task)
	if err != nil {
		return nil, err
	}
	for _, ref := range outputs {
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, fmt.Errorf(errorrender.MsgGenerate2, err)
		}
	}
	if len(outputs) > workspacetasks.MaxOutputsPerTask {
		return nil, fmt.Errorf(errorrender.MsgGenerate3, workspacetasks.MaxOutputsPerTask)
	}
	return outputs, nil
}

// completeExecution renders (when configured) and terminally completes
// one successful send. Renderer failures are deterministic finalize
// failures: failed, hold released, no retry burn.
func (w *WorkspaceTaskWorker) completeExecution(
	ctx context.Context,
	send *workspaceSend,
	claimed *workspacetasks.WorkspaceTask,
	outputs []workspacetasks.OutputRef,
	model string,
) error {
	workspaceID, taskID := claimed.WorkspaceID, claimed.ID
	if w.renderer != nil {
		rendered, renderErr := w.renderer.RenderOutputs(ctx, claimed, outputs)
		if renderErr != nil {
			send.finishFailed(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, workspaceFailureMessage(renderErr))
			return w.failTerminal(ctx, claimed, workspaceFailureMessage(renderErr))
		}
		outputs = rendered
	}
	send.finishCompleted(model)
	completed, err := w.settler.Complete(ctx, workspaceID, taskID, outputs)
	if err != nil {
		if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
			slog.Info("workspace task converged during completion; dropping", "task_id", taskID, "err", err)
			return nil
		}
		// The provider already returned, so this send is billed, but the
		// settlement transaction did not commit. Returning the error would
		// hand River a retryable outcome, and its redelivery re-opens the
		// SAME lease through the claim's same-token branch and calls the
		// provider again: one submission could produce up to
		// RiverMaxAttemptsDefault (25) billed sends. The send is therefore
		// terminal here (defect 2): it converges through the terminal
		// settlement path instead of re-entering the provider, and it does
		// NOT spend the whole-task retry budget, because ADR-0010 rule 5
		// scopes tasks.retry_count to transient PROVIDER failures only.
		slog.Warn("workspace task settlement failed after a billed send; converging terminally",
			"task_id", taskID, "model", model, "retry_layer", send.layer, "err", err)
		return w.convergeUnsettledSend(claimed, workspaceFailureMessage(fmt.Errorf("settlement failed after provider send: %w", err)))
	}
	// Save managed references after the atomic Complete commit. The
	// task is complete regardless of the outcome; trigger failures
	// are logged and never retried through River.
	w.saveCompletedTask(ctx, completed)
	slog.Info("workspace task completed", "task_id", taskID, "outputs", len(outputs),
		"model", model, "retry_layer", send.layer)
	return nil
}

// workspaceSettlementCleanupTimeout bounds the detached convergence attempt
// for a billed-but-unsettled send. It is deliberately short: this write only
// has to beat a brief infrastructure blip, and the execution context it
// replaces is frequently already cancelled.
const workspaceSettlementCleanupTimeout = 5 * time.Second

// convergeUnsettledSend terminally converges a task whose provider send
// already succeeded but whose settlement could not commit.
//
// It runs on a DETACHED bounded context because the dominant cause is the
// River execution context being cancelled at its timeout (or a pool blip), and
// reusing that dead context would throw away the only chance to release the
// hold. It retries the terminal write a few times inside that budget.
//
// It never returns a retryable error. After a billed send the only recovery a
// River retry can offer is a redelivery that calls the provider again, which
// is strictly worse than a task that stays running: the running row keeps its
// claim token, its durable attempt fact, and its Credit hold, so the state is
// fully attributable and an operator can settle it. Returning nil here is
// what caps one submission at ONE billed send.
func (w *WorkspaceTaskWorker) convergeUnsettledSend(task *workspacetasks.WorkspaceTask, reason string) error {
	if task == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), workspaceSettlementCleanupTimeout)
	defer cancel()
	const attempts = 3
	var err error
	tried := 0
	for i := 1; i <= attempts; i++ {
		if i > 1 {
			// Bounded backoff inside the detached budget: a short
			// infrastructure blip often clears on the next try, and a
			// dead context must stop the loop instead of spinning.
			select {
			case <-cleanupCtx.Done():
				i = attempts
				continue
			case <-time.After(time.Duration(i-1) * 200 * time.Millisecond):
			}
		}
		tried = i
		if _, err = w.settler.Fail(cleanupCtx, task.WorkspaceID, task.ID, reason); err == nil {
			slog.Warn("workspace task terminally failed after an unsettled billed send",
				"task_id", task.ID, "reason", reason, "attempts", tried)
			return nil
		}
		if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
			slog.Info("workspace task converged while settling an unsettled send; dropping",
				"task_id", task.ID, "err", err)
			return nil
		}
		slog.Warn("workspace task terminal convergence after an unsettled billed send failed",
			"task_id", task.ID, "attempt", tried, "of", attempts, "err", err)
	}
	// The task stays running with its claim token, its attempt fact, and its
	// hold. No retry_count is spent: the whole-task retry budget belongs to
	// transient provider failures (ADR-0010 rule 5), and requeueing here
	// would only re-enter the provider.
	slog.Error("workspace task stays running: provider send succeeded but neither settlement nor terminal convergence committed",
		"task_id", task.ID, "reason", reason, "attempts", tried, "err", err)
	return nil
}

// saveCompletedTask saves managed references after commit. The Task is
// already complete: failures are warn-logged and swallowed so a completed
// task never requeues through River.
func (w *WorkspaceTaskWorker) saveCompletedTask(ctx context.Context, completed *workspacetasks.WorkspaceTask) {
	if w.librarySaver == nil || completed == nil {
		return
	}
	if err := w.librarySaver.SaveCompletedTask(ctx, completed); err != nil {
		slog.Warn("workspace task auto-save failed; task stays complete",
			"task_id", completed.ID, "err", err)
	}
}

// WorkspaceGatewayCallInput carries one gateway execution's resolved
// inputs. It mirrors the runtime gateway seam with tasking-domain types so
// this package never imports the runtime layer; the execution token fences
// terminal writes so stale workers lose.
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

// Generate implements WorkspaceTaskProvider. The model is the
// snapshot-derived target the worker stamped onto the transient task
// copy before calling: the bridge never resolves a model itself and
// never consults the live pool or admin mapping.
func (p *workspaceGatewayProvider) Generate(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
	if p == nil || p.caller == nil {
		return nil, errors.New(errorrender.MsgGenerate4)
	}
	model := strings.TrimSpace(task.ResolvedModel)
	if model == "" {
		return nil, errors.New(errorrender.MsgGenerate5)
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
		return nil, errors.New(errorrender.MsgWorkspaceBridgeTaskPayload)
	}
	raw := strings.TrimSpace(task.CanonicalParams)
	if raw == "" {
		return nil, errors.New(errorrender.MsgWorkspaceBridgeTaskPayload2)
	}
	typed, err := tasks.UnmarshalParams(raw)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgWorkspaceBridgeTaskPayload3, err)
	}
	if typed.Extra == nil {
		typed.Extra = make(map[string]interface{})
	}
	typed.Extra[paramkeys.WorkspaceID] = strings.TrimSpace(task.WorkspaceID)
	encoded, err := json.Marshal(&typed)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgWorkspaceBridgeTaskPayload4, err)
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
			return nil, errors.New(errorrender.MsgMapWorkspaceProviderOutputs)
		}
		if err := workspacetasks.ValidateOutputRef(ref); err != nil {
			return nil, fmt.Errorf(errorrender.MsgMapWorkspaceProviderOutputs2, err)
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
