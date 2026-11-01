package providers

import (
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

type providerTaskParams struct {
	Canonical     *tasks.WorkflowTaskParams
	ExecutionPlan *workflow.FeatureExecutionPlan
}

// NOTE: plan-consistency (RuntimeInputs/ControlContract逐字段镜像) is owned
// fail-closed by tasking submit (application/task validateSubmitParamsAgainstPlan
// + application/submission compiler). This package keeps only the Worker wire
// 最小形状: task_id/provider/model非空 (execute.go CallWorkerAPI +
// ports.WorkerRequest.Validate), images上限/顺序 (Validate cap 4 +
// workerInputRefs order + callWorker public-asset check), provider_options经
// 共享规则 workflow.ParseProviderOptions.

func newProviderTaskParams(params *tasks.WorkflowTaskParams) providerTaskParams {
	return providerTaskParams{Canonical: params}
}
