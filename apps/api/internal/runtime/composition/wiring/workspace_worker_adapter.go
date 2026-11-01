package wiring

import (
	"context"
	"errors"
	"ideaboard/api/internal/platform/errorrender"

	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskworker "ideaboard/api/internal/tasking/interfaces/worker"
)

// workspaceWorkerCallerAdapter bridges the concrete runtime provider
// Worker executor to the tasking worker's Worker seam
// (taskworker.WorkspaceWorkerCaller). It lives in composition — the sole
// assembly root allowed to import both layers — so the tasking worker
// never imports runtime. Translation is field-for-field over
// tasking-domain types; no shaping, no policy.
type workspaceWorkerCallerAdapter struct {
	executor *providers.Executor
}

var _ taskworker.WorkspaceWorkerCaller = (*workspaceWorkerCallerAdapter)(nil)

// CallWorkerAPI implements taskworker.WorkspaceWorkerCaller.
func (a *workspaceWorkerCallerAdapter) CallWorkerAPI(ctx context.Context, input taskworker.WorkspaceWorkerCallInput) (taskworker.WorkspaceWorkerCallOutput, error) {
	if a == nil || a.executor == nil {
		return taskworker.WorkspaceWorkerCallOutput{}, errors.New(errorrender.MsgRuntimeCompositionWiringCallWorkerAPI)
	}
	output, err := a.executor.CallWorkerAPI(ctx, runtimeports.WorkerCallInput{
		Task:  input.Task,
		Model: input.Model,
	})
	if err != nil {
		return taskworker.WorkspaceWorkerCallOutput{}, err
	}
	return taskworker.WorkspaceWorkerCallOutput{Outputs: output.Outputs}, nil
}
