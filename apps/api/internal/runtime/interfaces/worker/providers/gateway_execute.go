package providers

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/telemetry"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/dialects"
	providerdomain "ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"

	"go.opentelemetry.io/otel/attribute"
)

// CallGatewayAPI executes one drawing operation through the sole AI egress.
// Transport, auth, and retries live inside the gateway; this method owns
// parameter validation, media resolution, dialect shaping/parsing, and
// result materialization. It never touches accounts, endpoints, or keys.
func (c *Executor) CallGatewayAPI(ctx context.Context, input runtimeports.GatewayCallInput) (runtimeports.ProviderCallOutput, error) {
	ctx, span := telemetry.Start(ctx, "generation.gateway.request",
		attribute.String("selected_model", input.Model),
		attribute.Int("attempt_number", int(input.AttemptNo)))
	defer telemetry.End(span, nil)
	if c == nil {
		return runtimeports.ProviderCallOutput{}, fmt.Errorf("provider executor is not initialized")
	}
	task := input.Task
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("drawing task is missing"))
	}
	correlationID := task.EnsureCorrelationID()
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, errors.New(errorrender.MsgRuntimeInterfacesWorkerProvidersCallGatewayAPI))
	}
	// Execution-side truth over the pinned display string: the pool
	// stores structured provider + bare model, the snapshot pins their
	// display form, and this split (not string-prefix inference) sets
	// the namespace, the URL path, and the native body model. A bare
	// model without a provider fails closed here.
	provider, modelName, pathOverride, err := workflow.ParseExecutionModelRef(model)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	// Catalog route overrides fill gaps only: mapping @path always wins.
	// A nil reader preserves mapping-only behavior (tests, minimal wiring).
	customParams := ""
	if pathOverride == "" && c != nil && c.modelRoutes != nil {
		catalogPath, catalogParams, found, routeErr := c.modelRoutes.LookupRouteOverride(ctx, provider, modelName)
		if routeErr != nil {
			return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryProviderError, fmt.Errorf(errorrender.MsgCallGatewayAPI2, routeErr))
		}
		if found {
			pathOverride = catalogPath
			customParams = catalogParams
		}
	}
	params, err := c.parseAndNormalizeTaskParams(ctx, task)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	if params.ExecutionPlan == nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("drawing task requires a durable execution plan"))
	}
	plan := *params.ExecutionPlan
	operation := plan.RuntimeOperation()
	// Single routing seam: operation dispatch, gateway path, and shaping
	// dialect resolve together; execution and preview share dialects.Resolve.
	route, err := dialects.Resolve(operation, provider, modelName, pathOverride, customParams)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	modelRequired := plan.RoutingRequirement().ModelRequirement() == workflow.RoutingModelRequired
	if modelRequired && modelName == "" {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("operation requires a model"))
	}
	params, err = c.resolveRuntimeProviderTaskParams(ctx, params)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	providerReq, err := params.providerRequest(task.Prompt, model)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}

	slogInfoGatewayCall(task.TaskID, correlationID, provider, model, operation)
	output := runtimeports.ProviderCallOutput{}
	providerStartedAt := time.Now()
	output.Timeline.ProviderStartedAt = &providerStartedAt
	// Gateway log metadata stays searchable: task id + feature travel as
	// cf-aig-metadata headers. Feature comes from canonical params (already
	// validated above); empty stays empty rather than failing the call.
	gatewayFeature := ""
	if params.Canonical != nil {
		gatewayFeature = strings.TrimSpace(params.Canonical.Service)
	}
	callResult, err := c.callGatewayAdapter(ctx, task.TaskID, gatewayFeature, route, provider, modelName, customParams, providerReq, operation, input.ExecutionToken)
	providerFinishedAt := time.Now()
	output.Timeline.ProviderFinishedAt = &providerFinishedAt
	applyGatewayReceipt(&output, callResult.Receipt, model)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return output, err
		}
		return output, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageProvider,
			providerFailureCategory(err),
			fmt.Errorf(errorrender.MsgCallGatewayAPI3, model, err),
		)
	}
	imageURLs := normalizeProviderResultImageURLs(callResult)
	if len(imageURLs) == 0 {
		return output, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageProvider,
			tasks.FailureCategoryProviderError,
			errors.New(errorrender.MsgCallGatewayAPI4),
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
		if _, _, ok := runtimeports.ExecutionFailureDetails(err); ok {
			return output, err
		}
		return output, runtimeports.NewExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, err)
	}
	if len(imageURLs) == 0 {
		markFinalizeFinished()
		return output, runtimeports.NewExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, errors.New(errorrender.MsgCallGatewayAPI5))
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
	return output, nil
}

func applyGatewayReceipt(output *runtimeports.ProviderCallOutput, receipt runtimeports.GatewayReceipt, requestedModel string) {
	if output == nil {
		return
	}
	output.ResolvedModel = strings.TrimSpace(receipt.Model)
	if output.ResolvedModel == "" {
		output.ResolvedModel = strings.TrimSpace(requestedModel)
	}
	output.GatewayStep = strings.TrimSpace(receipt.Step)
	output.CacheHit = receipt.CacheHit
	output.ReceiptLogID = strings.TrimSpace(receipt.LogID)
	output.ReceiptEventID = strings.TrimSpace(receipt.EventID)
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

// providerFailureCategory maps one provider call error to its failure
// category. It inlines the former errors.go helper verbatim: timeout first,
// then HTTP status mapping, provider error fallback.
func providerFailureCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return tasks.FailureCategoryTimeout
	}
	if status, ok := runtimeports.ProviderHTTPStatus(err); ok {
		switch status {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return tasks.FailureCategoryInputInvalid
		case http.StatusUnauthorized, http.StatusForbidden:
			return tasks.FailureCategoryAuth
		case http.StatusPaymentRequired:
			return tasks.FailureCategoryQuota
		case http.StatusRequestTimeout, http.StatusTooManyRequests:
			return tasks.FailureCategoryTimeout
		default:
			return tasks.FailureCategoryProviderError
		}
	}
	return tasks.FailureCategoryProviderError
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
	taskID, feature string,
	route dialects.Route,
	provider, model, customParams string,
	providerReq providerRequest,
	operation workflow.RuntimeOperation,
	executionToken string,
) (providerCallResult, error) {
	// Single 300s budget shared with the gateway client default
	// (runtimeports.GatewayDefaultTimeout, ADR-0010).
	timeout := runtimeports.GatewayDefaultTimeout
	switch route.Adapter {
	case providerdomain.ProviderRuntimeAdapterModelImage:
		return c.callGatewayModelImage(ctx, taskID, feature, provider, model, customParams, route, providerReq, operation, executionToken, timeout)
	case providerdomain.ProviderRuntimeAdapterCutout:
		payload, err := buildCutoutGatewayPayload(c, providerReq)
		if err != nil {
			return providerCallResult{}, err
		}
		return c.callGatewayImageBytes(ctx, taskID, feature, provider, model, route, payload, errorrender.MsgBuildCutoutGatewayPayload, executionToken, timeout)
	case providerdomain.ProviderRuntimeAdapterUpscale:
		payload, err := buildUpscaleGatewayPayload(c, providerReq)
		if err != nil {
			return providerCallResult{}, err
		}
		return c.callGatewayImageBytes(ctx, taskID, feature, provider, model, route, payload, errorrender.MsgBuildUpscaleGatewayPayload2, executionToken, timeout)
	default:
		return providerCallResult{}, fmt.Errorf("unsupported provider runtime adapter %d", route.Adapter)
	}
}
