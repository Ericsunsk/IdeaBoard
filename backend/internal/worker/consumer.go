package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path"
	"strings"
	"sync"
	"time"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/pkg/redis"
)

type Consumer struct {
	accountRepo *repository.AccountRepository
	channelRepo *repository.ChannelRepository
	taskRepo    *repository.TaskRepository
	redisMgr    *redis.RedisManager
	kodoService *service.KodoService

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
	kodoService *service.KodoService,
) *Consumer {
	return &Consumer{
		accountRepo: accountRepo,
		channelRepo: channelRepo,
		taskRepo:    taskRepo,
		redisMgr:    redisMgr,
		kodoService: kodoService,
		stopCh:      make(chan struct{}),
	}
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

	if err := c.refreshTaskAccounts(task); err != nil {
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

	if status == "queue_full" {
		log.Printf("[Consumer] 等待队列满，将任务放回: %s", task.TaskID)
		c.redisMgr.PushGlobalQueueFront(ctx, task)
		return
	}

	if status == "wait_queue" {
		log.Printf("[Consumer] 任务进入等待队列: %s", task.TaskID)
		return
	}

	go c.executeTask(task, accountID)
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

func (c *Consumer) refreshTaskAccounts(task *redis.GlobalQueueTask) error {
	accounts, err := c.accountRepo.GetAllEnabled()
	if err != nil {
		return err
	}

	task.AccountIDs = task.AccountIDs[:0]
	for _, account := range accounts {
		task.AccountIDs = append(task.AccountIDs, account.ID)
	}

	return nil
}

func (c *Consumer) executeTask(task *redis.GlobalQueueTask, accountID uint64) {
	log.Printf("[Consumer] 执行任务: %s, 账号: %d", task.TaskID, accountID)
	startTime := time.Now()

	account, err := c.accountRepo.GetByID(accountID)
	if err != nil {
		c.requeueTask(task, accountID, "账号不存在，重新调度")
		return
	}
	if account.Status != 1 {
		c.requeueTask(task, accountID, "账号已停用，重新调度")
		return
	}

	t, _ := c.taskRepo.GetByTaskID(task.TaskID)
	if t != nil {
		t.Status = "running"
		t.AccountID = accountID
		c.taskRepo.Update(t)
	}

	result, err := c.callDrawingAPI(account, task)

	if err != nil {
		log.Printf("[Consumer] 任务 %s 执行失败: %v", task.TaskID, err)
		elapsed := time.Since(startTime).Seconds()
		c.completeTask(task, accountID, "failed", err.Error(), elapsed)
		c.accountRepo.IncrementFailCount(accountID)
		if account.FailCount+1 >= 5 {
			c.accountRepo.UpdateStatus(accountID, 0)
			log.Printf("[Consumer] 账号 %d 失败次数过多，已自动禁用", accountID)
		}
		return
	}

	elapsed := time.Since(startTime).Seconds()
	c.completeTask(task, accountID, "completed", result, elapsed)
	c.accountRepo.ResetFailCount(accountID)
}

func (c *Consumer) requeueTask(task *redis.GlobalQueueTask, accountID uint64, reason string) {
	ctx := context.Background()
	if accountID > 0 {
		_ = c.redisMgr.ReleaseTask(ctx, accountID, task.TaskID)
	}
	if err := c.refreshTaskAccounts(task); err != nil {
		c.failTask(task, 0, reason)
		return
	}
	if len(task.AccountIDs) == 0 {
		c.failTask(task, 0, reason)
		return
	}
	if err := c.redisMgr.PushGlobalQueueFront(ctx, task); err != nil {
		c.failTask(task, 0, reason)
	}
}

func (c *Consumer) completeTask(task *redis.GlobalQueueTask, accountID uint64, status, result string, elapsedSec float64) {
	ctx := context.Background()
	c.redisMgr.ReleaseTask(ctx, accountID, task.TaskID)

	t, err := c.taskRepo.GetByTaskID(task.TaskID)
	if err == nil {
		t.Status = status
		t.Result = result
		now := time.Now()
		t.CompletedAt = &now
		c.taskRepo.Update(t)
	}

	log.Printf("[Consumer] 任务 %s 完成, 状态: %s, 耗时: %.1fs", task.TaskID, status, elapsedSec)
}

func (c *Consumer) failTask(task *redis.GlobalQueueTask, accountID uint64, reason string) {
	ctx := context.Background()
	if accountID > 0 {
		c.redisMgr.ReleaseTask(ctx, accountID, task.TaskID)
	}

	result := fmt.Sprintf(`{"task_id":"%s","status":"failed","error":"%s"}`, task.TaskID, reason)
	c.completeTask(task, accountID, "failed", result, 0)

	log.Printf("[Consumer] 任务 %s 失败: %s", task.TaskID, reason)
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

	// 如果七牛云 Kodo 已启用，将图片转存到 Kodo
	log.Printf("[Consumer] Kodo 检查: service=%v, enabled=%v", c.kodoService != nil, c.kodoService != nil && c.kodoService.IsEnabled())
	if c.kodoService != nil && c.kodoService.IsEnabled() {
		log.Printf("[Consumer] 七牛云已启用，开始转存图片, directFetch=%v, imageURL=%.80s", c.kodoService.IsDirectFetch(), imageURL)
		var reURL string
		var reErr error

		if c.kodoService.IsDirectFetch() && !strings.HasPrefix(imageURL, "data:image/") {
			// 直传模式：七牛云直接从上游 URL 抓取，不经过业务服务器
			log.Printf("[Consumer] 使用七牛直抓模式")
			reURL, reErr = c.kodoService.FetchURL(imageURL)
			if reErr == nil {
				log.Printf("[Consumer] 图片已通过七牛直抓存储: %s", reURL)
			} else {
				log.Printf("[Consumer] 七牛直抓失败，回退中转上传: %v", reErr)
				reURL, reErr = c.reuploadToKodo(imageURL)
			}
		} else {
			// 中转模式：下载到本服务器再上传到七牛
			log.Printf("[Consumer] 使用中转上传模式")
			reURL, reErr = c.reuploadToKodo(imageURL)
		}

		if reErr == nil {
			log.Printf("[Consumer] 转存成功: %s -> %s", imageURL[:min(80, len(imageURL))], reURL)
			imageURL = reURL
		} else {
			log.Printf("[Consumer] 转存七牛云失败，使用原始 URL: %v", reErr)
		}
	} else {
		log.Printf("[Consumer] 七牛云未启用，跳过转存")
	}

	result := map[string]interface{}{
		"task_id":      task.TaskID,
		"account_id":   account.ID,
		"channel_id":   account.ChannelID,
		"channel_name": channelName,
		"platform":     account.Platform,
		"api_type":     account.ApiType,
		"model_set":    account.ModelSet,
		"status":       "success",
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

	// 透传参考图片 URL（用于图生图 / 变体生成）
	if params != nil {
		if imgURLs, ok := params["image_urls"]; ok {
			reqBody["image"] = imgURLs
		}
		if strength, ok := params["variation_strength"]; ok {
			reqBody["variation_strength"] = strength
		}
		if blendRatio, ok := params["blend_ratio"]; ok {
			reqBody["blend_ratio"] = blendRatio
		}
		if imageAWeight, ok := params["image_a_weight"]; ok {
			reqBody["image_a_weight"] = imageAWeight
		}
		if imageBWeight, ok := params["image_b_weight"]; ok {
			reqBody["image_b_weight"] = imageBWeight
		}
	}

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

// reuploadToKodo 下载远程图片并转存到七牛云
func (c *Consumer) reuploadToKodo(imageURL string) (string, error) {
	if strings.HasPrefix(imageURL, "data:image/") {
		// base64 图片，构建临时 multipart.FileHeader
		return c.uploadBase64ToKodo(imageURL)
	}

	// 下载远程图片
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取图片数据失败: %v", err)
	}

	// 从 URL 推断扩展名
	ext := path.Ext(strings.Split(imageURL, "?")[0])
	if ext == "" {
		ct := resp.Header.Get("Content-Type")
		switch {
		case strings.Contains(ct, "png"):
			ext = ".png"
		case strings.Contains(ct, "webp"):
			ext = ".webp"
		case strings.Contains(ct, "gif"):
			ext = ".gif"
		default:
			ext = ".jpg"
		}
	}

	filename := fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), ext)
	fh := createFileHeader(filename, imgData)
	return c.kodoService.Upload(fh)
}

// uploadBase64ToKodo 将 base64 图片上传到七牛云
func (c *Consumer) uploadBase64ToKodo(dataURL string) (string, error) {
	// 格式: data:image/png;base64,xxxxx
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("无效的 base64 图片格式")
	}

	b64Data := parts[1]
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %v", err)
	}

	ext := ".png"
	if strings.Contains(parts[0], "jpeg") || strings.Contains(parts[0], "jpg") {
		ext = ".jpg"
	} else if strings.Contains(parts[0], "webp") {
		ext = ".webp"
	}

	filename := fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), ext)
	fh := createFileHeader(filename, decoded)
	return c.kodoService.Upload(fh)
}

// createFileHeader 从内存数据创建 multipart.FileHeader
func createFileHeader(filename string, data []byte) *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", "application/octet-stream")

	part, _ := writer.CreatePart(h)
	part.Write(data)
	writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(int64(len(data) + 1024))
	if files, ok := form.File["file"]; ok && len(files) > 0 {
		return files[0]
	}
	return nil
}
