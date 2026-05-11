package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"api/internal/accounts"
	"api/internal/channels"
	"api/internal/platform/redis"
	"api/internal/runtime/taskqueue"
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
	queue          taskqueue.TaskQueue
	assigner       taskqueue.TaskAssigner
	storageService *storage.StorageService
	taskRuntime    consumerTaskRuntime
	orchestrator   *taskExecutionOrchestrator
	executeTask    func(task *redis.GlobalQueueTask, accountID uint64)

	wg      sync.WaitGroup
	stopCh  chan struct{}
	running bool
	mu      sync.Mutex
}

type consumerTaskRuntime interface {
	RefreshTaskAccounts(task *redis.GlobalQueueTask) error
	ReleaseTask(accountID uint64, taskID string) error
	MarkTaskFailed(taskID string, reason string) error
}

type providerCallResult struct {
	ImageURL string
	Model    string
}

func NewConsumer(
	accountRepo *accounts.AccountRepository,
	channelRepo *channels.ChannelRepository,
	taskRepo *accounts.TaskRepository,
	redisMgr *redis.RedisManager,
	storageService *storage.StorageService,
	taskEventHub *accounts.TaskEventHub,
) *Consumer {
	// Compatibility wrapper for default Redis wiring.
	redisQueue := taskqueue.NewRedisQueue(redisMgr)
	return NewConsumerWithQueue(accountRepo, channelRepo, taskRepo, redisQueue, redisQueue, redisQueue, storageService, taskEventHub)
}

func NewConsumerWithQueue(
	accountRepo *accounts.AccountRepository,
	channelRepo *channels.ChannelRepository,
	taskRepo *accounts.TaskRepository,
	queue taskqueue.TaskQueue,
	assigner taskqueue.TaskAssigner,
	cancelStore taskqueue.TaskCancellationStore,
	storageService *storage.StorageService,
	taskEventHub *accounts.TaskEventHub,
) *Consumer {
	taskRuntime := accounts.NewTaskRuntimeServiceWithQueue(accountRepo, taskRepo, assigner, cancelStore)
	taskRuntime.SetEventHub(taskEventHub)
	consumer := &Consumer{
		accountRepo:    accountRepo,
		channelRepo:    channelRepo,
		taskRepo:       taskRepo,
		queue:          queue,
		assigner:       assigner,
		storageService: storageService,
		taskRuntime:    taskRuntime,
		stopCh:         make(chan struct{}),
	}
	consumer.orchestrator = newTaskExecutionOrchestrator(accountRepo, queue, taskRuntime, consumer.callDrawingAPI)
	consumer.executeTask = consumer.orchestrator.execute
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

	task, err := c.queue.Dequeue(ctx)
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

	accountID, status, err := c.assigner.Assign(ctx, task.AccountIDs, task)
	if err != nil {
		log.Printf("[Consumer] 分配任务失败: %v", err)
		c.failTask(task, 0, "分配失败: "+err.Error())
		return
	}

	if status == taskqueue.AssignStatusQueueFull {
		log.Printf("[Consumer] 等待队列满，将任务放回: %s", task.TaskID)
		if pushErr := c.queue.EnqueueFront(ctx, task); pushErr != nil {
			log.Printf("[Consumer] 任务回流失败: %v", pushErr)
		}
		return
	}

	if status == taskqueue.AssignStatusWaitQueue {
		log.Printf("[Consumer] 任务进入等待队列: %s", task.TaskID)
		return
	}

	go c.executeTask(task, accountID)
}

func (c *Consumer) promoteWaitQueue(ctx context.Context) {
	task, err := c.queue.DequeueWait(ctx)
	if err != nil {
		log.Printf("[Consumer] 拉取等待队列失败: %v", err)
		return
	}
	if task == nil {
		return
	}
	if err := c.queue.EnqueueFront(ctx, task); err != nil {
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

func (c *Consumer) callDrawingAPI(ctx context.Context, account *accounts.ApiAccount, task *redis.GlobalQueueTask) (string, error) {
	log.Printf("[Consumer] 调用 API, 账号: %s, ChannelID: %d",
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
	imageURL := callResult.ImageURL

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
		"status":       tasks.ResultStatusSuccess,
		"image_url":    imageURL,
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
