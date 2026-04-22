package service

import (
	"backend/internal/constants"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/redis"
	"backend/pkg/utils"
)

type AccountService struct {
	accountRepo      *repository.AccountRepository
	taskRepo         *repository.TaskRepository
	redisMgr         *redis.RedisManager
	taskRuntime      *TaskRuntimeService
	cardService      *StyleCardService
	systemConfigRepo *repository.SystemConfigRepository
}

const defaultAccountMaxConcurrent = 3

func NewAccountService(
	accountRepo *repository.AccountRepository,
	taskRepo *repository.TaskRepository,
	redisMgr *redis.RedisManager,
	cardService *StyleCardService,
	systemConfigRepo *repository.SystemConfigRepository,
) *AccountService {
	return &AccountService{
		accountRepo:      accountRepo,
		taskRepo:         taskRepo,
		redisMgr:         redisMgr,
		taskRuntime:      NewTaskRuntimeService(accountRepo, taskRepo, redisMgr),
		cardService:      cardService,
		systemConfigRepo: systemConfigRepo,
	}
}

// ============ 账号池管理 ============

func (s *AccountService) CreateAccount(req *model.CreateAccountRequest) (*model.ApiAccount, error) {
	req.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	if req.Timeout <= 0 {
		req.Timeout = 120
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}

	account := &model.ApiAccount{
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

func (s *AccountService) GetAccount(id uint64) (*model.ApiAccount, error) {
	return s.accountRepo.GetByID(id)
}

func (s *AccountService) ListAccounts(req *model.AccountListRequest) ([]*model.AccountWithLoad, int64, error) {
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
	result := make([]*model.AccountWithLoad, len(accounts))

	for i, acc := range accounts {
		concurrent, _ := s.redisMgr.GetAccountConcurrent(ctx, acc.ID)
		waitQueue, _ := s.redisMgr.GetWaitQueueLength(ctx)

		status := "运行中"
		if acc.Status == 0 {
			status = "已停止"
		} else if acc.FailCount >= MaxFailCount {
			status = "故障"
		}

		result[i] = &model.AccountWithLoad{
			ApiAccount:        *acc,
			CurrentConcurrent: concurrent,
			WaitQueueCount:    int(waitQueue),
			RunningStatus:     status,
		}
	}

	return result, total, nil
}

func (s *AccountService) UpdateAccount(id uint64, req *model.UpdateAccountRequest) error {
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
	// 1. 将该账号下所有 running/pending 的任务标记为 failed
	affected, err := s.taskRepo.FailRunningByAccountID(id, "账号已停用")
	if err != nil {
		log.Printf("[StopAccount] 标记账号 %d 任务失败: %v", id, err)
	} else if affected > 0 {
		log.Printf("[StopAccount] 已将账号 %d 的 %d 个进行中任务标记为失败", id, affected)
	}

	// 2. 清除 Redis 中该账号的并发计数（释放占位）
	ctx := context.Background()
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

func (s *AccountService) SubmitTask(userID uint64, req *model.SubmitTaskRequest) (string, error) {
	// 获取所有可用的账号
	accounts, err := s.accountRepo.GetAllEnabled()
	if err != nil {
		return "", err
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
		featureSlug, err = ResolveWorkflowFeatureSlug(req.Params)
		if err != nil {
			return "", err
		}
		if featureSlug != "" {
			req.Params["feature"] = featureSlug
		}
	}

	// 功能页系统提示词：根据 feature slug 查找系统预设 prompt，支持模板变量替换
	fullPrompt = applyFeatureSystemPrompt(fullPrompt, req.Prompt, featureSlug, req.Params, func(slug string) (string, error) {
		if s.systemConfigRepo == nil {
			return "", fmt.Errorf("system config repo 未配置")
		}
		return s.systemConfigRepo.Get(constants.FeaturePromptConfigKey(slug))
	})

	// line-draft 已退休：line_draft_mode_id 不再参与系统提示词主链覆盖。
	if req.Params != nil {
		if _, exists := req.Params["line_draft_mode_id"]; exists {
			// 保留参数透传，避免破坏旧调用方；这里只显式不再介入 fullPrompt 优先级。
		}
	}

	if featureSlug == styleFusionFeatureSlug {
		fullPrompt = appendHiddenConstraint(fullPrompt, buildStyleFusionHiddenConstraint(req.Params))
	}
	if featureSlug == "multi-angle" {
		fullPrompt = appendHiddenConstraint(fullPrompt, buildMultiAngleHiddenConstraint(req.Params))
	}
	if featureSlug == canvasExtendFeatureSlug {
		fullPrompt = appendHiddenConstraint(fullPrompt, buildCanvasExtendHiddenConstraint(req.Params))
	}
	if featureSlug == hdUpscaleFeatureSlug && isMaskedHDUpscaleRequest(req.Params) {
		fullPrompt = appendHiddenConstraint(fullPrompt, hdUpscaleMaskedConstraint)
	}
	if featureSlug == fabricOnBodyFeatureSlug && isMaskedFabricOnBodyRequest(req.Params) {
		fullPrompt = appendHiddenConstraint(fullPrompt, fabricOnBodyMaskedConstraint)
	}

	// 序列化参数
	paramsJSON := ""
	if req.Params != nil {
		b, _ := utils.JSONMarshal(req.Params)
		paramsJSON = string(b)
	}

	// 创建任务记录
	task := &model.Task{
		TaskID:     taskID,
		UserID:     userID,
		ChannelID:  0,
		Prompt:     fullPrompt,
		UserPrompt: req.Prompt,
		Params:     paramsJSON,
		Status:     constants.TaskStatusPending,
	}

	if err := s.taskRepo.Create(task); err != nil {
		return "", err
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
		return "", err
	}

	return taskID, nil
}

func (s *AccountService) GetTask(taskID string) (*model.Task, error) {
	return s.taskRepo.GetByTaskID(taskID)
}

func (s *AccountService) GetUserTask(taskID string, userID uint64) (*model.Task, error) {
	return s.taskRepo.GetByTaskIDAndUserID(taskID, userID)
}

func (s *AccountService) ListTasks(req *model.TaskListRequest) ([]*model.Task, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	return s.taskRepo.List(req.AccountID, req.Status, req.Page, req.PageSize)
}

func (s *AccountService) DeleteUserTask(taskID string, userID uint64) error {
	return s.taskRepo.DeleteByTaskIDAndUserID(taskID, userID)
}

func (s *AccountService) GetUserTasks(userID uint64, page, pageSize int) ([]*model.Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.taskRepo.GetByUserID(userID, page, pageSize)
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
	pending, _ := s.taskRepo.CountByStatus(constants.TaskStatusPending)
	running, _ := s.taskRepo.CountByStatus(constants.TaskStatusRunning)
	completed, _ := s.taskRepo.CountByStatus(constants.TaskStatusCompleted)
	failed, _ := s.taskRepo.CountByStatus(constants.TaskStatusFailed)

	return map[string]int64{
		constants.TaskStatusPending:   pending,
		constants.TaskStatusRunning:   running,
		constants.TaskStatusCompleted: completed,
		constants.TaskStatusFailed:    failed,
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
