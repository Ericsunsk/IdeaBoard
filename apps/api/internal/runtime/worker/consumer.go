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
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"api/internal/channels"
	"api/internal/platform/redis"
	"api/internal/platform/safehttp"
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
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")

	if workflow.IsStabilityTextToImagePlatform(account.Platform) && workflow.IsReferenceImageFeature(featureSlug) {
		return "", fmt.Errorf("当前渠道不支持该图像参考能力")
	}

	timeout := time.Duration(account.Timeout) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	localRedrawCtx, redrawErr := c.prepareLocalRedrawPipeline(ctx, account, params, timeout)
	if redrawErr != nil {
		return "", redrawErr
	}
	localRedrawStored := false

	// 根据 ratio 或自定义尺寸计算输出尺寸；local redraw 会先改写为 ROI 尺寸。
	width, height := parseSizeFromParams(params)

	var imageURL string
	var err error

	platform := strings.ToLower(strings.TrimSpace(account.Platform))
	switch {
	case workflow.IsStabilityTextToImagePlatform(platform):
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

	if localRedrawCtx != nil {
		imageURL, localRedrawStored, err = c.finalizeLocalRedrawResult(ctx, localRedrawCtx, imageURL, timeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			return "", err
		}
	}

	// 如果对象存储已启用，将图片转存到当前存储
	log.Printf("[Consumer] 存储检查: service=%v, objectStorage=%v", c.storageService != nil, c.storageService != nil && c.storageService.UsesObjectStorage())
	if !localRedrawStored && c.storageService != nil && c.storageService.UsesObjectStorage() {
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
	var req *http.Request
	var err error
	if useEditEndpoint {
		body, contentType, buildErr := buildOpenAIEditMultipartRequestBody(ctx, reqBody, timeout)
		if buildErr != nil {
			return "", buildErr
		}
		req, err = http.NewRequestWithContext(ctx, "POST", endpoint, body)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", contentType)
	} else {
		bodyJSON, _ := json.Marshal(reqBody)
		req, err = http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyJSON))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
	}
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

func buildOpenAIEditMultipartRequestBody(ctx context.Context, reqBody map[string]interface{}, timeout time.Duration) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range reqBody {
		if key == "images" || key == "mask" {
			continue
		}
		if err := writeOpenAIMultipartField(writer, key, value); err != nil {
			return nil, "", err
		}
	}

	imageRefs := extractOpenAIImageRefs(reqBody["images"])
	for index, ref := range imageRefs {
		if err := writeOpenAIMultipartImagePart(ctx, writer, "image", ref, index, timeout); err != nil {
			return nil, "", err
		}
	}
	if len(imageRefs) == 0 {
		return nil, "", fmt.Errorf("OpenAI 编辑请求缺少 image")
	}

	if maskRef := extractOpenAIImageRef(reqBody["mask"]); strings.TrimSpace(maskRef) != "" {
		if err := writeOpenAIMultipartImagePart(ctx, writer, "mask", maskRef, 0, timeout); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &body, writer.FormDataContentType(), nil
}

func writeOpenAIMultipartField(writer *multipart.Writer, key string, value interface{}) error {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return writer.WriteField(key, typed)
	case bool:
		return writer.WriteField(key, strconv.FormatBool(typed))
	case float32:
		return writer.WriteField(key, strconv.FormatFloat(float64(typed), 'f', -1, 32))
	case float64:
		return writer.WriteField(key, strconv.FormatFloat(typed, 'f', -1, 64))
	case int:
		return writer.WriteField(key, strconv.Itoa(typed))
	case int8:
		return writer.WriteField(key, strconv.FormatInt(int64(typed), 10))
	case int16:
		return writer.WriteField(key, strconv.FormatInt(int64(typed), 10))
	case int32:
		return writer.WriteField(key, strconv.FormatInt(int64(typed), 10))
	case int64:
		return writer.WriteField(key, strconv.FormatInt(typed, 10))
	case uint:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint8:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint16:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint32:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint64:
		return writer.WriteField(key, strconv.FormatUint(typed, 10))
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("序列化 multipart 字段失败 %s: %w", key, err)
		}
		return writer.WriteField(key, string(encoded))
	}
}

func writeOpenAIMultipartImagePart(ctx context.Context, writer *multipart.Writer, fieldName, ref string, index int, timeout time.Duration) error {
	filename, mimeType, data, err := resolveOpenAIImageBytes(ctx, strings.TrimSpace(ref), index, timeout)
	if err != nil {
		return err
	}

	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
	headers.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(headers)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}

func resolveOpenAIImageBytes(ctx context.Context, ref string, index int, timeout time.Duration) (filename string, mimeType string, data []byte, err error) {
	if strings.HasPrefix(ref, "data:") {
		decodedMimeType, decodedData, decodeErr := decodeOpenAIDataURL(ref)
		if decodeErr != nil {
			return "", "", nil, fmt.Errorf("解析 data URL 失败: %w", decodeErr)
		}
		ext := inferOpenAIImageFileExtension(decodedMimeType)
		return fmt.Sprintf("image-%d.%s", index+1, ext), decodedMimeType, decodedData, nil
	}

	parsed, parseErr := url.Parse(ref)
	if parseErr != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", nil, fmt.Errorf("不支持的图像引用: %q", ref)
	}

	allowHosts := trustedOpenAIAssetHosts()
	resp, doErr := safehttp.GetWithAllowHosts(ctx, ref, timeout, allowHosts...)
	if doErr != nil {
		return "", "", nil, fmt.Errorf("拉取图像失败: %w", doErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimitedBody(resp.Body)
		return "", "", nil, fmt.Errorf("拉取图像返回 %d: %s", resp.StatusCode, truncateErrorBody(string(body)))
	}

	body, readErr := readLimitedBody(resp.Body)
	if readErr != nil {
		return "", "", nil, readErr
	}
	if len(body) == 0 {
		return "", "", nil, fmt.Errorf("拉取图像为空: %q", ref)
	}

	detectedMimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if parsedMimeType, _, parseMimeErr := mime.ParseMediaType(detectedMimeType); parseMimeErr == nil && strings.HasPrefix(parsedMimeType, "image/") {
		detectedMimeType = parsedMimeType
	} else {
		detectedMimeType = inferLocalAssetMimeType(path.Base(parsed.Path), body)
	}

	filename = path.Base(parsed.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = fmt.Sprintf("image-%d.%s", index+1, inferOpenAIImageFileExtension(detectedMimeType))
	}
	if ext := strings.TrimSpace(path.Ext(filename)); ext == "" {
		filename += "." + inferOpenAIImageFileExtension(detectedMimeType)
	}

	return filename, detectedMimeType, body, nil
}

func decodeOpenAIDataURL(raw string) (string, []byte, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "data:") {
		return "", nil, fmt.Errorf("不是 data URL")
	}
	comma := strings.Index(trimmed, ",")
	if comma <= 5 {
		return "", nil, fmt.Errorf("data URL 格式无效")
	}
	header := trimmed[5:comma]
	payload := trimmed[comma+1:]
	parts := strings.Split(header, ";")
	mimeType := strings.TrimSpace(parts[0])
	if mimeType == "" {
		mimeType = "image/png"
	}

	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
			break
		}
	}
	if !isBase64 {
		decoded, err := url.QueryUnescape(payload)
		if err != nil {
			return "", nil, err
		}
		return mimeType, []byte(decoded), nil
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, err
	}
	return mimeType, decoded, nil
}

func inferOpenAIImageFileExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return "png"
	}
}

func extractOpenAIImageRef(raw interface{}) string {
	switch value := raw.(type) {
	case map[string]string:
		return value["image_url"]
	case map[string]interface{}:
		if text, ok := value["image_url"].(string); ok {
			return text
		}
	}
	return ""
}

func extractOpenAIImageRefs(raw interface{}) []string {
	switch values := raw.(type) {
	case []map[string]string:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if ref := strings.TrimSpace(value["image_url"]); ref != "" {
				out = append(out, ref)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if ref := strings.TrimSpace(extractOpenAIImageRef(value)); ref != "" {
				out = append(out, ref)
			}
		}
		return out
	}
	return nil
}

func appendOpenAIAllowedParams(reqBody map[string]interface{}, params map[string]interface{}) {
	if params == nil {
		return
	}

	if imageURLs := normalizeOpenAIImageURLStrings(params); len(imageURLs) > 0 {
		// 默认保留首图 image 字段以兼容部分只接收单图的上游实现。
		// 但多图输入必须避免回退成单图字段，防止上游优先读取首图导致退化。
		if len(imageURLs) == 1 {
			reqBody["image"] = imageURLs[0]
		}
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
	// 兜底保护：当 image_urls 已是多图时，确保不会携带退化的单图 image 字段。
	if hasMultipleOpenAIImageURLs(reqBody["image_urls"]) {
		delete(reqBody, "image")
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
	if len(collectOpenAIEditImageInputs(params)) == 0 {
		return false
	}
	featureSlug, ok := workflow.ReadWorkflowStringParam(params, "feature")
	if !ok {
		return false
	}
	if featureSlug == workflow.FeatureSlugCanvasExtend ||
		featureSlug == workflow.FeatureSlugPartialEdit ||
		featureSlug == workflow.FeatureSlugBgSwap ||
		featureSlug == workflow.FeatureSlugRemoveBg ||
		featureSlug == workflow.FeatureSlugStyleFusion ||
		featureSlug == workflow.FeatureSlugSeriesColor ||
		featureSlug == workflow.FeatureSlugDirectedRecolor ||
		featureSlug == workflow.FeatureSlugFaceSwap ||
		featureSlug == workflow.FeatureSlugFabricOnBody ||
		featureSlug == workflow.FeatureSlugMultiAngle {
		return true
	}
	return featureSlug == workflow.FeatureSlugHDUpscale && (hasOpenAIMaskInput(params) || isLocalRedrawEnabled(params))
}

func isLocalRedrawEnabled(params map[string]interface{}) bool {
	if params == nil {
		return false
	}
	enabled, ok := params[localRedrawEnabledParam].(bool)
	return ok && enabled
}

func hasOpenAIMaskInput(params map[string]interface{}) bool {
	if params == nil {
		return false
	}
	for _, key := range []string{"mask_image_url", "mask_url"} {
		if value, ok := params[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	for _, key := range []string{"mask_image_urls", "mask_urls"} {
		switch values := params[key].(type) {
		case []string:
			if len(values) > 0 {
				return true
			}
		case []interface{}:
			for _, value := range values {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					return true
				}
			}
		}
	}
	for _, key := range []string{"use_mask", "mask_enabled", "is_masked", "masked"} {
		if enabled, ok := params[key].(bool); ok && enabled {
			return true
		}
	}
	return false
}

func trustedOpenAIAssetHosts() []string {
	base := resolvePublicAssetBaseURL()
	if base == nil || strings.TrimSpace(base.Hostname()) == "" {
		return nil
	}
	return []string{base.Hostname()}
}

func countOpenAIImageInputs(params map[string]interface{}) int {
	return len(collectOpenAIEditImageInputs(params))
}

func hasMultipleOpenAIImageURLs(raw interface{}) bool {
	switch imageURLs := raw.(type) {
	case []string:
		return len(imageURLs) > 1
	case []interface{}:
		count := 0
		for _, value := range imageURLs {
			if imageURL, ok := value.(string); ok && strings.TrimSpace(imageURL) != "" {
				count++
			}
		}
		return count > 1
	default:
		return false
	}
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
	if refs := collectOpenAIEditImageInputs(params); len(refs) > 0 {
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

func collectOpenAIEditImageInputs(params map[string]interface{}) []map[string]string {
	baseRefs := normalizeOpenAIImageInputs(params)
	if params == nil {
		return baseRefs
	}

	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")
	refs := make([]map[string]string, 0, len(baseRefs)+2)
	seen := make(map[string]struct{}, len(baseRefs)+2)

	appendUniqueRef := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		refs = append(refs, map[string]string{"image_url": trimmed})
	}

	for _, ref := range baseRefs {
		appendUniqueRef(ref["image_url"])
	}

	if featureSlug == workflow.FeatureSlugMultiAngle {
		for _, key := range []string{"model_url", "pose_url"} {
			if ref, ok := workflow.ReadWorkflowStringParam(params, key); ok {
				appendUniqueRef(ref)
			}
		}
	}

	return refs
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
	if cw, ok := readPositiveSizeParam(params, "custom_width"); ok {
		if ch, okh := readPositiveSizeParam(params, "custom_height"); okh {
			w, h = cw, ch
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

func readPositiveSizeParam(params map[string]interface{}, key string) (int, bool) {
	if params == nil {
		return 0, false
	}
	switch value := params[key].(type) {
	case float64:
		if value > 0 {
			return int(value), true
		}
	case float32:
		if value > 0 {
			return int(value), true
		}
	case int:
		if value > 0 {
			return value, true
		}
	case int64:
		if value > 0 {
			return int(value), true
		}
	case int32:
		if value > 0 {
			return int(value), true
		}
	case json.Number:
		if parsed, err := strconv.Atoi(string(value)); err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}
