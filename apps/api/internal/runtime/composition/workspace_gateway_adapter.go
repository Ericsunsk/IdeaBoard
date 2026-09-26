package composition

import (
	"context"
	"errors"

	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskworker "ideaboard/api/internal/tasking/interfaces/worker"
)

// workspaceGatewayCallerAdapter bridges the concrete runtime provider
// gateway executor to the tasking worker's gateway seam
// (taskworker.WorkspaceGatewayCaller). It lives in composition — the sole
// assembly root allowed to import both layers — so the tasking worker
// never imports runtime. Translation is field-for-field over
// tasking-domain types; no shaping, no policy.
type workspaceGatewayCallerAdapter struct {
	executor *providers.Executor
}

var _ taskworker.WorkspaceGatewayCaller = (*workspaceGatewayCallerAdapter)(nil)

// CallGatewayAPI implements taskworker.WorkspaceGatewayCaller.
func (a *workspaceGatewayCallerAdapter) CallGatewayAPI(ctx context.Context, input taskworker.WorkspaceGatewayCallInput) (taskworker.WorkspaceGatewayCallOutput, error) {
	if a == nil || a.executor == nil {
		return taskworker.WorkspaceGatewayCallOutput{}, errors.New("上游图像服务未配置，请联系管理员")
	}
	output, err := a.executor.CallGatewayAPI(ctx, runtimeports.GatewayCallInput{
		Task:           input.Task,
		Model:          input.Model,
		ExecutionToken: input.ExecutionToken,
	})
	if err != nil {
		return taskworker.WorkspaceGatewayCallOutput{}, err
	}
	return taskworker.WorkspaceGatewayCallOutput{Outputs: output.Outputs}, nil
}
