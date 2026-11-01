package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/workertransfer"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

var _ runtimeports.WorkerCaller = (*Executor)(nil)

const (
	// maxProviderRawOutputPreviewLen keeps task result envelopes inspectable without
	// storing full image payloads or oversized Worker responses.
	maxProviderRawOutputPreviewLen = 2000
)

const (
	parsedOutputStatusValid = "valid"
)

func providerRawOutputPreview(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return truncateProviderRawOutputPreview(trimmed)
}

func truncateProviderRawOutputPreview(value string) string {
	if len(value) <= maxProviderRawOutputPreviewLen {
		return value
	}
	return value[:maxProviderRawOutputPreviewLen] + fmt.Sprintf("...(+%d chars)", len(value)-maxProviderRawOutputPreviewLen)
}

type Executor struct {
	resultMaterializer taskmedia.ResultMaterializer
	workerFactory      runtimeports.WorkerClientFactory
	customParams       runtimeports.ModelCustomParamsReader
	transfer           workertransfer.WorkerTransfer
}

// ProviderExecutor is the internal name used by provider adapter files.
type ProviderExecutor = Executor

// NewExecutor constructs the sole Worker execution engine. A nil
// customParams disables catalog custom-params lookup (Worker defaults;
// tests, minimal wiring). Transfer is required: nil fails every execution
// closed before any Worker send.
func NewExecutor(
	resultMaterializer taskmedia.ResultMaterializer,
	workerFactory runtimeports.WorkerClientFactory,
	customParams runtimeports.ModelCustomParamsReader,
	transfer workertransfer.WorkerTransfer,
) *Executor {
	return &Executor{
		resultMaterializer: resultMaterializer,
		workerFactory:      workerFactory,
		customParams:       customParams,
		transfer:           transfer,
	}
}

type providerCallResult struct {
	ImageURL          string
	ImageURLs         []string
	Model             string
	Receipt           runtimeports.WorkerReceipt
	RawOutputPreview  string
	ParsedOutputKind  string
	ParsedOutputCount int
}

type ProviderCallTimeline = runtimeports.ProviderCallTimeline
type ProviderCallOutput = runtimeports.ProviderCallOutput

// CallDrawingAPIWithAssignment executes drawing operation for an assigned account.

func resolvedProviderParsedOutputKind(callResult providerCallResult) string {
	if strings.TrimSpace(callResult.ParsedOutputKind) != "" {
		return strings.TrimSpace(callResult.ParsedOutputKind)
	}
	return "image-results"
}

func resolvedProviderParsedOutputCount(callResult providerCallResult, imageURLs []string) int {
	if callResult.ParsedOutputCount > 0 {
		return callResult.ParsedOutputCount
	}
	return len(imageURLs)
}

func (c *Executor) parseAndNormalizeTaskParams(ctx context.Context, task *tasks.TaskPayload) (providerTaskParams, error) {
	typedParams, err := tasks.UnmarshalParams(task.Params)
	if err != nil {
		return providerTaskParams{}, fmt.Errorf("parse task params: %w", err)
	}
	feature := strings.TrimSpace(typedParams.Service)
	var executionPlan *workflow.FeatureExecutionPlan
	if workflow.IsAIImageFeature(feature) {
		plan, decodeErr := workflow.DecodeFeatureExecutionPlan(typedParams.ExecutionPlan)
		if decodeErr != nil {
			return providerTaskParams{}, fmt.Errorf("decode task execution plan: %w", decodeErr)
		}
		if plan.Feature() != feature {
			return providerTaskParams{}, fmt.Errorf("task execution plan service does not match params service")
		}
		// Plan-consistency is owned fail-closed by tasking submit
		// (validateSubmitParamsAgainstPlan + submission compiler); the
		// execution side keeps only the Worker wire最小形状 (task_id /
		// provider / model presence in CallWorkerAPI, images上限/顺序 in
		// WorkerRequest.Validate + workerInputRefs, provider_options via
		// workflow.ParseProviderOptions).
		executionPlan = &plan
	} else if len(typedParams.ExecutionPlan) > 0 && string(typedParams.ExecutionPlan) != "null" {
		return providerTaskParams{}, fmt.Errorf("non-image task must not carry execution plan")
	}
	params := newProviderTaskParams(typedParams)
	params.ExecutionPlan = executionPlan
	return params, nil
}

func workerWorkspaceID(params providerTaskParams) string {
	if params.Canonical == nil || params.Canonical.Extra == nil {
		return ""
	}
	if ws, ok := params.Canonical.Extra[paramkeys.WorkspaceID].(string); ok {
		return strings.TrimSpace(ws)
	}
	return ""
}

// workerInputRefs flattens canonical media refs in Worker order (inputs,
// references, original, mask last). callWorker explicitly identifies the mask
// with MaskIndex; multiple ordinary references never imply a mask.
func workerInputRefs(params providerTaskParams) []tasks.MediaRef {
	if params.Canonical == nil || params.Canonical.Media == nil {
		return nil
	}
	media := params.Canonical.Media
	refs := make([]tasks.MediaRef, 0, len(media.Inputs)+len(media.References)+2)
	refs = append(refs, media.Inputs...)
	refs = append(refs, media.References...)
	if media.Original != nil {
		refs = append(refs, *media.Original)
	}
	if media.Mask != nil {
		refs = append(refs, *media.Mask)
	}
	out := make([]tasks.MediaRef, 0, len(refs))
	for _, ref := range refs {
		if ref.AssetID <= 0 && strings.TrimSpace(ref.PublicAssetID) == "" {
			continue
		}
		out = append(out, ref)
	}
	return out
}

// workerImageSize derives the planned output pixels from canonical controls:
// explicit custom dimensions win (scaled by quality tier), else ratio plus
// quality tier snapped to 64px.
func workerImageSize(controls *tasks.GenerationControls) (width, height int) {
	var customWidth, customHeight int
	var ratio, quality string
	if controls != nil {
		customWidth, customHeight = int(controls.CustomWidth), int(controls.CustomHeight)
		ratio, quality = controls.Ratio, controls.Quality
	}
	if customWidth > 0 && customHeight > 0 {
		w, h := customWidth, customHeight
		switch strings.ToLower(strings.TrimSpace(quality)) {
		case "2k":
			w, h = w*2, h*2
		case "4k":
			w, h = w*4, h*4
		}
		return w, h
	}
	wr, hr := 1.0, 1.0
	if trimmed := strings.TrimSpace(ratio); trimmed != "" {
		parts := strings.Split(trimmed, ":")
		if len(parts) == 2 {
			wVal, errW := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			hVal, errH := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if errW == nil && errH == nil && wVal > 0 && hVal > 0 {
				wr, hr = wVal, hVal
			}
		}
	}
	k := 1
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "2k":
		k = 2
	case "4k":
		k = 4
	}
	baseLength := float64(1024 * k)
	r := wr / hr
	var wFloat, hFloat float64
	if r >= 1.0 {
		wFloat = baseLength
		hFloat = math.Round(baseLength / r)
	} else {
		hFloat = baseLength
		wFloat = math.Round(baseLength * r)
	}
	w := int(math.Max(64, math.Round(wFloat/64.0)*64.0))
	h := int(math.Max(64, math.Round(hFloat/64.0)*64.0))
	return w, h
}

func marshalProviderTaskResult(taskID string, result map[string]interface{}) (string, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryFinalizeError,
			fmt.Errorf(errorrender.MsgMarshalProviderTaskResult, taskID, err),
		)
	}
	return string(resultJSON), nil
}

func (c *Executor) materializeProviderResultImages(ctx context.Context, userID int64, taskID string, correlationID string, imageURLs []string) ([]string, []tasks.TaskOutput, error) {
	if len(imageURLs) == 0 {
		return imageURLs, nil, nil
	}
	if c.resultMaterializer == nil {
		return nil, nil, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			errors.New(errorrender.MsgMaterializeProviderResultImages),
		)
	}

	correlationID = tasks.NormalizeCorrelationID(correlationID)
	if correlationID == "" {
		correlationID = tasks.CorrelationIDForTaskID(taskID)
	}
	slog.Info("provider result materialization started", "task_id", taskID, "correlation_id", correlationID, "image_count", len(imageURLs))

	materialized, storeErr := c.resultMaterializer.MaterializeRemoteResults(ctx, userID, taskID, imageURLs, taskmedia.ResultMaterializeOptions{
		AllowHosts: trustedImageDownloadHosts(ctx),
	})
	if storeErr != nil {
		if errors.Is(storeErr, context.Canceled) {
			return nil, nil, storeErr
		}
		return nil, nil, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf(errorrender.MsgMaterializeProviderResultImages2, storeErr),
		)
	}
	if len(materialized.Items) == 0 {
		return nil, nil, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			errors.New(errorrender.MsgMaterializeProviderResultImages3),
		)
	}
	if len(materialized.Items) != len(imageURLs) {
		return nil, nil, runtimeports.NewExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf(errorrender.MsgMaterializeProviderResultImages4, len(imageURLs), len(materialized.Items)),
		)
	}
	for index, sourceURL := range imageURLs {
		slog.Info("provider image materialized", "task_id", taskID, "correlation_id", correlationID, "source_url", redactLogURL(sourceURL), "stored_url", redactLogURL(materialized.Items[index].ImageURL))
	}
	return materialized.ImageURLs(), materialized.Outputs(), nil
}

func isInlineDataImageURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:image/")
}

func redactLogURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if isInlineDataImageURL(trimmed) {
		return "data:image/...;base64,[redacted]"
	}
	if parsed, err := url.Parse(trimmed); err == nil {
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			clone := *parsed
			clone.User = nil
			clone.RawQuery = ""
			clone.ForceQuery = false
			clone.Fragment = ""
			trimmed = clone.String()
		}
	}
	if len(trimmed) <= 80 {
		return trimmed
	}
	return trimmed[:80] + "...(truncated)"
}

func normalizeProviderResultImageURLs(result providerCallResult) []string {
	source := result.ImageURLs
	if len(source) == 0 && strings.TrimSpace(result.ImageURL) != "" {
		source = []string{result.ImageURL}
	}
	urls := make([]string, 0, len(source))
	for _, raw := range source {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		urls = append(urls, url)
	}
	return urls
}
