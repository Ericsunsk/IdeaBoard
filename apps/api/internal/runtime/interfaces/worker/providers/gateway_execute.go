package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

const (
	// defaultGatewayCallTimeout bounds one gateway attempt for ops whose
	// provider latency is unknown but historically interactive.
	defaultGatewayCallTimeout = 300 * time.Second
)

// CallGatewayAPI executes one drawing operation through the sole AI egress.
// Transport, auth, and retries live inside the gateway; this method owns
// parameter validation, media resolution, dialect shaping/parsing, and
// result materialization. It never touches accounts, endpoints, or keys.
func (c *Executor) CallGatewayAPI(ctx context.Context, input runtimeports.GatewayCallInput) (runtimeports.ProviderCallOutput, error) {
	if c == nil {
		return runtimeports.ProviderCallOutput{}, fmt.Errorf("provider executor is not initialized")
	}
	task := input.Task
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("drawing task is missing"))
	}
	correlationID := task.EnsureCorrelationID()
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("任务未分配模型，请先在后台配置功能模型"))
	}
	provider, modelName, pathOverride, err := runtimeports.SplitGatewayModel(model)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	// Catalog route overrides fill gaps only: mapping @path always wins.
	// A nil reader preserves mapping-only behavior (tests, minimal wiring).
	customParams := ""
	if pathOverride == "" && c != nil && c.modelRoutes != nil {
		catalogPath, catalogParams, found, routeErr := c.modelRoutes.LookupRouteOverride(ctx, model)
		if routeErr != nil {
			return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryProviderError, fmt.Errorf("读取模型路由配置失败: %w", routeErr))
		}
		if found {
			pathOverride = catalogPath
			customParams = catalogParams
		}
	}
	params, err := c.parseAndNormalizeTaskParams(ctx, task)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	if params.ExecutionPlan == nil {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("drawing task requires a durable execution plan"))
	}
	plan := *params.ExecutionPlan
	operation := plan.RuntimeOperation()
	adapter, err := domain.AdapterForOperation(operation)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	modelRequired := plan.RoutingRequirement().ModelRequirement() == workflow.RoutingModelRequired
	if modelRequired && modelName == "" {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("operation requires a model"))
	}
	params, err = c.resolveRuntimeProviderTaskParams(ctx, params)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	providerReq, err := params.providerRequest(task.Prompt, model)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}

	slogInfoGatewayCall(task.TaskID, correlationID, provider, model, operation)
	output := runtimeports.ProviderCallOutput{}
	providerStartedAt := time.Now()
	output.Timeline.ProviderStartedAt = &providerStartedAt
	callResult, err := c.callGatewayAdapter(ctx, adapter, provider, modelName, pathOverride, customParams, providerReq, operation)
	providerFinishedAt := time.Now()
	output.Timeline.ProviderFinishedAt = &providerFinishedAt
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return output, err
		}
		return output, newExecutionFailureError(
			tasks.ExecutionStageProvider,
			providerFailureCategory(err),
			fmt.Errorf("调用 API 失败 (模型: %s): %w", model, err),
		)
	}
	imageURLs := normalizeProviderResultImageURLs(callResult)
	if len(imageURLs) == 0 {
		return output, newExecutionFailureError(
			tasks.ExecutionStageProvider,
			tasks.FailureCategoryProviderError,
			fmt.Errorf("API 未返回图片"),
		)
	}
	finalizeStartedAt := time.Now()
	output.Timeline.FinalizeStartedAt = &finalizeStartedAt
	markFinalizeFinished := func() {
		if output.Timeline.FinalizeFinishedAt != nil {
			return
		}
		finalizeFinishedAt := time.Now()
		output.Timeline.FinalizeFinishedAt = &finalizeFinishedAt
	}
	finalization := providerResultFinalizer{
		executor:      c,
		correlationID: task.EffectiveCorrelationID(),
	}
	finalizeResult := finalization.finalize(ctx, task.UserID, task.TaskID, imageURLs)
	imageURLs, taskOutputs, err := finalizeResult.completedOutputs()
	if err != nil {
		markFinalizeFinished()
		if errors.Is(err, context.Canceled) {
			return output, err
		}
		if _, _, ok := executionFailureDetails(err); ok {
			return output, err
		}
		return output, newExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, err)
	}
	if len(imageURLs) == 0 {
		markFinalizeFinished()
		return output, newExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, fmt.Errorf("finalizer 未返回图片"))
	}
	imageURL := imageURLs[0]
	finalizeFinishedAt := time.Now()
	output.Timeline.FinalizeFinishedAt = &finalizeFinishedAt

	result := baseGatewayTaskResult(task, model, callResult, imageURLs)
	result["status"] = tasks.ResultStatusSuccess
	result["image_url"] = imageURL
	result["image_urls"] = imageURLs
	result["model"] = model

	resultJSON, err := marshalProviderTaskResult(task.TaskID, result)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, err
	}
	output.Result = resultJSON
	output.Outputs = taskOutputs
	output.ResolvedModel = model
	return output, nil
}

func baseGatewayTaskResult(task *tasks.TaskPayload, model string, callResult providerCallResult, imageURLs []string) map[string]interface{} {
	result := map[string]interface{}{
		"task_id":        task.TaskID,
		"correlation_id": task.EffectiveCorrelationID(),
		"model":          model,
	}
	result["parsed_output_status"] = "valid"
	result["parsed_output"] = map[string]interface{}{
		"kind":        resolvedProviderParsedOutputKind(callResult),
		"image_count": resolvedProviderParsedOutputCount(callResult, imageURLs),
	}
	if strings.TrimSpace(callResult.RawOutputPreview) != "" {
		result["raw_provider_output_preview"] = strings.TrimSpace(callResult.RawOutputPreview)
	}
	return result
}

// gatewayCallTimeout bounds one gateway attempt per adapter family.
func gatewayCallTimeout(_ domain.ProviderRuntimeAdapter) time.Duration {
	return defaultGatewayCallTimeout
}

func (c *Executor) gatewayClientForCall(ctx context.Context) (runtimeports.GatewayClient, error) {
	if c == nil || c.gatewayFactory == nil {
		return nil, fmt.Errorf("gateway client factory is not configured")
	}
	client, err := c.gatewayFactory(ctx)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("gateway client factory returned nil")
	}
	return client, nil
}

func (c *Executor) callGatewayAdapter(
	ctx context.Context,
	adapter domain.ProviderRuntimeAdapter,
	provider, model, pathOverride, customParams string,
	providerReq providerRequest,
	operation workflow.RuntimeOperation,
) (providerCallResult, error) {
	timeout := gatewayCallTimeout(adapter)
	switch adapter {
	case domain.ProviderRuntimeAdapterModelImage:
		return c.callGatewayModelImage(ctx, provider, model, pathOverride, customParams, providerReq, timeout)
	case domain.ProviderRuntimeAdapterCutout:
		path, err := cutoutGatewayPath(pathOverride)
		if err != nil {
			return providerCallResult{}, err
		}
		payload, err := buildCutoutGatewayPayload(c, providerReq)
		if err != nil {
			return providerCallResult{}, err
		}
		return c.callGatewayImageBytes(ctx, provider, model, path, payload, "抠图", timeout)
	case domain.ProviderRuntimeAdapterUpscale:
		path, err := upscaleGatewayPath(pathOverride)
		if err != nil {
			return providerCallResult{}, err
		}
		payload, err := buildUpscaleGatewayPayload(c, providerReq)
		if err != nil {
			return providerCallResult{}, err
		}
		return c.callGatewayImageBytes(ctx, provider, model, path, payload, "高清放大", timeout)
	default:
		return providerCallResult{}, fmt.Errorf("unsupported provider runtime adapter %d", adapter)
	}
}
