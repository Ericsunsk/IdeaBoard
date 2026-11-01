package providers

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/telemetry"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"

	"go.opentelemetry.io/otel/attribute"
)

func slogInfoWorkerCall(taskID, correlationID, provider, model string, operation workflow.RuntimeOperation) {
	slog.Info("calling worker API",
		"task_id", taskID, "correlation_id", correlationID,
		"provider", provider, "model", model, "operation", string(operation))
}

// CallWorkerAPI executes one drawing operation through the sole Worker egress.
// Transport, auth, and retries live inside the Worker; this method owns
// parameter validation, presigned media handoff, and result materialization.
// It never touches accounts, endpoints, or keys.
func (c *Executor) CallWorkerAPI(ctx context.Context, input runtimeports.WorkerCallInput) (runtimeports.ProviderCallOutput, error) {
	ctx, span := telemetry.Start(ctx, "generation.worker.request",
		attribute.String("selected_model", input.Model),
		attribute.Int("attempt_number", int(input.AttemptNo)))
	defer telemetry.End(span, nil)
	if c == nil {
		return runtimeports.ProviderCallOutput{}, fmt.Errorf("provider executor is not initialized")
	}
	if c.transfer == nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryProviderError, fmt.Errorf("worker transfer is not configured"))
	}
	task := input.Task
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("drawing task is missing"))
	}
	correlationID := task.EnsureCorrelationID()
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, errors.New(errorrender.MsgRuntimeInterfacesWorkerProvidersCallWorkerAPI))
	}
	// Execution-side truth over the pinned display string: the pool
	// stores structured provider + bare model, the snapshot pins their
	// display form, and this split sets the Worker provider/model facts.
	// A bare model without a provider fails closed here (the Worker is
	// authoritative for provider membership and rejects the rest).
	provider, modelName, err := workflow.ParseExecutionModelRef(model)
	if err != nil {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, err)
	}
	// Catalog custom params fill gaps only (opaque object merged by the
	// Worker after its defaults). A nil reader preserves Worker-default
	// behavior (tests, minimal wiring).
	var providerOptions map[string]interface{}
	if c != nil && c.customParams != nil {
		foundParams, found, routeErr := c.customParams.LookupCustomParams(ctx, provider, modelName)
		if routeErr != nil {
			return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryProviderError, fmt.Errorf(errorrender.MsgCallWorkerAPI2, routeErr))
		}
		if found {
			// Canonical custom-params rule lives in workflow.ParseProviderOptions
			// (shared with Admin preview validation); malformed catalog JSON is
			// ignored here because SaveCatalog already fails it closed upstream.
			providerOptions, _ = workflow.ParseProviderOptions(foundParams)
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
	modelRequired := plan.RoutingRequirement().ModelRequirement() == workflow.RoutingModelRequired
	if modelRequired && modelName == "" {
		return runtimeports.ProviderCallOutput{}, runtimeports.NewExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, fmt.Errorf("operation requires a model"))
	}

	slogInfoWorkerCall(task.TaskID, correlationID, provider, model, operation)
	output := runtimeports.ProviderCallOutput{}
	providerStartedAt := time.Now()
	output.Timeline.ProviderStartedAt = &providerStartedAt
	// Worker log metadata stays searchable: task id + feature travel for
	// RED observations. Feature comes from canonical params (already
	// validated above); empty stays empty rather than failing the call.
	workerFeature := ""
	if params.Canonical != nil {
		workerFeature = strings.TrimSpace(params.Canonical.Service)
	}
	workspaceID := workerWorkspaceID(params)
	// NOTE: the execution token is deliberately not forwarded: the edge
	// contract carries no idempotency field, and local settlement never
	// depended on upstream dedup (the claim token fences terminal writes).
	callResult, err := c.callWorker(
		ctx, task.TaskID, workerFeature, workspaceID, provider, modelName,
		task.Prompt, params, providerOptions,
	)
	providerFinishedAt := time.Now()
	output.Timeline.ProviderFinishedAt = &providerFinishedAt
	applyWorkerReceipt(&output, callResult.Receipt, model)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return output, err
		}
		return output, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageProvider,
			providerFailureCategory(err),
			fmt.Errorf(errorrender.MsgCallWorkerAPI3, model, err),
		)
	}
	imageURLs := normalizeProviderResultImageURLs(callResult)
	if len(imageURLs) == 0 {
		return output, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageProvider,
			tasks.FailureCategoryProviderError,
			errors.New(errorrender.MsgCallWorkerAPI4),
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
		return output, runtimeports.NewExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, errors.New(errorrender.MsgCallWorkerAPI5))
	}
	imageURL := imageURLs[0]
	finalizeFinishedAt := time.Now()
	output.Timeline.FinalizeFinishedAt = &finalizeFinishedAt

	result := baseWorkerTaskResult(task, model, callResult, imageURLs)
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

func applyWorkerReceipt(output *runtimeports.ProviderCallOutput, receipt runtimeports.WorkerReceipt, requestedModel string) {
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

func baseWorkerTaskResult(task *tasks.TaskPayload, model string, callResult providerCallResult, imageURLs []string) map[string]interface{} {
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

// providerFailureCategory maps one Worker call error to its failure
// category: edge codes first, then transport HTTP status, provider error
// fallback. Deterministic codes (bad request, auth, unsupported model)
// never burn retry budget; upstream/transient codes do per policy.
func providerFailureCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return tasks.FailureCategoryTimeout
	}
	if code, ok := runtimeports.WorkerErrorDetails(err); ok {
		switch code {
		case runtimeports.WorkerErrorBadRequest, runtimeports.WorkerErrorUnsupportedModel:
			return tasks.FailureCategoryInputInvalid
		case runtimeports.WorkerErrorUnauthorized:
			return tasks.FailureCategoryAuth
		case runtimeports.WorkerErrorUpstream:
			var workerErr *runtimeports.WorkerError
			if errors.As(err, &workerErr) && workerErr.UpstreamStatus != 0 {
				return providerFailureCategoryForStatus(workerErr.UpstreamStatus)
			}
			return tasks.FailureCategoryProviderError
		default:
			return tasks.FailureCategoryProviderError
		}
	}
	if status, ok := runtimeports.ProviderHTTPStatus(err); ok {
		return providerFailureCategoryForStatus(status)
	}
	return tasks.FailureCategoryProviderError
}

func providerFailureCategoryForStatus(status int) string {
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

func (c *Executor) workerClientForCall(ctx context.Context) (runtimeports.WorkerClient, error) {
	if c == nil || c.workerFactory == nil {
		return nil, fmt.Errorf("worker client factory is not configured")
	}
	client, err := c.workerFactory(ctx)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("worker client factory returned nil")
	}
	return client, nil
}

// callWorker is the single Worker send for one execution: presign inputs +
// output, POST CanonicalImageRequest, verify the landed key exactly, read
// back landed bytes. Transport errors carry on unchanged; context.Canceled
// passes through so interruption converges as requeue, not failure.
func (c *Executor) callWorker(
	ctx context.Context,
	taskID, feature string,
	workspaceID, provider, model, prompt string,
	params providerTaskParams,
	providerOptions map[string]interface{},
) (providerCallResult, error) {
	client, err := c.workerClientForCall(ctx)
	if err != nil {
		return providerCallResult{}, err
	}
	if c.transfer == nil {
		return providerCallResult{}, fmt.Errorf("worker transfer is not configured")
	}
	// Presigned GETs for inputs (inputs first, mask last).
	var inputURLs []string
	for _, ref := range workerInputRefs(params) {
		publicID := strings.TrimSpace(ref.PublicAssetID)
		if publicID == "" {
			return providerCallResult{}, fmt.Errorf("worker input asset is missing a public asset id")
		}
		signed, _, err := c.transfer.PresignInputGET(ctx, workspaceID, publicID)
		if err != nil {
			return providerCallResult{}, err
		}
		inputURLs = append(inputURLs, signed)
	}
	var maskIndex *int
	if params.Canonical != nil && params.Canonical.Media != nil && params.Canonical.Media.Mask != nil {
		mask := params.Canonical.Media.Mask
		if mask.AssetID > 0 || strings.TrimSpace(mask.PublicAssetID) != "" {
			i := len(inputURLs) - 1
			maskIndex = &i
		}
	}
	// The slot owns the output key, signed policy and trusted acceptance.
	slot, err := c.transfer.PrepareOutput(workspaceID, taskID)
	if err != nil {
		return providerCallResult{}, err
	}
	width, height := workerImageSize(nil)
	if params.Canonical != nil && params.Canonical.Controls != nil {
		width, height = workerImageSize(params.Canonical.Controls)
	}
	var seed uint64
	if params.Canonical != nil && params.Canonical.Generation != nil {
		seed = params.Canonical.Generation.Seed
	}
	timeout := runtimeports.WorkerDefaultTimeout
	resp, err := client.Do(ctx, runtimeports.WorkerRequest{
		TaskID:          taskID,
		Provider:        provider,
		Model:           model,
		Prompt:          prompt,
		Width:           width,
		Height:          height,
		Seed:            seed,
		Images:          inputURLs,
		MaskIndex:       maskIndex,
		ProviderOptions: providerOptions,
		UploadURL:       slot.UploadURL(),
		Feature:         feature,
		Timeout:         timeout,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{Receipt: resp.Receipt}, err
		}
		return providerCallResult{Receipt: resp.Receipt}, fmt.Errorf("worker image call failed: %w", err)
	}
	dataURL, err := slot.Accept(ctx, resp.ImageURL, resp.ContentType)
	if err != nil {
		return providerCallResult{Receipt: resp.Receipt}, err
	}
	preview := strings.TrimSpace(resp.RawPreview)
	if preview == "" || strings.Contains(preview, "base64,") {
		preview = providerRawOutputPreview("1 worker image")
	}
	return providerCallResult{
		ImageURL:          dataURL,
		ImageURLs:         []string{dataURL},
		Model:             provider + "/" + model,
		Receipt:           resp.Receipt,
		RawOutputPreview:  preview,
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: 1,
	}, nil
}
