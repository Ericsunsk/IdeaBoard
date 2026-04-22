package worker

import (
	"backend/internal/constants"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/pkg/redis"
)

type Consumer struct {
	accountRepo    *repository.AccountRepository
	channelRepo    *repository.ChannelRepository
	taskRepo       *repository.TaskRepository
	redisMgr       *redis.RedisManager
	storageService *service.StorageService
	taskRuntime    *service.TaskRuntimeService
	orchestrator   *taskExecutionOrchestrator

	wg      sync.WaitGroup
	stopCh  chan struct{}
	running bool
	mu      sync.Mutex
}

func NewConsumer(
	accountRepo *repository.AccountRepository,
	channelRepo *repository.ChannelRepository,
	taskRepo *repository.TaskRepository,
	redisMgr *redis.RedisManager,
	storageService *service.StorageService,
) *Consumer {
	taskRuntime := service.NewTaskRuntimeService(accountRepo, taskRepo, redisMgr)
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

	if status == constants.QueueAssignStatusQueueFull {
		log.Printf("[Consumer] 等待队列满，将任务放回: %s", task.TaskID)
		c.redisMgr.PushGlobalQueueFront(ctx, task)
		return
	}

	if status == constants.QueueAssignStatusWaitQueue {
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

func (c *Consumer) callDrawingAPI(account *model.ApiAccount, task *redis.GlobalQueueTask) (string, error) {
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
		json.Unmarshal([]byte(task.Params), &params)
	}

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
		imageURL, err = c.callStabilityAI(account, task.Prompt, width, height, timeout)
	default:
		// nano banana / openai / gemini 等 OpenAI 兼容平台（官方原生格式）
		imageURL, err = c.callOpenAI(account, task.Prompt, width, height, timeout, params)
	}

	if err != nil {
		return "", fmt.Errorf("调用 API 失败 (渠道: %s): %v", channelName, err)
	}

	// 如果对象存储已启用，将图片转存到当前存储
	log.Printf("[Consumer] 存储检查: service=%v, objectStorage=%v", c.storageService != nil, c.storageService != nil && c.storageService.UsesObjectStorage())
	if c.storageService != nil && c.storageService.UsesObjectStorage() {
		log.Printf("[Consumer] 对象存储已启用，开始转存图片, directFetch=%v, imageURL=%.80s", c.storageService.IsDirectFetchEnabled(), imageURL)
		reURL, reErr := c.storageService.StoreRemote(context.Background(), imageURL, service.StorageStoreOptions{
			AllowHosts: []string{account.ProxyURL},
		})
		if reErr == nil {
			log.Printf("[Consumer] 转存成功: %s -> %s", imageURL[:min(80, len(imageURL))], reURL)
			imageURL = reURL
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
		"status":       constants.TaskResultStatusSuccess,
		"image_url":    imageURL,
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// callOpenAI 调用 OpenAI DALL-E 风格接口（也兼容其他 OpenAI 兼容平台）
func (c *Consumer) callOpenAI(account *model.ApiAccount, prompt string, width, height int, timeout time.Duration, params map[string]interface{}) (string, error) {
	baseURL := "https://api.openai.com"
	if account.ProxyURL != "" {
		baseURL = strings.TrimRight(account.ProxyURL, "/")
	}

	model := "dall-e-3"
	if account.ModelSet != "" {
		model = account.ModelSet
	}

	size := fmt.Sprintf("%dx%d", width, height)

	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"n":      1,
		"size":   size,
	}

	appendOpenAIAllowedParams(reqBody, params)

	bodyJSON, _ := json.Marshal(reqBody)

	// 如果 ProxyURL 已包含 /v1，则不再拼接 /v1
	endpoint := baseURL + "/v1/images/generations"
	if strings.HasSuffix(baseURL, "/v1") {
		endpoint = baseURL + "/images/generations"
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+account.ApiKey)

	log.Printf("[Consumer] OpenAI 请求: model=%s, size=%s, prompt=%.50s...", model, size, prompt)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP 请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API 返回 %d: %s", resp.StatusCode, string(respBody))
	}

	var respData struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	if len(respData.Data) == 0 {
		return "", fmt.Errorf("API 未返回图片")
	}

	return respData.Data[0].URL, nil
}

func appendOpenAIAllowedParams(reqBody map[string]interface{}, params map[string]interface{}) {
	if params == nil {
		return
	}

	// OpenAI 兼容图像接口通常接收 image 字段；前端沿用 image_urls，这里做受控映射。
	if imgURLs, ok := params["image_urls"]; ok {
		reqBody["image"] = imgURLs
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

// callStabilityAI 调用 Stability AI 接口
func (c *Consumer) callStabilityAI(account *model.ApiAccount, prompt string, width, height int, timeout time.Duration) (string, error) {
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
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+account.ApiKey)
	req.Header.Set("Accept", "application/json")

	log.Printf("[Consumer] StabilityAI 请求: engine=%s, %dx%d", engine, width, height)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP 请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API 返回 %d: %s", resp.StatusCode, string(respBody))
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
