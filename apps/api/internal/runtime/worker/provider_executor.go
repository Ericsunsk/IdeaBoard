package worker

import (
	"api/internal/platform/outbound"
	"api/internal/runtime/queuetypes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"api/internal/accounts"
	"api/internal/channels"
	"api/internal/storage"
	"api/internal/tasks"
	"api/internal/workflow"
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

func (c *ProviderExecutor) callDrawingAPI(ctx context.Context, account *accounts.ApiAccount, task *queuetypes.TaskPayload) (string, error) {
	slog.Info("calling provider API", "account_name", account.Name, "channel_id", account.ChannelID, "task_id", task.TaskID)

	if account.ApiKey == "" {
		return "", fmt.Errorf("账号 %s 的 API Key 为空", account.Name)
	}

	// 如果账号绑定了渠道商，使用渠道商的 API URL 作为 ProxyURL
	channelName := ""
	if account.ChannelID > 0 && c.channelRepo != nil {
		channel, err := c.channelRepo.GetByID(ctx, account.ChannelID)
		if err != nil {
			return "", fmt.Errorf("渠道商(ID=%d)不存在: %v", account.ChannelID, err)
		}
		proxyURL, validateErr := outbound.ValidateConfiguredEndpoint(channel.ApiURL, outbound.AllowHostsFromEnv()...)
		if validateErr != nil {
			return "", fmt.Errorf("渠道商 API URL 不可用")
		}
		account.ProxyURL = proxyURL
		channelName = channel.Name
		slog.Info("using channel API URL", "channel_name", channel.Name, "channel_id", channel.ID, "api_url", channel.ApiURL)
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
		return "", fmt.Errorf("账号 %s 的接口类型不支持: %s", account.Name, account.ApiType)
	}

	timeout := resolveAccountTimeout(account)

	localRedrawCtx, redrawErr := c.prepareLocalRedrawPipeline(ctx, account, params, timeout)
	if redrawErr != nil {
		return "", redrawErr
	}
	localRedrawStored := false

	// 根据 ratio 或自定义尺寸计算输出尺寸；local redraw 会先改写为 ROI 尺寸。
	width, height := parseSizeFromParams(params)

	var callResult providerCallResult
	var err error

	resolvedModel := resolveRequestedModel(account.ModelSet, params)
	if strings.TrimSpace(resolvedModel) == "" {
		return "", fmt.Errorf("账号 %s 未分配模型，请先在后台检测并选择模型", account.Name)
	}
	providerReq := newProviderRequest(task.Prompt, width, height, params, resolvedModel)
	switch {
	case workflow.IsGoogleNativeAPIType(apiType):
		callResult, err = c.callGoogleAIWithRequest(ctx, account, providerReq, timeout)
	default:
		// OpenAI-compatible providers use the DALL-E style image endpoints.
		callResult, err = c.callOpenAIWithRequest(ctx, account, providerReq, timeout)
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("调用 API 失败 (渠道: %s): %w", channelName, err)
	}
	imageURLs := normalizeProviderResultImageURLs(callResult)
	if len(imageURLs) == 0 {
		return "", fmt.Errorf("API 未返回图片")
	}
	imageURL := imageURLs[0]

	if localRedrawCtx != nil {
		imageURL, localRedrawStored, err = c.finalizeLocalRedrawResult(ctx, task.UserID, task.TaskID, localRedrawCtx, imageURL, timeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			return "", err
		}
		imageURLs = []string{imageURL}
	}

	if !localRedrawStored {
		imageURLs, err = c.materializeProviderResultImages(ctx, task.UserID, task.TaskID, account, imageURLs)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			return "", err
		}
		imageURL = imageURLs[0]
	}

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
	if strings.TrimSpace(account.ModelSet) != "" && !strings.Contains(account.ModelSet, ",") {
		result["model_set"] = strings.TrimSpace(account.ModelSet)
	}
	if strings.TrimSpace(callResult.Model) != "" {
		result["model"] = strings.TrimSpace(callResult.Model)
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

func (c *ProviderExecutor) materializeProviderResultImages(ctx context.Context, userID int64, taskID string, account *accounts.ApiAccount, imageURLs []string) ([]string, error) {
	if len(imageURLs) == 0 {
		return imageURLs, nil
	}
	if c.storageService == nil {
		return imageURLs, nil
	}

	usesObjectStorage := c.storageService.UsesObjectStorage(ctx)
	slog.Info("provider result storage check", "object_storage", usesObjectStorage, "image_count", len(imageURLs))

	next := append([]string(nil), imageURLs...)
	for index, currentURL := range next {
		inlineImage := isInlineDataImageURL(currentURL)
		if !inlineImage && !usesObjectStorage {
			continue
		}

		storeOpts := storage.StorageStoreOptions{
			AllowHosts: []string{account.ProxyURL},
		}
		if dirPrefix, err := storage.TaskOutputUploadDirPrefix(userID, taskID); err == nil {
			storeOpts.DirPrefix = dirPrefix
		}
		storedURL, storeErr := c.storageService.StoreRemote(ctx, currentURL, storeOpts)
		if storeErr == nil {
			slog.Info("provider image materialized", "source_url", truncateLogURL(currentURL), "stored_url", storedURL)
			next[index] = storedURL
			continue
		}
		if errors.Is(storeErr, context.Canceled) {
			return nil, storeErr
		}
		if inlineImage {
			return nil, fmt.Errorf("持久化上游图片失败: %w", storeErr)
		}
		slog.Warn("materialize remote image failed; keeping original URL", "err", storeErr)
	}
	return next, nil
}

func isInlineDataImageURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:image/")
}

func truncateLogURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
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

func resolveRequestedModel(accountModelSet string, params map[string]interface{}) string {
	candidates := splitModelSet(accountModelSet)
	if len(candidates) == 0 {
		return ""
	}
	if modelFromParams, ok := workflow.ReadWorkflowStringParam(params, "model"); ok {
		modelFromParams = strings.TrimSpace(modelFromParams)
		if modelFromParams != "" {
			for _, candidate := range candidates {
				if strings.EqualFold(candidate, modelFromParams) {
					return candidate
				}
			}
		}
	}
	return candidates[0]
}

func splitModelSet(modelSet string) []string {
	items := strings.Split(modelSet, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		model := strings.TrimSpace(item)
		if model == "" {
			continue
		}
		result = append(result, model)
	}
	return result
}

func resolveAccountTimeout(account *accounts.ApiAccount) time.Duration {
	if account == nil {
		return time.Duration(accounts.NormalizeAccountTimeout(0)) * time.Second
	}
	timeoutSeconds := accounts.NormalizeAccountTimeout(account.Timeout)
	return time.Duration(timeoutSeconds) * time.Second
}
