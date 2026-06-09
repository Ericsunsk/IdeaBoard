package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/runtime/taskqueue"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/channels"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/workflow"
)

const (
	// maxAPIResponseBody caps upstream API reads to prevent OOM from malicious or broken streams.
	maxAPIResponseBody = 50 * 1024 * 1024 // 50 MB
	// maxErrorBodyLen caps error messages stored in task records.
	maxErrorBodyLen = 500
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

func truncateErrorBody(body string) string {
	if len(body) > maxErrorBodyLen {
		return body[:maxErrorBodyLen] + "...(truncated)"
	}
	return body
}

type ProviderExecutor struct {
	channelRepo    *channels.ChannelRepository
	storageService *storage.StorageService
}

type providerCallResult struct {
	ImageURL  string
	ImageURLs []string
	Model     string
}

type providerCallTimeline struct {
	ProviderStartedAt  *time.Time
	ProviderFinishedAt *time.Time
	FinalizeStartedAt  *time.Time
	FinalizeFinishedAt *time.Time
}

type providerCallOutput struct {
	Result   string
	Timeline providerCallTimeline
}

func (c *ProviderExecutor) callDrawingAPI(ctx context.Context, account *accounts.ApiAccount, task *queuetypes.TaskPayload) (providerCallOutput, error) {
	return c.callDrawingAPIWithAssignment(ctx, account, task, taskqueue.TaskAssignment{})
}

func (c *ProviderExecutor) callDrawingAPIWithAssignment(ctx context.Context, account *accounts.ApiAccount, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment) (providerCallOutput, error) {
	slog.Info("calling provider API", "account_name", account.Name, "channel_id", account.ChannelID, "task_id", task.TaskID)

	if account.ApiKey == "" {
		return providerCallOutput{}, newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryAuth,
			fmt.Errorf("账号 %s 的 API Key 为空", account.Name),
		)
	}

	// 如果账号绑定了渠道商，使用渠道商的 API URL 作为 ProxyURL
	channelName := ""
	if account.ChannelID > 0 && c.channelRepo != nil {
		channel, err := c.channelRepo.GetByID(ctx, account.ChannelID)
		if err != nil {
			return providerCallOutput{}, newExecutionFailureError(
				tasks.ExecutionStageAssigned,
				tasks.FailureCategoryInputInvalid,
				fmt.Errorf("渠道商(ID=%d)不存在: %v", account.ChannelID, err),
			)
		}
		proxyURL, validateErr := outbound.ValidateConfiguredEndpoint(channel.ApiURL, outbound.AllowHostsFromEnv()...)
		if validateErr != nil {
			return providerCallOutput{}, newExecutionFailureError(
				tasks.ExecutionStageAssigned,
				tasks.FailureCategoryInputInvalid,
				fmt.Errorf("渠道商 API URL 不可用"),
			)
		}
		account.ProxyURL = proxyURL
		channelName = channel.Name
		slog.Info("using channel API URL", "channel_name", channel.Name, "channel_id", channel.ID, "api_url", redactLogURL(channel.ApiURL))
	}

	// 解析任务参数（ratio, count 等）
	var params map[string]interface{}
	if task.Params != "" {
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			slog.Warn("parse task params failed", "task_id", task.TaskID, "err", err)
		}
	}
	c.normalizeUpstreamMediaParams(params)

	apiType, ok := workflow.ValidateProviderAPIType(account.ApiType, account.ProxyURL)
	if !ok {
		return providerCallOutput{}, newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryInputInvalid,
			fmt.Errorf("账号 %s 的接口类型不支持: %s", account.Name, account.ApiType),
		)
	}

	timeout := resolveAccountTimeout(account)

	localRedrawCtx, redrawErr := c.prepareLocalRedrawPipeline(ctx, account, params, timeout)
	if redrawErr != nil {
		return providerCallOutput{}, newExecutionFailureError(tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid, redrawErr)
	}
	localRedrawStored := false

	// 根据 ratio 或自定义尺寸计算输出尺寸；local redraw 会先改写为 ROI 尺寸。
	width, height := parseSizeFromParams(params)

	var callResult providerCallResult
	var err error

	featureSlug := ""
	if params != nil {
		if slug, err := workflow.ResolveWorkflowFeatureSlug(params); err == nil {
			featureSlug = slug
		}
	}
	resolvedAccountModel, ok := resolveAccountModelForExecution(account, featureSlug, params, assignment.ResolvedModel)
	resolvedModel := strings.TrimSpace(resolvedAccountModel.Name)
	if !ok || resolvedModel == "" {
		return providerCallOutput{}, newExecutionFailureError(
			tasks.ExecutionStageAssigned,
			tasks.FailureCategoryInputInvalid,
			fmt.Errorf("账号 %s 未分配模型，请先在后台检测并选择模型", account.Name),
		)
	}
	providerReq := newProviderRequest(task.Prompt, width, height, params, resolvedModel)
	output := providerCallOutput{}
	providerStartedAt := time.Now()
	output.Timeline.ProviderStartedAt = &providerStartedAt
	switch {
	case workflow.IsGoogleNativeAPIType(apiType):
		callResult, err = c.callGoogleAIWithRequest(ctx, account, providerReq, timeout)
	default:
		// OpenAI-compatible providers use the DALL-E style image endpoints.
		callResult, err = c.callOpenAIWithRequest(ctx, account, providerReq, timeout)
	}
	providerFinishedAt := time.Now()
	output.Timeline.ProviderFinishedAt = &providerFinishedAt

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return output, err
		}
		return output, newExecutionFailureError(
			tasks.ExecutionStageProvider,
			providerFailureCategory(err),
			fmt.Errorf("调用 API 失败 (渠道: %s): %w", channelName, err),
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
	imageURL := imageURLs[0]

	finalizeStartedAt := time.Now()
	output.Timeline.FinalizeStartedAt = &finalizeStartedAt
	if localRedrawCtx != nil {
		imageURL, localRedrawStored, err = c.finalizeLocalRedrawResult(ctx, task.UserID, task.TaskID, localRedrawCtx, imageURL, timeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return output, err
			}
			if _, _, ok := executionFailureDetails(err); ok {
				return output, err
			}
			return output, newExecutionFailureError(tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, err)
		}
		imageURLs = []string{imageURL}
	}

	if !localRedrawStored {
		imageURLs, err = c.materializeProviderResultImages(ctx, task.UserID, task.TaskID, account, imageURLs)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return output, err
			}
			return output, err
		}
		imageURL = imageURLs[0]
	}
	finalizeFinishedAt := time.Now()
	output.Timeline.FinalizeFinishedAt = &finalizeFinishedAt

	result := map[string]interface{}{
		"task_id":      task.TaskID,
		"account_id":   account.ID,
		"channel_id":   account.ChannelID,
		"channel_name": channelName,
		"platform":     account.Platform,
		"api_type":     apiType,
		"status":       tasks.ResultStatusSuccess,
		"image_url":    imageURL,
		"image_urls":   imageURLs,
	}
	if strings.TrimSpace(callResult.Model) != "" {
		result["model"] = strings.TrimSpace(callResult.Model)
	}

	resultJSON, _ := json.Marshal(result)
	output.Result = string(resultJSON)
	return output, nil
}

func resolveAccountModelForExecution(account *accounts.ApiAccount, featureSlug string, params map[string]interface{}, assignedModel string) (accounts.AccountModel, bool) {
	if model := strings.TrimSpace(assignedModel); model != "" {
		return accounts.ResolveAccountModel(account, featureSlug, model)
	}
	return accounts.ResolveAccountModelForParams(account, featureSlug, params)
}

func (c *ProviderExecutor) materializeProviderResultImages(ctx context.Context, userID int64, taskID string, account *accounts.ApiAccount, imageURLs []string) ([]string, error) {
	if len(imageURLs) == 0 {
		return imageURLs, nil
	}
	if c.storageService == nil {
		return nil, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("持久化上游图片失败: 存储服务未初始化"),
		)
	}

	slog.Info("provider result storage check", "object_storage", c.storageService.UsesObjectStorage(ctx), "image_count", len(imageURLs))

	next := append([]string(nil), imageURLs...)
	for index, currentURL := range next {
		storeOpts := storage.StorageStoreOptions{
			AllowHosts: trustedImageDownloadHosts(account),
		}
		if dirPrefix, err := storage.TaskOutputUploadDirPrefix(userID, taskID); err == nil {
			storeOpts.DirPrefix = dirPrefix
		}
		storedURL, storeErr := c.storageService.StoreRemote(ctx, currentURL, storeOpts)
		if storeErr == nil {
			slog.Info("provider image materialized", "source_url", redactLogURL(currentURL), "stored_url", redactLogURL(storedURL))
			next[index] = storedURL
			continue
		}
		if errors.Is(storeErr, context.Canceled) {
			return nil, storeErr
		}
		return nil, newExecutionFailureError(
			tasks.ExecutionStageFinalizing,
			tasks.FailureCategoryStorageError,
			fmt.Errorf("持久化上游图片失败: %w", storeErr),
		)
	}
	return next, nil
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
		return time.Duration(accounts.NormalizeAccountTimeout(0)) * time.Second
	}
	timeoutSeconds := accounts.NormalizeAccountTimeout(account.Timeout)
	return time.Duration(timeoutSeconds) * time.Second
}
