package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"api/internal/assets"
	"api/internal/platform/redis"
	"api/internal/platform/settings"
	"api/internal/prompts"
	"api/internal/tasks"
	"api/internal/workflow"
)

type AccountService struct {
	accountRepo      *AccountRepository
	taskRepo         *TaskRepository
	redisMgr         *redis.RedisManager
	taskRuntime      *TaskRuntimeService
	cardService      *assets.StyleCardService
	systemConfigRepo *settings.SystemConfigRepository
	previewResolver  previewURLResolver
}

type previewURLResolver interface {
	PreviewURLTransformer() func(string) string
}

const defaultAccountMaxConcurrent = 3
const taskCancelTTL = 24 * time.Hour

func NewAccountService(
	accountRepo *AccountRepository,
	taskRepo *TaskRepository,
	redisMgr *redis.RedisManager,
	cardService *assets.StyleCardService,
	systemConfigRepo *settings.SystemConfigRepository,
	previewResolver previewURLResolver,
) *AccountService {
	return &AccountService{
		accountRepo:      accountRepo,
		taskRepo:         taskRepo,
		redisMgr:         redisMgr,
		taskRuntime:      NewTaskRuntimeService(accountRepo, taskRepo, redisMgr),
		cardService:      cardService,
		systemConfigRepo: systemConfigRepo,
		previewResolver:  previewResolver,
	}
}

// ============ 账号池管理 ============

func (s *AccountService) CreateAccount(req *CreateAccountRequest) (*ApiAccount, error) {
	req.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	if req.Timeout <= 0 {
		req.Timeout = 120
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}

	account := &ApiAccount{
		Name:          req.Name,
		ChannelID:     req.ChannelID,
		Platform:      req.Platform,
		ApiType:       req.ApiType,
		ModelSet:      req.ModelSet,
		ApiKey:        req.ApiKey,
		ProxyURL:      req.ProxyURL,
		MaxConcurrent: req.MaxConcurrent,
		Timeout:       req.Timeout,
		ExtraConfig:   req.ExtraConfig,
		Weight:        req.Weight,
		Status:        1,
	}

	if err := s.accountRepo.Create(account); err != nil {
		return nil, err
	}

	ctx := context.Background()
	s.redisMgr.InitAccountConcurrent(ctx, account.ID, account.MaxConcurrent)

	return account, nil
}

func (s *AccountService) GetAccount(id uint64) (*ApiAccount, error) {
	return s.accountRepo.GetByID(id)
}

func (s *AccountService) ListAccounts(req *AccountListRequest) ([]*AccountWithLoad, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	accounts, total, err := s.accountRepo.List(req.Keyword, req.Platform, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}

	ctx := context.Background()
	result := make([]*AccountWithLoad, len(accounts))

	for i, acc := range accounts {
		concurrent, _ := s.redisMgr.GetAccountConcurrent(ctx, acc.ID)
		waitQueue, _ := s.redisMgr.GetWaitQueueLength(ctx)

		status := "运行中"
		if acc.Status == 0 {
			status = "已停止"
		} else if acc.FailCount >= MaxFailCount {
			status = "故障"
		}

		result[i] = &AccountWithLoad{
			ApiAccount:        *acc,
			CurrentConcurrent: concurrent,
			WaitQueueCount:    int(waitQueue),
			RunningStatus:     status,
		}
	}

	return result, total, nil
}

func (s *AccountService) UpdateAccount(id uint64, req *UpdateAccountRequest) error {
	account, err := s.accountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("账号不存在")
	}

	account.Name = req.Name
	if req.ChannelID > 0 {
		account.ChannelID = req.ChannelID
	}
	account.Platform = req.Platform
	account.ApiType = req.ApiType
	account.ModelSet = req.ModelSet
	account.ProxyURL = req.ProxyURL
	account.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	account.Timeout = req.Timeout
	account.ExtraConfig = req.ExtraConfig
	account.Weight = req.Weight
	account.Status = req.Status

	if req.ApiKey != "" {
		account.ApiKey = req.ApiKey
	}

	if err := s.accountRepo.Update(account); err != nil {
		return err
	}

	ctx := context.Background()
	s.redisMgr.SetAccountMaxConcurrent(ctx, account.ID, account.MaxConcurrent)

	return nil
}

func (s *AccountService) DeleteAccount(id uint64) error {
	ctx := context.Background()
	s.redisMgr.DeleteAccountConcurrent(ctx, id)
	return s.accountRepo.Delete(id)
}

func (s *AccountService) StartAccount(id uint64) error {
	account, err := s.accountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("账号不存在")
	}

	account.MaxConcurrent = normalizeMaxConcurrent(account.MaxConcurrent)
	if err := s.accountRepo.UpdateStatus(id, 1); err != nil {
		return err
	}

	ctx := context.Background()
	return s.redisMgr.EnsureAccountConcurrent(ctx, id, account.MaxConcurrent)
}

func (s *AccountService) StopAccount(id uint64) error {
	ctx := context.Background()

	taskIDs, err := s.taskRepo.ListTaskIDsByAccountIDAndStatuses(id, tasks.StatusPending, tasks.StatusRunning)
	if err != nil {
		log.Printf("[StopAccount] 查询账号 %d 进行中任务失败: %v", id, err)
	} else {
		for _, taskID := range taskIDs {
			if cancelErr := s.redisMgr.MarkTaskCancelled(ctx, taskID, taskCancelTTL); cancelErr != nil {
				log.Printf("[StopAccount] 标记任务取消失败: account=%d task=%s err=%v", id, taskID, cancelErr)
			}
		}
		if len(taskIDs) > 0 {
			log.Printf("[StopAccount] 账号 %d 已标记 %d 个任务取消", id, len(taskIDs))
		}
	}

	// 1. 将该账号下所有 running/pending 的任务标记为 failed
	affected, err := s.taskRepo.FailRunningByAccountID(id, "账号已停用")
	if err != nil {
		log.Printf("[StopAccount] 标记账号 %d 任务失败: %v", id, err)
	} else if affected > 0 {
		log.Printf("[StopAccount] 已将账号 %d 的 %d 个进行中任务标记为失败", id, affected)
	}

	// 2. 清除 Redis 中该账号的并发计数（释放占位）
	s.redisMgr.DeleteAccountConcurrent(ctx, id)

	// 3. 停用账号
	return s.accountRepo.UpdateStatus(id, 0)
}

func (s *AccountService) GetAccountQueueInfo(id uint64) (concurrent int, maxConcurrent int, err error) {
	ctx := context.Background()
	concurrent, err = s.redisMgr.GetAccountConcurrent(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	maxConcurrent, err = s.redisMgr.GetAccountMaxConcurrent(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return concurrent, maxConcurrent, nil
}

// ============ 任务管理 ============

func (s *AccountService) SubmitTask(userID uint64, req *tasks.SubmitTaskRequest) (string, error) {
	// 获取所有可用的账号
	accounts, err := s.accountRepo.GetAllEnabled()
	if err != nil {
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	if len(accounts) == 0 {
		return "", fmt.Errorf("没有可用的 API 账号，请先添加账号")
	}

	// 生成任务ID
	taskID := generateSecureTaskID("task")

	params := req.Params
	if params == nil {
		params = map[string]interface{}{}
	}
	if req.Feature != "" {
		params["feature"] = req.Feature
	}
	if len(req.ImageURLs) > 0 {
		params["image_urls"] = req.ImageURLs
	}
	req.Params = params

	// 使用风格卡片拼接完整提示词
	fullPrompt := req.Prompt
	if req.CardIDs != nil && len(req.CardIDs) > 0 {
		p, err := s.cardService.BuildPrompt(req.Prompt, req.CardIDs)
		if err == nil {
			fullPrompt = p
		}
	}

	featureSlug := ""
	if req.Params != nil {
		featureSlug, err = workflow.ResolveWorkflowFeatureSlug(req.Params)
		if err != nil {
			return "", err
		}
		if featureSlug != "" {
			req.Params["feature"] = featureSlug
		}
	}
	accounts = filterAccountsByFeature(accounts, featureSlug)
	if len(accounts) == 0 {
		return "", fmt.Errorf("没有可用的 API 账号，请先添加账号")
	}

	fullPrompt = applyTaskSystemPrompt(fullPrompt, req.Prompt, featureSlug, req.Params, func(slug string) (string, error) {
		if s.systemConfigRepo == nil {
			return "", fmt.Errorf("system config repo 未配置")
		}
		return s.systemConfigRepo.Get(prompts.FeaturePromptConfigKey(slug))
	})
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		return "", err
	}

	// line-draft 已退休：line_draft_mode_id 不再参与系统提示词主链覆盖。
	if req.Params != nil {
		if _, exists := req.Params["line_draft_mode_id"]; exists {
			// 保留参数透传，避免破坏旧调用方；这里只显式不再介入 fullPrompt 优先级。
		}
	}

	// 序列化参数
	paramsJSON := ""
	if req.Params != nil {
		b, _ := json.Marshal(req.Params)
		paramsJSON = string(b)
	}

	// 创建任务记录
	task := &tasks.Task{
		TaskID:     taskID,
		UserID:     userID,
		ChannelID:  0,
		Prompt:     fullPrompt,
		UserPrompt: req.Prompt,
		Params:     paramsJSON,
		Status:     tasks.StatusPending,
	}

	if err := s.taskRepo.Create(task); err != nil {
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	// 获取所有可用账号ID
	accountIDs := make([]uint64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}

	// 推送到全局队列
	queueTask := &redis.GlobalQueueTask{
		TaskID:     taskID,
		UserID:     userID,
		Prompt:     fullPrompt,
		Params:     paramsJSON,
		AccountIDs: accountIDs,
		RetryCount: 0,
		CreatedAt:  time.Now().Unix(),
	}

	if err := s.redisMgr.PushGlobalQueue(context.Background(), queueTask); err != nil {
		_ = s.taskRepo.MarkQueueFailed(taskID, "任务入队失败: "+err.Error())
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	return taskID, nil
}

func (s *AccountService) GetTask(taskID string) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return nil, err
	}
	s.applyPreviewForTasks([]*tasks.Task{task})
	return task, nil
}

func (s *AccountService) GetUserTask(taskID string, userID uint64) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskIDAndUserID(taskID, userID)
	if err != nil {
		return nil, err
	}
	s.applyPreviewForTasks([]*tasks.Task{task})
	return task, nil
}

func (s *AccountService) ListTasks(req *tasks.TaskListRequest) ([]*tasks.Task, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	items, total, err := s.taskRepo.List(req.AccountID, req.Status, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}
	s.applyPreviewForTasks(items)
	return items, total, nil
}

func (s *AccountService) DeleteUserTask(taskID string, userID uint64) error {
	if _, err := s.taskRepo.GetByTaskIDAndUserID(taskID, userID); err != nil {
		return err
	}
	if err := s.redisMgr.MarkTaskCancelled(context.Background(), taskID, taskCancelTTL); err != nil {
		return err
	}
	return s.taskRepo.DeleteByTaskIDAndUserID(taskID, userID)
}

func (s *AccountService) GetUserTasks(userID uint64, page, pageSize int) ([]*tasks.Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	items, total, err := s.taskRepo.GetByUserID(userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	s.applyPreviewForTasks(items)
	return items, total, nil
}

// ============ 监控 ============

func (s *AccountService) GetMonitorInfo() (*redis.MonitorInfo, error) {
	ctx := context.Background()
	accounts, _ := s.accountRepo.GetAllEnabled()
	accountIDs := make([]uint64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}
	return s.redisMgr.GetMonitorInfo(ctx, accountIDs)
}

func normalizeMaxConcurrent(value int) int {
	if value <= 0 {
		return defaultAccountMaxConcurrent
	}
	return value
}

func (s *AccountService) GetTaskStats() (map[string]int64, error) {
	pending, _ := s.taskRepo.CountByStatus(tasks.StatusPending)
	running, _ := s.taskRepo.CountByStatus(tasks.StatusRunning)
	completed, _ := s.taskRepo.CountByStatus(tasks.StatusCompleted)
	failed, _ := s.taskRepo.CountByStatus(tasks.StatusFailed)

	return map[string]int64{
		tasks.StatusPending:   pending,
		tasks.StatusRunning:   running,
		tasks.StatusCompleted: completed,
		tasks.StatusFailed:    failed,
	}, nil
}

// ============ 消费服务调用 ============

func (s *AccountService) GetNextTaskFromQueue() (*redis.GlobalQueueTask, error) {
	ctx := context.Background()
	return s.redisMgr.PopGlobalQueue(ctx)
}

func (s *AccountService) AssignTaskToAccount(task *redis.GlobalQueueTask) (uint64, string, error) {
	ctx := context.Background()
	return s.redisMgr.AssignTask(ctx, task.AccountIDs, task)
}

func (s *AccountService) ReleaseTask(accountID uint64, taskID string) error {
	return s.redisMgr.ReleaseTask(context.Background(), accountID, taskID)
}

func (s *AccountService) MarkTaskRunning(taskID string, accountID uint64) error {
	return s.taskRuntime.MarkTaskRunning(taskID, accountID)
}

func (s *AccountService) MarkTaskCompleted(taskID string, result string) error {
	return s.taskRuntime.MarkTaskCompleted(taskID, result)
}

func (s *AccountService) MarkTaskFailed(taskID string, accountID uint64, errMsg string) error {
	return s.taskRuntime.MarkTaskFailedWithRetry(taskID, accountID, errMsg)
}

// generateSecureTaskID produces a non-guessable task identifier using crypto/rand.
func generateSecureTaskID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to timestamp only.
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

func applyTaskSystemPrompt(
	basePrompt string,
	userPrompt string,
	featureSlug string,
	params map[string]interface{},
	loadPrompt func(string) (string, error),
) string {
	// 管理员后台配置是系统提示词唯一来源；这里不再追加代码层隐藏约束。
	return prompts.ApplyFeatureSystemPrompt(basePrompt, userPrompt, featureSlug, params, loadPrompt)
}

func validateExecutablePrompt(fullPrompt string) error {
	if strings.TrimSpace(fullPrompt) == "" {
		return fmt.Errorf("缺少提示词，请填写 Prompt 或配置后台 system prompt")
	}
	return nil
}

func (s *AccountService) applyPreviewForTasks(items []*tasks.Task) {
	if len(items) == 0 || s.previewResolver == nil {
		return
	}
	transform := s.previewResolver.PreviewURLTransformer()
	if transform == nil {
		return
	}
	for _, item := range items {
		s.applyPreviewForTask(item, transform)
	}
}

func (s *AccountService) applyPreviewForTask(task *tasks.Task, transform func(string) string) {
	if task == nil || strings.TrimSpace(task.Result) == "" {
		return
	}
	if !strings.Contains(task.Result, "image_url") && !strings.Contains(task.Result, "image_urls") {
		return
	}

	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(task.Result), &resultMap); err != nil {
		return
	}

	changed := false
	if raw, ok := resultMap["image_url"].(string); ok {
		next := transform(strings.TrimSpace(raw))
		if next != raw {
			resultMap["image_url"] = next
			changed = true
		}
	}

	if rawList, ok := resultMap["image_urls"].([]interface{}); ok {
		for i, item := range rawList {
			raw, ok := item.(string)
			if !ok {
				continue
			}
			next := transform(strings.TrimSpace(raw))
			if next != raw {
				rawList[i] = next
				changed = true
			}
		}
		resultMap["image_urls"] = rawList
	}

	if !changed {
		return
	}
	if encoded, err := json.Marshal(resultMap); err == nil {
		task.Result = string(encoded)
	}
}
