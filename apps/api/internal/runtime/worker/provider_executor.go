package worker

import (
	"api/internal/platform/outbound"
	"api/internal/runtime/queuetypes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	log.Printf("[Worker] 调用 API, 账号: %s, ChannelID: %d",
		account.Name, account.ChannelID)

	if account.ApiKey == "" {
		return "", fmt.Errorf("账号 %s 的 API Key 为空", account.Name)
	}

	// 如果账号绑定了渠道商，使用渠道商的 API URL 作为 ProxyURL
	channelName := ""
	if account.ChannelID > 0 && c.channelRepo != nil {
		channel, err := c.channelRepo.GetByID(account.ChannelID)
		if err != nil {
			return "", fmt.Errorf("渠道商(ID=%d)不存在: %v", account.ChannelID, err)
		}
		proxyURL, validateErr := outbound.ValidateConfiguredEndpoint(channel.ApiURL, outbound.AllowHostsFromEnv()...)
		if validateErr != nil {
			return "", fmt.Errorf("渠道商 API URL 不可用")
		}
		account.ProxyURL = proxyURL
		channelName = channel.Name
		log.Printf("[Worker] 使用渠道商「%s」API URL: %s", channel.Name, channel.ApiURL)
	}

	// 解析任务参数（ratio, count 等）
	var params map[string]interface{}
	if task.Params != "" {
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			log.Printf("[Worker] 解析任务参数失败: task=%s err=%v", task.TaskID, err)
		}
	}
	c.normalizeUpstreamMediaParams(params)
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")

	if workflow.IsStabilityTextToImagePlatform(account.Platform) && workflow.IsReferenceImageFeature(featureSlug) {
		return "", fmt.Errorf("当前渠道不支持该图像参考能力")
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

	platform := strings.ToLower(strings.TrimSpace(account.Platform))
	resolvedModel := resolveRequestedModel(account.ModelSet, params)
	switch {
	case workflow.IsStabilityTextToImagePlatform(platform):
		callResult, err = c.callStabilityAI(ctx, account, task.Prompt, width, height, timeout, resolvedModel)
	default:
		// nano banana / openai / gemini 等 OpenAI 兼容平台（官方原生格式）
		callResult, err = c.callOpenAI(ctx, account, task.Prompt, width, height, timeout, params, resolvedModel)
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
		imageURL, localRedrawStored, err = c.finalizeLocalRedrawResult(ctx, localRedrawCtx, imageURL, timeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			return "", err
		}
		imageURLs = []string{imageURL}
	}

	// 如果对象存储已启用，将图片转存到当前存储
	log.Printf("[Worker] 存储检查: service=%v, objectStorage=%v", c.storageService != nil, c.storageService != nil && c.storageService.UsesObjectStorage())
	if !localRedrawStored && c.storageService != nil && c.storageService.UsesObjectStorage() {
		log.Printf("[Worker] 对象存储已启用，开始转存图片, directFetch=%v, imageCount=%d", c.storageService.IsDirectFetchEnabled(), len(imageURLs))
		for index, currentURL := range imageURLs {
			reURL, reErr := c.storageService.StoreRemote(ctx, currentURL, storage.StorageStoreOptions{
				AllowHosts: []string{account.ProxyURL},
			})
			if reErr == nil {
				log.Printf("[Worker] 转存成功: %s -> %s", currentURL[:min(80, len(currentURL))], reURL)
				imageURLs[index] = reURL
			} else if errors.Is(reErr, context.Canceled) {
				return "", reErr
			} else {
				log.Printf("[Worker] 转存对象存储失败，使用原始 URL: %v", reErr)
			}
		}
		imageURL = imageURLs[0]
	} else {
		log.Printf("[Worker] 对象存储未启用，跳过转存")
	}

	result := map[string]interface{}{
		"task_id":      task.TaskID,
		"account_id":   account.ID,
		"channel_id":   account.ChannelID,
		"channel_name": channelName,
		"platform":     account.Platform,
		"api_type":     account.ApiType,
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
