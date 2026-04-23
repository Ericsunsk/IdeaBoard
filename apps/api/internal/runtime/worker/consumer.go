package worker

import (
	"api/internal/accounts"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"api/internal/channels"
	"api/internal/platform/redis"
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

type Consumer struct {
	accountRepo    *accounts.AccountRepository
	channelRepo    *channels.ChannelRepository
	taskRepo       *accounts.TaskRepository
	redisMgr       *redis.RedisManager
	storageService *storage.StorageService
	taskRuntime    *accounts.TaskRuntimeService
	orchestrator   *taskExecutionOrchestrator

	wg      sync.WaitGroup
	stopCh  chan struct{}
	running bool
	mu      sync.Mutex
}

func NewConsumer(
	accountRepo *accounts.AccountRepository,
	channelRepo *channels.ChannelRepository,
	taskRepo *accounts.TaskRepository,
	redisMgr *redis.RedisManager,
	storageService *storage.StorageService,
) *Consumer {
	taskRuntime := accounts.NewTaskRuntimeService(accountRepo, taskRepo, redisMgr)
	consumer := &Consumer{
		accountRepo:    accountRepo,
		channelRepo:    channelRepo,
		taskRepo:       taskRepo,
		redisMgr:       redisMgr,
		storageService: storageService,
		taskRuntime:    taskRuntime,
		stopCh:         make(chan struct{}),
	}
	consumer.orchestrator = newTaskExecutionOrchestrator(accountRepo, redisMgr, taskRuntime, consumer.callDrawingAPI)
	return consumer
}

func (c *Consumer) Start(workerCount int) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	log.Printf("[Consumer] 启动 %d 个工作协程", workerCount)

	for i := 0; i < workerCount; i++ {
		c.wg.Add(1)
		go c.worker(i)
	}
}

func (c *Consumer) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	c.mu.Unlock()

	log.Println("[Consumer] 停止中...")
	close(c.stopCh)
	c.wg.Wait()
	log.Println("[Consumer] 已停止")
}

func (c *Consumer) worker(id int) {
	defer c.wg.Done()
	log.Printf("[Consumer] 协程 %d 启动", id)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			log.Printf("[Consumer] 协程 %d 退出", id)
			return
		case <-ticker.C:
			c.processQueue()
		}
	}
}

func (c *Consumer) processQueue() {
	ctx := context.Background()
	c.promoteWaitQueue(ctx)

	task, err := c.redisMgr.PopGlobalQueue(ctx)
	if err != nil {
		log.Printf("[Consumer] 拉取任务失败: %v", err)
		return
	}
	if task == nil {
		return
	}

	log.Printf("[Consumer] 拉到任务: %s", task.TaskID)

	if err := c.taskRuntime.RefreshTaskAccounts(task); err != nil {
		log.Printf("[Consumer] 刷新任务账号失败: %v", err)
		c.failTask(task, 0, "刷新可用账号失败: "+err.Error())
		return
	}

	if len(task.AccountIDs) == 0 {
		log.Printf("[Consumer] 没有可用账号: %s", task.TaskID)
		c.failTask(task, 0, "没有可用账号")
		return
	}

	accountID, status, err := c.redisMgr.AssignTask(ctx, task.AccountIDs, task)
	if err != nil {
		log.Printf("[Consumer] 分配任务失败: %v", err)
		c.failTask(task, 0, "分配失败: "+err.Error())
		return
	}

	if status == tasks.QueueAssignStatusQueueFull {
		log.Printf("[Consumer] 等待队列满，将任务放回: %s", task.TaskID)
		c.redisMgr.PushGlobalQueueFront(ctx, task)
		return
	}

	if status == tasks.QueueAssignStatusWaitQueue {
		log.Printf("[Consumer] 任务进入等待队列: %s", task.TaskID)
		return
	}

	go c.orchestrator.execute(task, accountID)
}

func (c *Consumer) promoteWaitQueue(ctx context.Context) {
	task, err := c.redisMgr.PopWaitQueue(ctx)
	if err != nil {
		log.Printf("[Consumer] 拉取等待队列失败: %v", err)
		return
	}
	if task == nil {
		return
	}
	if err := c.redisMgr.PushGlobalQueueFront(ctx, task); err != nil {
		log.Printf("[Consumer] 回流等待队列失败: %v", err)
	}
}

func (c *Consumer) failTask(task *redis.GlobalQueueTask, accountID uint64, reason string) {
	if err := c.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := c.taskRuntime.MarkTaskFailed(task.TaskID, reason); err != nil {
		log.Printf("[Consumer] 标记任务失败失败: task=%s err=%v", task.TaskID, err)
	}
	log.Printf("[Consumer] 任务 %s 失败, 原因: %s", task.TaskID, reason)
}

func resolvePublicAssetBaseURL() *url.URL {
	for _, key := range []string{"APP_PUBLIC_BASE_URL", "APP_BASE_URL", "PUBLIC_BASE_URL"} {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			log.Printf("[Consumer] 忽略无效的公网基址配置: %s=%q", key, raw)
			continue
		}
		return parsed
	}
	return nil
}

func resolveLocalUploadFilePath(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "/uploads/") {
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if !strings.HasPrefix(cleaned, "/uploads/") {
		return "", false
	}
	relative := filepath.FromSlash(strings.TrimPrefix(cleaned, "/"))
	candidates := []string{
		relative,
		filepath.Join("apps", "api", relative),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, true
	}
	return "", false
}

func inferLocalAssetMimeType(filename string, data []byte) string {
	if len(data) > 0 {
		detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
		if strings.HasPrefix(detected, "image/") {
			return detected
		}
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}

func inlineLocalAssetDataURL(raw string) (string, bool, error) {
	localPath, ok := resolveLocalUploadFilePath(raw)
	if !ok {
		return "", false, nil
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", true, err
	}
	mimeType := inferLocalAssetMimeType(localPath, data)
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), true, nil
}

func absolutizeAssetURL(base *url.URL, raw string) (string, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "data:") {
		return trimmed, false, false
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return trimmed, false, false
	}
	if !strings.HasPrefix(trimmed, "/") {
		return trimmed, false, false
	}
	if base == nil {
		return trimmed, false, true
	}
	rel, err := url.Parse(trimmed)
	if err != nil {
		return trimmed, false, true
	}
	return base.ResolveReference(rel).String(), true, false
}

func (c *Consumer) normalizeUpstreamMediaParams(params map[string]interface{}) {
	if len(params) == 0 {
		return
	}

	base := resolvePublicAssetBaseURL()
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")
	converted := 0
	inlined := 0
	unresolvedKeys := map[string]struct{}{}

	normalizeStringKey := func(key string) {
		raw, ok := params[key]
		if !ok {
			return
		}
		value, ok := raw.(string)
		if !ok {
			return
		}
		normalized, changed, unresolved := absolutizeAssetURL(base, value)
		if changed {
			params[key] = normalized
			converted++
			return
		}
		if unresolved {
			if inlinedValue, handled, err := inlineLocalAssetDataURL(value); handled {
				if err != nil {
					log.Printf("[Consumer] 本地媒体内联失败: feature=%s key=%s err=%v", featureSlug, key, err)
				} else {
					params[key] = inlinedValue
					inlined++
					return
				}
			}
		}
		if unresolved {
			unresolvedKeys[key] = struct{}{}
		}
	}

	normalizeListKey := func(key string) {
		raw, ok := params[key]
		if !ok {
			return
		}
		switch values := raw.(type) {
		case []string:
			updated := false
			for index, item := range values {
				normalized, changed, unresolved := absolutizeAssetURL(base, item)
				if changed {
					values[index] = normalized
					updated = true
					converted++
					continue
				}
				if unresolved {
					if inlinedValue, handled, err := inlineLocalAssetDataURL(item); handled {
						if err != nil {
							log.Printf("[Consumer] 本地媒体内联失败: feature=%s key=%s index=%d err=%v", featureSlug, key, index, err)
						} else {
							values[index] = inlinedValue
							updated = true
							inlined++
							continue
						}
					}
				}
				if unresolved {
					unresolvedKeys[key] = struct{}{}
				}
			}
			if updated {
				params[key] = values
			}
		case []interface{}:
			updated := false
			for index, item := range values {
				text, ok := item.(string)
				if !ok {
					continue
				}
				normalized, changed, unresolved := absolutizeAssetURL(base, text)
				if changed {
					values[index] = normalized
					updated = true
					converted++
					continue
				}
				if unresolved {
					if inlinedValue, handled, err := inlineLocalAssetDataURL(text); handled {
						if err != nil {
							log.Printf("[Consumer] 本地媒体内联失败: feature=%s key=%s index=%d err=%v", featureSlug, key, index, err)
						} else {
							values[index] = inlinedValue
							updated = true
							inlined++
							continue
						}
					}
				}
				if unresolved {
					unresolvedKeys[key] = struct{}{}
				}
			}
			if updated {
				params[key] = values
			}
		}
	}

	for _, key := range []string{"image_url", "original_image_url", "mask_image_url", "mask_url", "model_url", "pose_url"} {
		normalizeStringKey(key)
	}
	for _, key := range []string{"image_urls", "mask_image_urls", "mask_urls"} {
		normalizeListKey(key)
	}

	if converted > 0 {
		log.Printf("[Consumer] 已补全上游媒体地址: feature=%s converted=%d", featureSlug, converted)
	}
	if inlined > 0 {
		log.Printf("[Consumer] 已内联本地媒体到上游请求: feature=%s inlined=%d", featureSlug, inlined)
	}
	if len(unresolvedKeys) > 0 {
		keys := make([]string, 0, len(unresolvedKeys))
		for key := range unresolvedKeys {
			keys = append(keys, key)
		}
		log.Printf("[Consumer] 上游媒体地址仍为相对路径，请配置 APP_PUBLIC_BASE_URL: feature=%s keys=%s", featureSlug, strings.Join(keys, ","))
	}
}

func (c *Consumer) callDrawingAPI(ctx context.Context, account *accounts.ApiAccount, task *redis.GlobalQueueTask) (string, error) {
	log.Printf("[Consumer] 调用 API, 账号: %s, ChannelID: %d, ApiKey长度: %d",
		account.Name, account.ChannelID, len(account.ApiKey))

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
		account.ProxyURL = channel.ApiURL
		channelName = channel.Name
		log.Printf("[Consumer] 使用渠道商「%s」API URL: %s", channel.Name, channel.ApiURL)
	}

	// 解析任务参数（ratio, count 等）
	var params map[string]interface{}
	if task.Params != "" {
		if err := json.Unmarshal([]byte(task.Params), &params); err != nil {
			log.Printf("[Consumer] 解析任务参数失败: task=%s err=%v", task.TaskID, err)
		}
	}
	c.normalizeUpstreamMediaParams(params)

	// 根据 ratio 计算尺寸
	width, height := parseSizeFromParams(params)

	timeout := time.Duration(account.Timeout) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	var imageURL string
	var err error

	platform := strings.ToLower(account.Platform)
	switch {
	case platform == "stability" || platform == "stable diffusion":
		imageURL, err = c.callStabilityAI(ctx, account, task.Prompt, width, height, timeout)
	default:
		// nano banana / openai / gemini 等 OpenAI 兼容平台（官方原生格式）
		imageURL, err = c.callOpenAI(ctx, account, task.Prompt, width, height, timeout, params)
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("调用 API 失败 (渠道: %s): %w", channelName, err)
	}

	// 如果对象存储已启用，将图片转存到当前存储
	log.Printf("[Consumer] 存储检查: service=%v, objectStorage=%v", c.storageService != nil, c.storageService != nil && c.storageService.UsesObjectStorage())
	if c.storageService != nil && c.storageService.UsesObjectStorage() {
		log.Printf("[Consumer] 对象存储已启用，开始转存图片, directFetch=%v, imageURL=%.80s", c.storageService.IsDirectFetchEnabled(), imageURL)
		reURL, reErr := c.storageService.StoreRemote(ctx, imageURL, storage.StorageStoreOptions{
			AllowHosts: []string{account.ProxyURL},
		})
		if reErr == nil {
			log.Printf("[Consumer] 转存成功: %s -> %s", imageURL[:min(80, len(imageURL))], reURL)
			imageURL = reURL
		} else if errors.Is(reErr, context.Canceled) {
			return "", reErr
		} else {
			log.Printf("[Consumer] 转存对象存储失败，使用原始 URL: %v", reErr)
		}
	} else {
		log.Printf("[Consumer] 对象存储未启用，跳过转存")
	}

	result := map[string]interface{}{
		"task_id":      task.TaskID,
		"account_id":   account.ID,
		"channel_id":   account.ChannelID,
		"channel_name": channelName,
		"platform":     account.Platform,
		"api_type":     account.ApiType,
		"model_set":    account.ModelSet,
		"status":       tasks.ResultStatusSuccess,
		"image_url":    imageURL,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// callOpenAI 调用 OpenAI DALL-E 风格接口（也兼容其他 OpenAI 兼容平台）
func (c *Consumer) callOpenAI(ctx context.Context, account *accounts.ApiAccount, prompt string, width, height int, timeout time.Duration, params map[string]interface{}) (string, error) {
	baseURL := "https://api.openai.com"
	if account.ProxyURL != "" {
		baseURL = strings.TrimRight(account.ProxyURL, "/")
	}

	model := "dall-e-3"
	if account.ModelSet != "" {
		model = account.ModelSet
	}

	size := fmt.Sprintf("%dx%d", width, height)
	useEditEndpoint := shouldUseOpenAIEditEndpoint(params)
	imageInputCount := countOpenAIImageInputs(params)
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")

	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"n":      1,
		"size":   size,
	}

	if useEditEndpoint {
		appendOpenAIEditParams(reqBody, params)
	} else {
		appendOpenAIAllowedParams(reqBody, params)
	}
	finalizeOpenAIRequestBody(reqBody, params)

	bodyJSON, _ := json.Marshal(reqBody)

	// 如果 ProxyURL 已包含 /v1，则不再拼接 /v1
	endpoint := baseURL + "/v1/images/generations"
	if strings.HasSuffix(baseURL, "/v1") {
		endpoint = baseURL + "/images/generations"
	}
	if useEditEndpoint {
		endpoint = baseURL + "/v1/images/edits"
		if strings.HasSuffix(baseURL, "/v1") {
			endpoint = baseURL + "/images/edits"
		}
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+account.ApiKey)

	log.Printf(
		"[Consumer] OpenAI 请求: feature=%s endpoint=%s model=%s size=%s edit=%t images=%d",
		featureSlug,
		endpoint,
		model,
		size,
		useEditEndpoint,
		imageInputCount,
	)

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("读取 API 响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API 返回 %d: %s", resp.StatusCode, truncateErrorBody(string(respBody)))
	}

	return extractOpenAIImageResponse(respBody, params)
}

func appendOpenAIAllowedParams(reqBody map[string]interface{}, params map[string]interface{}) {
	if params == nil {
		return
	}

	// OpenAI 兼容图像接口通常接收单个 image URL，这里显式传首图，避免中转站忽略数组型字段。
	// 同时保留 image_urls，兼容需要读取完整图片列表的上游实现。
	if imageURLs := normalizeOpenAIImageURLStrings(params); len(imageURLs) > 0 {
		reqBody["image"] = imageURLs[0]
		reqBody["image_urls"] = imageURLs
	}

	allowedKeys := []string{
		"variation_strength",
		"angle_spec",
		"background",
		"viewpoint",
		"model_url",
		"model_strength",
		"pose_url",
		"pose_strength",
		"blend_ratio",
		"extend_top",
		"extend_right",
		"extend_bottom",
		"extend_left",
		"image_a_weight",
		"image_b_weight",
		"original_image_url",
		"mask_image_url",
		"mask_image_urls",
		"mask_url",
		"mask_urls",
		"use_mask",
		"mask_enabled",
		"is_masked",
		"masked",
		"local_enhance",
		"is_local_enhance",
		"partial_enhance",
		"is_partial_enhance",
		"fabric_rotation",
		"fabric_scale",
		"fabric_opacity",
		"tile_mode",
		"mode",
		"upscale_mode",
		"upscale_factor",
		"output_format",
		"output_quality",
	}
	for _, key := range allowedKeys {
		if v, ok := params[key]; ok {
			reqBody[key] = v
		}
	}
}

func finalizeOpenAIRequestBody(reqBody map[string]interface{}, params map[string]interface{}) {
	if reqBody == nil || params == nil {
		return
	}
	image, ok := reqBody["image"].(string)
	if !ok || !strings.HasPrefix(image, "data:") {
		return
	}
	switch imageURLs := reqBody["image_urls"].(type) {
	case []string:
		if len(imageURLs) == 1 && imageURLs[0] == image {
			delete(reqBody, "image_urls")
		}
	case []interface{}:
		if len(imageURLs) == 1 {
			if first, ok := imageURLs[0].(string); ok && first == image {
				delete(reqBody, "image_urls")
			}
		}
	}
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")
	if featureSlug != workflow.FeatureSlugStyleInnovate {
		return
	}
	if original, ok := reqBody["original_image_url"].(string); ok && original == image {
		delete(reqBody, "original_image_url")
	}
}

func shouldUseOpenAIEditEndpoint(params map[string]interface{}) bool {
	if len(normalizeOpenAIImageInputs(params)) == 0 {
		return false
	}
	featureSlug, ok := workflow.ReadWorkflowStringParam(params, "feature")
	if !ok {
		return false
	}
	return featureSlug == workflow.FeatureSlugCanvasExtend
}

func countOpenAIImageInputs(params map[string]interface{}) int {
	return len(normalizeOpenAIImageInputs(params))
}

func normalizeOpenAIImageURLStrings(params map[string]interface{}) []string {
	refs := normalizeOpenAIImageInputs(params)
	if len(refs) == 0 {
		return nil
	}
	urls := make([]string, 0, len(refs))
	for _, ref := range refs {
		url := strings.TrimSpace(ref["image_url"])
		if url == "" {
			continue
		}
		urls = append(urls, url)
	}
	return urls
}

func normalizeOpenAIImageInputs(params map[string]interface{}) []map[string]string {
	if params == nil {
		return nil
	}
	raw, ok := params["image_urls"]
	if !ok {
		return nil
	}

	var refs []map[string]string
	appendRef := func(url string) {
		trimmed := strings.TrimSpace(url)
		if trimmed == "" {
			return
		}
		refs = append(refs, map[string]string{"image_url": trimmed})
	}

	switch urls := raw.(type) {
	case []string:
		for _, url := range urls {
			appendRef(url)
		}
	case []interface{}:
		for _, item := range urls {
			if url, ok := item.(string); ok {
				appendRef(url)
			}
		}
	}
	return refs
}

func appendOpenAIEditParams(reqBody map[string]interface{}, params map[string]interface{}) {
	if params == nil {
		return
	}
	if refs := normalizeOpenAIImageInputs(params); len(refs) > 0 {
		reqBody["images"] = refs
	}
	if maskURL, ok := workflow.ReadWorkflowStringParam(params, "mask_image_url"); ok {
		reqBody["mask"] = map[string]string{"image_url": maskURL}
	}

	allowedKeys := []string{
		"variation_strength",
		"angle_spec",
		"background",
		"viewpoint",
		"model_url",
		"model_strength",
		"pose_url",
		"pose_strength",
		"blend_ratio",
		"extend_top",
		"extend_right",
		"extend_bottom",
		"extend_left",
		"image_a_weight",
		"image_b_weight",
		"original_image_url",
		"mask_image_urls",
		"mask_url",
		"mask_urls",
		"use_mask",
		"mask_enabled",
		"is_masked",
		"masked",
		"local_enhance",
		"is_local_enhance",
		"partial_enhance",
		"is_partial_enhance",
		"fabric_rotation",
		"fabric_scale",
		"fabric_opacity",
		"tile_mode",
		"mode",
		"upscale_mode",
		"upscale_factor",
		"output_format",
		"output_quality",
	}
	for _, key := range allowedKeys {
		if v, ok := params[key]; ok {
			reqBody[key] = v
		}
	}
}

func extractOpenAIImageResponse(respBody []byte, params map[string]interface{}) (string, error) {
	var respData struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	if len(respData.Data) == 0 {
		return "", fmt.Errorf("API 未返回图片")
	}
	if strings.TrimSpace(respData.Data[0].URL) != "" {
		return strings.TrimSpace(respData.Data[0].URL), nil
	}
	if strings.TrimSpace(respData.Data[0].B64JSON) != "" {
		return "data:" + inferOpenAIImageMimeType(params) + ";base64," + strings.TrimSpace(respData.Data[0].B64JSON), nil
	}
	return "", fmt.Errorf("API 未返回可用图片")
}

func inferOpenAIImageMimeType(params map[string]interface{}) string {
	if params != nil {
		if format, ok := params["output_format"].(string); ok {
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "jpg", "jpeg":
				return "image/jpeg"
			case "webp":
				return "image/webp"
			}
		}
	}
	return "image/png"
}

// callStabilityAI 调用 Stability AI 接口
func (c *Consumer) callStabilityAI(ctx context.Context, account *accounts.ApiAccount, prompt string, width, height int, timeout time.Duration) (string, error) {
	baseURL := "https://api.stability.ai"
	if account.ProxyURL != "" {
		baseURL = account.ProxyURL
	}

	engine := "stable-diffusion-xl-1024-v1-0"
	if account.ModelSet != "" {
		engine = account.ModelSet
	}

	reqBody := map[string]interface{}{
		"text_prompts": []map[string]interface{}{
			{"text": prompt, "weight": 1},
		},
		"width":   width,
		"height":  height,
		"samples": 1,
	}
	bodyJSON, _ := json.Marshal(reqBody)

	client := &http.Client{Timeout: timeout}
	url := fmt.Sprintf("%s/v1/generation/%s/text-to-image", baseURL, engine)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+account.ApiKey)
	req.Header.Set("Accept", "application/json")

	log.Printf("[Consumer] StabilityAI 请求: engine=%s, %dx%d", engine, width, height)

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("读取 API 响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API 返回 %d: %s", resp.StatusCode, truncateErrorBody(string(respBody)))
	}

	var respData struct {
		Artifacts []struct {
			Base64 string `json:"base64"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	if len(respData.Artifacts) == 0 {
		return "", fmt.Errorf("API 未返回图片")
	}

	// 返回 base64 数据 URL
	return "data:image/png;base64," + respData.Artifacts[0].Base64, nil
}

// parseSizeFromParams 从任务参数中解析图片尺寸
func parseSizeFromParams(params map[string]interface{}) (int, int) {
	ratio, _ := params["ratio"].(string)
	quality, _ := params["quality"].(string)

	var w, h int

	// 自定义尺寸优先
	if cw, ok := params["custom_width"].(float64); ok && cw > 0 {
		if ch, okh := params["custom_height"].(float64); okh && ch > 0 {
			w, h = int(cw), int(ch)
			// 根据清晰度缩放
			switch quality {
			case "2k":
				w, h = w*2, h*2
			case "4k":
				w, h = w*4, h*4
			}
			return w, h
		}
	}

	switch ratio {
	case "3:2":
		w, h = 1536, 1024
	case "16:9":
		w, h = 1792, 1024
	case "1:1":
		w, h = 1024, 1024
	case "2:3":
		w, h = 1024, 1536
	case "3:4":
		w, h = 1024, 1365
	case "9:16":
		w, h = 1024, 1792
	default:
		w, h = 1024, 1024
	}

	// 根据清晰度缩放
	switch quality {
	case "2k":
		w, h = w*2, h*2
	case "4k":
		w, h = w*4, h*4
	}

	return w, h
}
