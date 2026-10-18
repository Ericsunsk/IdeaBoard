package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"net/url"
	"strings"

	"ideaboard/api/internal/platform/config"
	providerdomain "ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"
)

var _ runtimeports.GatewayCaller = (*Executor)(nil)

const (
	// maxProviderRawOutputPreviewLen keeps task result envelopes inspectable without
	// storing full image payloads or oversized provider responses.
	maxProviderRawOutputPreviewLen = 2000
	maxProviderRawOutputStringLen  = 300
	maxProviderRawOutputDepth      = 4
	maxProviderRawOutputArrayItems = 5
)

const (
	parsedOutputStatusValid = "valid"
)

func providerRawOutputPreview(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}

	var parsed interface{}
	if err := json.Unmarshal(raw, &parsed); err == nil {
		sanitized := sanitizeProviderRawOutputValue("", parsed, 0)
		if preview, err := json.Marshal(sanitized); err == nil {
			return truncateProviderRawOutputPreview(string(preview))
		}
	}
	return truncateProviderRawOutputPreview(trimmed)
}

func sanitizeProviderRawOutputValue(key string, value interface{}, depth int) interface{} {
	if depth >= maxProviderRawOutputDepth {
		switch typed := value.(type) {
		case map[string]interface{}:
			return fmt.Sprintf("[object:%d keys]", len(typed))
		case []interface{}:
			return fmt.Sprintf("[array:%d items]", len(typed))
		}
	}

	switch typed := value.(type) {
	case map[string]interface{}:
		sanitized := make(map[string]interface{}, len(typed))
		for nestedKey, nestedValue := range typed {
			sanitized[nestedKey] = sanitizeProviderRawOutputValue(nestedKey, nestedValue, depth+1)
		}
		return sanitized
	case []interface{}:
		limit := len(typed)
		if limit > maxProviderRawOutputArrayItems {
			limit = maxProviderRawOutputArrayItems
		}
		sanitized := make([]interface{}, 0, limit+1)
		for i := 0; i < limit; i++ {
			sanitized = append(sanitized, sanitizeProviderRawOutputValue(key, typed[i], depth+1))
		}
		if len(typed) > limit {
			sanitized = append(sanitized, fmt.Sprintf("...(+%d items)", len(typed)-limit))
		}
		return sanitized
	case string:
		return sanitizeProviderRawOutputString(key, typed)
	case float64, bool, nil:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func sanitizeProviderRawOutputString(key string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	if isProviderRawOutputImagePayloadKey(key) {
		return fmt.Sprintf("[redacted:%d chars]", len(value))
	}
	if isInlineDataImageURL(trimmed) {
		return redactLogURL(trimmed)
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return redactLogURL(trimmed)
	}
	if len(trimmed) > maxProviderRawOutputStringLen {
		return trimmed[:maxProviderRawOutputStringLen] + fmt.Sprintf("...(+%d chars)", len(trimmed)-maxProviderRawOutputStringLen)
	}
	return trimmed
}

func isProviderRawOutputImagePayloadKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "b64_json", "bytesbase64encoded", "data":
		return true
	}
	return strings.Contains(normalized, "base64")
}

func truncateProviderRawOutputPreview(value string) string {
	if len(value) <= maxProviderRawOutputPreviewLen {
		return value
	}
	return value[:maxProviderRawOutputPreviewLen] + fmt.Sprintf("...(+%d chars)", len(value)-maxProviderRawOutputPreviewLen)
}

type Executor struct {
	mediaReader        taskmedia.UploadReader
	mediaResolver      taskmedia.RuntimeMediaResolver
	resultMaterializer taskmedia.ResultMaterializer
	publicOrigins      config.PublicOriginsConfig
	gatewayFactory     runtimeports.GatewayClientFactory
	modelRoutes        runtimeports.ModelRouteOverrideReader
}

// ProviderExecutor is the internal name used by provider adapter files.
type ProviderExecutor = Executor

// NewExecutor constructs a new provider execution engine. A nil
// modelRoutes disables catalog route-override lookup (mapping @path only).
func NewExecutor(
	mediaReader taskmedia.UploadReader,
	resultMaterializer taskmedia.ResultMaterializer,
	publicOrigins config.PublicOriginsConfig,
	gatewayFactory runtimeports.GatewayClientFactory,
	modelRoutes runtimeports.ModelRouteOverrideReader,
) *Executor {
	executor := &Executor{
		mediaReader:        mediaReader,
		resultMaterializer: resultMaterializer,
		publicOrigins:      publicOrigins,
		gatewayFactory:     gatewayFactory,
		modelRoutes:        modelRoutes,
	}
	if resolver, ok := mediaReader.(taskmedia.RuntimeMediaResolver); ok {
		executor.mediaResolver = resolver
	}
	return executor
}

type providerCallResult struct {
	ImageURL          string
	ImageURLs         []string
	Model             string
	Receipt           runtimeports.GatewayReceipt
	RawOutputPreview  string
	ParsedOutputKind  string
	ParsedOutputCount int
}

type ProviderCallTimeline = runtimeports.ProviderCallTimeline
type ProviderCallOutput = runtimeports.ProviderCallOutput

// Adapter identity lives in providerdomain (single enum); this package switches on
// providerdomain.ProviderRuntimeAdapter* directly instead of converting.

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
		if validateErr := validateProviderTaskContract(typedParams, plan); validateErr != nil {
			return providerTaskParams{}, validateErr
		}
		executionPlan = &plan
	} else if len(typedParams.ExecutionPlan) > 0 && string(typedParams.ExecutionPlan) != "null" {
		return providerTaskParams{}, fmt.Errorf("non-image task must not carry execution plan")
	}
	params := newProviderTaskParams(typedParams, nil)
	params.ExecutionPlan = executionPlan
	return params, nil
}

func (c *Executor) resolveRuntimeProviderTaskParams(ctx context.Context, params providerTaskParams) (providerTaskParams, error) {
	if params.Canonical == nil || params.Canonical.Media == nil {
		return params, nil
	}
	if c == nil || c.mediaResolver == nil {
		return providerTaskParams{}, fmt.Errorf("runtime media resolver is unavailable")
	}
	runtimeMedia, err := c.mediaResolver.ResolveRuntimeMedia(ctx, params.Canonical)
	if err != nil {
		return providerTaskParams{}, fmt.Errorf("resolve task media: %w", err)
	}
	resolved := newProviderTaskParams(params.Canonical, runtimeMedia)
	resolved.ExecutionPlan = params.ExecutionPlan
	return resolved, nil
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
		AllowHosts: trustedImageDownloadHosts(ctx, c.publicOrigins),
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

func resolveProviderPublicImageURL(c *Executor, ref, label string) (string, error) {
	var baseParsedURL = c.publicOrigins.APIParsedURL()
	return providerdomain.ResolveProviderPublicImageURL(baseParsedURL, ref, label)
}
