package worker

// Execution phase (#860): run one provider send against the
// snapshot-derived model and converge success.
//
// generate runs the provider seam (nil provider fails closed: tasks
// converge to failed, never strand). completeExecution renders (when
// configured) and terminally completes one successful send through the
// retry+settlement phase; renderer failures are deterministic finalize
// failures (failed, hold released, no retry burn). The post-commit
// auto-materialize trigger (#828) is best-effort: the task is complete
// regardless of its outcome.
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
	"log/slog"
	"strings"

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
		return fmt.Errorf("workspace task complete: %w", err)
	}
	// #828: auto-materialize AFTER the atomic Complete commit. The
	// task is complete regardless of the outcome; trigger failures
	// are logged and never retried through River.
	w.materializeCompletedTask(ctx, completed)
	slog.Info("workspace task completed", "task_id", taskID, "outputs", len(outputs),
		"model", model, "retry_layer", send.layer)
	return nil
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
