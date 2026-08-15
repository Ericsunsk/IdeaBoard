package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

const (
	// maxAPIResponseBody caps upstream API reads to prevent OOM from malicious or broken streams.
	maxAPIResponseBody = 50 * 1024 * 1024 // 50 MB
	// maxRemoteImageRefBytes caps remote image reference downloads to match the
	// 10 MB limit already enforced for data URLs and local uploads.
	maxRemoteImageRefBytes = 10 * 1024 * 1024
	// maxErrorBodyLen caps error messages stored in task records.
	maxErrorBodyLen = 500
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

func readLimitedBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxAPIResponseBody+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAPIResponseBody {
		return nil, fmt.Errorf("响应体超过大小限制 (%d MB)", maxAPIResponseBody/1024/1024)
	}
	return data, nil
}

// readLimitedImageRef reads a remote image reference body with the 10 MB cap
// that matches data URL and local upload limits.
func readLimitedImageRef(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxRemoteImageRefBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRemoteImageRefBytes {
		return nil, fmt.Errorf("参考图像超过大小限制 (%d MB)", maxRemoteImageRefBytes/1024/1024)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("参考图像为空")
	}
	return data, nil
}

func truncateErrorBody(body string) string {
	if len(body) > maxErrorBodyLen {
		return body[:maxErrorBodyLen] + "...(truncated)"
	}
	return body
}

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

type ProviderExecutor struct {
	providerEndpointRepo *providerendpoints.ProviderEndpointRepository
	mediaReader          taskmedia.UploadReader
	resultMaterializer   taskmedia.ResultMaterializer
	publicOrigins        config.PublicOriginsConfig
}

func (c *ProviderExecutor) readRelativeUploadObject(ctx context.Context, ref string, maxBytes int64) (*taskmedia.StoredObject, bool, error) {
	trimmed := strings.TrimSpace(ref)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return nil, false, nil
	}
	if c == nil || c.mediaReader == nil {
		return nil, true, fmt.Errorf("存储服务未初始化，无法读取上传文件: %q", ref)
	}
	obj, err := c.mediaReader.ReadUploadObject(ctx, trimmed, maxBytes)
	if err != nil {
		return nil, true, err
	}
	if obj == nil {
		return nil, true, fmt.Errorf("存储服务返回空上传文件: %q", ref)
	}
	return obj, true, nil
}

type providerCallResult struct {
	ImageURL          string
	ImageURLs         []string
	Model             string
	RawOutputPreview  string
	ParsedOutputKind  string
	ParsedOutputCount int
}

type providerCallTimeline struct {
	ProviderStartedAt  *time.Time
	ProviderFinishedAt *time.Time
	FinalizeStartedAt  *time.Time
	FinalizeFinishedAt *time.Time
}

type providerCallOutput struct {
	Result   string
	Outputs  []tasks.TaskOutput
	Timeline providerCallTimeline
}

type providerExecutionConfig struct {
	APIType              string
	ProviderEndpointName string
	FeatureSlug          string
	Timeout              time.Duration
	Finalization         providerFinalizationPipeline
	Request              providerRequest
}

func (c *ProviderExecutor) callDrawingAPIWithAssignment(ctx context.Context, account *accounts.ApiAccount, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) (providerCallOutput, error) {
	correlationID := task.EnsureCorrelationID()
	slog.Info("calling provider API", "account_name", account.Name, "provider_endpoint_id", account.ProviderEndpointID, "task_id", task.TaskID, "correlation_id", correlationID)

	exec, err := c.prepareProviderExecution(ctx, account, task, assignment)
	if err != nil {
		return providerCallOutput{}, err
	}
	output := providerCallOutput{}
	providerStartedAt := time.Now()
	output.Timeline.ProviderStartedAt = &providerStartedAt
	callResult, err := c.callProviderBatchWithRequest(ctx, account, exec)
	providerFinishedAt := time.Now()
	output.Timeline.ProviderFinishedAt = &providerFinishedAt
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return output, err
		}
		return output, newExecutionFailureError(
			tasks.ExecutionStageProvider,
			providerFailureCategory(err),
			fmt.Errorf("调用 API 失败 (服务端点: %s): %w", exec.ProviderEndpointName, err),
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
	imageURLs, taskOutputs, err := exec.Finalization.finalize(ctx, task.UserID, task.TaskID, imageURLs, exec.Timeout)
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

	result := baseProviderTaskResult(task, account, exec, callResult, imageURLs, parsedOutputStatusValid)
	result["status"] = tasks.ResultStatusSuccess
	result["image_url"] = imageURL
	result["image_urls"] = imageURLs
	if strings.TrimSpace(callResult.Model) != "" {
		result["model"] = strings.TrimSpace(callResult.Model)
	}

	resultJSON, err := marshalProviderTaskResult(task.TaskID, result)
	if err != nil {
		return providerCallOutput{}, err
	}
	output.Result = resultJSON
	output.Outputs = taskOutputs
	return output, nil
}

func baseProviderTaskResult(task *tasks.TaskPayload, account *accounts.ApiAccount, exec providerExecutionConfig, callResult providerCallResult, imageURLs []string, parsedStatus string) map[string]interface{} {
	result := map[string]interface{}{
		"task_id":                task.TaskID,
		"correlation_id":         task.EffectiveCorrelationID(),
		"account_id":             account.ID,
		"provider_endpoint_id":   account.ProviderEndpointID,
		"provider_endpoint_name": exec.ProviderEndpointName,
		"api_type":               exec.APIType,
	}
	result["parsed_output_status"] = parsedStatus
	result["parsed_output"] = map[string]interface{}{
		"kind":        resolvedProviderParsedOutputKind(callResult),
		"image_count": resolvedProviderParsedOutputCount(callResult, imageURLs),
	}
	if strings.TrimSpace(callResult.RawOutputPreview) != "" {
		result["raw_provider_output_preview"] = strings.TrimSpace(callResult.RawOutputPreview)
	}
	return result
}

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

func (c *ProviderExecutor) prepareProviderExecution(ctx context.Context, account *accounts.ApiAccount, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) (providerExecutionConfig, error) {
	if account.ApiKey == "" {
		return providerExecutionConfig{}, newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryAuth,
			fmt.Errorf("账号 %s 的 API Key 为空", account.Name),
		)
	}

	providerEndpointName, err := c.applyAssignmentEndpointSnapshot(ctx, account, assignment)
	if err != nil {
		return providerExecutionConfig{}, err
	}
	if providerEndpointName == "" {
		providerEndpointName, err = c.applyAccountProviderEndpoint(ctx, account)
		if err != nil {
			return providerExecutionConfig{}, err
		}
	}

	taskParams := c.parseAndNormalizeTaskParams(ctx, task)
	apiType, ok := workflow.ValidateProviderAPIType(account.ApiType, account.ProxyURL)
	if !ok {
		return providerExecutionConfig{}, newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryInputInvalid,
			fmt.Errorf("账号 %s 的接口类型不支持: %s", account.Name, account.ApiType),
		)
	}

	timeout := resolveAccountTimeout(account)
	finalization, finalizationErr := c.prepareProviderFinalization(
		ctx,
		account,
		taskParams.Provider,
		timeout,
		task.EffectiveCorrelationID(),
	)
	if finalizationErr != nil {
		return providerExecutionConfig{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, finalizationErr)
	}

	resolvedAccountModel, modelOK := resolveAccountModelForExecution(account, taskParams, assignment.ResolvedModel)
	resolvedModel := strings.TrimSpace(resolvedAccountModel.Name)
	modelRequired := strings.TrimSpace(account.Protocol) != providerendpoints.ProtocolImageURLJSONImage
	if modelRequired && (!modelOK || resolvedModel == "") {
		return providerExecutionConfig{}, newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryInputInvalid,
			fmt.Errorf("账号 %s 未分配模型，请先在后台检测并选择模型", account.Name),
		)
	}

	return providerExecutionConfig{
		APIType:              apiType,
		ProviderEndpointName: providerEndpointName,
		FeatureSlug:          taskParams.featureSlug(),
		Timeout:              timeout,
		Finalization:         finalization,
		Request:              taskParams.providerRequest(task.Prompt, resolvedModel),
	}, nil
}

func (c *ProviderExecutor) applyAccountProviderEndpoint(ctx context.Context, account *accounts.ApiAccount) (string, error) {
	if account.ProviderEndpointID <= 0 || c.providerEndpointRepo == nil {
		return "", nil
	}
	providerEndpoint, err := c.providerEndpointRepo.GetByID(ctx, account.ProviderEndpointID)
	if err != nil {
		return "", newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryInputInvalid,
			fmt.Errorf("ProviderEndpoint(ID=%d)不存在: %w", account.ProviderEndpointID, err),
		)
	}
	proxyURL, validateErr := outbound.ValidateConfiguredEndpoint(providerEndpoint.BaseURL, outbound.AllowHosts(ctx)...)
	if validateErr != nil {
		return "", newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryInputInvalid,
			fmt.Errorf("ProviderEndpoint API URL 不可用"),
		)
	}
	account.ProxyURL = proxyURL
	slog.Info("using provider endpoint base URL", "provider_endpoint_name", providerEndpoint.Name, "provider_endpoint_id", providerEndpoint.ID, "base_url", redactLogURL(providerEndpoint.BaseURL))
	return providerEndpoint.Name, nil
}

func (c *ProviderExecutor) applyAssignmentEndpointSnapshot(ctx context.Context, account *accounts.ApiAccount, assignment taskqueue.TaskAssignment) (string, error) {
	baseURL := strings.TrimSpace(assignment.ProviderEndpointBaseURL)
	apiType := strings.TrimSpace(assignment.APIType)
	if assignment.ProviderEndpointID <= 0 || baseURL == "" || apiType == "" {
		return "", nil
	}
	proxyURL, validateErr := outbound.ValidateConfiguredEndpoint(baseURL, outbound.AllowHosts(ctx)...)
	if validateErr != nil {
		return "", newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryInputInvalid,
			fmt.Errorf("ProviderEndpoint snapshot URL 不可用"),
		)
	}
	account.ProviderEndpointID = assignment.ProviderEndpointID
	account.ApiType = apiType
	account.ProxyURL = proxyURL
	name := strings.TrimSpace(assignment.ProviderEndpointName)
	if name == "" {
		name = fmt.Sprintf("ProviderEndpoint#%d", assignment.ProviderEndpointID)
	}
	slog.Info("using assignment provider endpoint snapshot", "provider_endpoint_name", name, "provider_endpoint_id", assignment.ProviderEndpointID, "base_url", redactLogURL(baseURL))
	return name, nil
}

func (c *ProviderExecutor) parseAndNormalizeTaskParams(ctx context.Context, task *tasks.TaskPayload) providerTaskParams {
	typedParams, err := tasks.UnmarshalParams(task.Params)
	if err != nil {
		slog.Warn("parse task params failed", "task_id", task.TaskID, "err", err)
		return providerTaskParams{}
	}
	params := newProviderTaskParams(typedParams)
	c.normalizeUpstreamMediaParams(ctx, params.Provider)
	return params
}

func (c *ProviderExecutor) callProviderWithRequest(ctx context.Context, account *accounts.ApiAccount, exec providerExecutionConfig) (providerCallResult, error) {
	if account == nil {
		return providerCallResult{}, fmt.Errorf("provider account 未配置")
	}
	if strings.TrimSpace(account.Protocol) == providerendpoints.ProtocolImageURLJSONImage {
		switch exec.FeatureSlug {
		case workflow.FeatureSlugRemoveBg:
			return c.callBackgroundRemoval(ctx, account, exec.Request, exec.Timeout)
		case workflow.FeatureSlugHDUpscale:
			return c.callImageUpscale(ctx, account, exec.Request, exec.Timeout)
		default:
			return providerCallResult{}, fmt.Errorf("image-url-json-to-image 不支持功能 %q", exec.FeatureSlug)
		}
	}
	if workflow.IsGoogleNativeAPIType(exec.APIType) {
		return c.callGoogleProviderWithRequest(ctx, account, exec.APIType, exec.Request, exec.Timeout)
	}
	// OpenAI-compatible providers use the DALL-E style image endpoints.
	return c.callOpenAIWithRequest(ctx, account, exec.Request, exec.Timeout)
}

func (c *ProviderExecutor) callProviderBatchWithRequest(ctx context.Context, account *accounts.ApiAccount, exec providerExecutionConfig) (providerCallResult, error) {
	if account == nil {
		return providerCallResult{}, fmt.Errorf("provider account 未配置")
	}
	if strings.TrimSpace(account.Protocol) == providerendpoints.ProtocolImageURLJSONImage {
		exec.Request = exec.Request.withOutputCount(1)
		return c.callProviderWithRequest(ctx, account, exec)
	}
	orchestrator := newProviderBatchOrchestrator(func(callCtx context.Context, request providerRequest) (providerCallResult, error) {
		current := exec
		current.Request = request
		return c.callProviderWithRequest(callCtx, account, current)
	})
	return orchestrator.run(ctx, exec.Request)
}

func marshalProviderTaskResult(taskID string, result map[string]interface{}) (string, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryFinalizeError,
			fmt.Errorf("序列化任务结果失败 (task_id=%s): %w", taskID, err),
		)
	}
	return string(resultJSON), nil
}

func resolveAccountModelForExecution(account *accounts.ApiAccount, _ providerTaskParams, assignedModel string) (accounts.AccountModel, bool) {
	assignedModel = strings.TrimSpace(assignedModel)
	if assignedModel == "" {
		return accounts.AccountModel{}, false
	}
	return accounts.ResolveAccountModel(account, assignedModel)
}

func (c *ProviderExecutor) materializeProviderResultImages(ctx context.Context, userID int64, taskID string, correlationID string, account *accounts.ApiAccount, imageURLs []string) ([]string, []tasks.TaskOutput, error) {
	if len(imageURLs) == 0 {
		return imageURLs, nil, nil
	}
	if c.resultMaterializer == nil {
		return nil, nil, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("持久化上游图片失败: 存储服务未初始化"),
		)
	}

	correlationID = tasks.NormalizeCorrelationID(correlationID)
	if correlationID == "" {
		correlationID = tasks.CorrelationIDForTaskID(taskID)
	}
	slog.Info("provider result materialization started", "task_id", taskID, "correlation_id", correlationID, "image_count", len(imageURLs))

	materialized, storeErr := c.resultMaterializer.MaterializeRemoteResults(ctx, userID, taskID, imageURLs, taskmedia.ResultMaterializeOptions{
		AllowHosts: trustedImageDownloadHosts(ctx, account, c.publicOrigins),
	})
	if storeErr != nil {
		if errors.Is(storeErr, context.Canceled) {
			return nil, nil, storeErr
		}
		return nil, nil, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("持久化上游图片失败: %w", storeErr),
		)
	}
	if len(materialized.Items) == 0 {
		return nil, nil, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("持久化上游图片失败: materializer returned no image URLs"),
		)
	}
	if len(materialized.Items) != len(imageURLs) {
		return nil, nil, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("持久化上游图片失败: materializer returned inconsistent result counts: source=%d items=%d", len(imageURLs), len(materialized.Items)),
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

func resolveAccountTimeout(account *accounts.ApiAccount) time.Duration {
	if account == nil {
		return time.Duration(accountlimits.NormalizeTimeoutSeconds(0)) * time.Second
	}
	timeoutSeconds := accountlimits.NormalizeTimeoutSeconds(account.Timeout)
	return time.Duration(timeoutSeconds) * time.Second
}
