package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"api/internal/prompts"
	"api/internal/runtime/queuetypes"
	"api/internal/tasks"
	"api/internal/workflow"
)

func (s *AccountService) SubmitTask(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("请求参数错误")
	}
	accounts, params, _, fullPrompt, err := s.prepareSubmitTask(ctx, userID, req)
	if err != nil {
		return "", err
	}

	// 生成任务ID
	taskID := generateSecureTaskID("task")

	// 序列化参数
	paramsJSON, err := marshalSubmitTaskParams(params)
	if err != nil {
		return "", err
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

	if err := s.taskRepo.Create(ctx, task); err != nil {
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	// 获取所有可用账号ID
	accountIDs := make([]int64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}

	// 推送到全局队列
	queueTask := &queuetypes.TaskPayload{
		TaskID:     taskID,
		UserID:     userID,
		Prompt:     fullPrompt,
		Params:     paramsJSON,
		AccountIDs: accountIDs,
		RetryCount: 0,
		CreatedAt:  time.Now().Unix(),
	}

	if err := s.queue.Enqueue(ctx, queueTask); err != nil {
		if markErr := s.taskRepo.MarkQueueFailed(ctx, taskID, "任务入队失败: "+err.Error()); markErr != nil {
			slog.Warn("mark queue failed task failed", "task_id", taskID, "err", markErr)
			return "", fmt.Errorf("任务提交失败，请稍后重试")
		}
		s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
		if failedTask, loadErr := s.taskRepo.GetByTaskID(ctx, taskID); loadErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(ctx, failedTask); statErr != nil {
				slog.Warn("upsert daily stat failed after queue fail", "task_id", taskID, "err", statErr)
			}
		} else {
			slog.Warn("load task failed after queue fail", "task_id", taskID, "err", loadErr)
		}
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	s.publishTaskStatusEvent(ctx, taskID, tasks.StatusPending)
	return taskID, nil
}

func (s *AccountService) CheckSubmitTaskReadiness(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("请求参数错误")
	}
	_, _, featureSlug, _, err := s.prepareSubmitTask(ctx, userID, req)
	if err != nil {
		return "", err
	}
	return featureSlug, nil
}

func (s *AccountService) prepareSubmitTask(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) ([]*ApiAccount, map[string]interface{}, string, string, error) {
	if err := validateSubmitTaskPrompt(req.Prompt, maxSubmitTaskPromptBytes, "提示词"); err != nil {
		return nil, nil, "", "", err
	}
	if err := s.validateSubmitTaskImageURLs(ctx, userID, req.ImageURLs); err != nil {
		return nil, nil, "", "", err
	}

	accounts, err := s.accountRepo.GetAllEnabled(ctx)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("任务提交失败，请稍后重试")
	}
	if len(accounts) == 0 {
		return nil, nil, "", "", errors.New(noAvailableAPIAccountMessage)
	}

	params := make(map[string]interface{}, len(req.Params)+2)
	for key, value := range req.Params {
		params[key] = value
	}
	if req.Feature != "" {
		params["feature"] = req.Feature
	}
	if len(req.ImageURLs) > 0 {
		params["image_urls"] = req.ImageURLs
	}
	normalizeSubmitTaskMediaParams(params)
	req.Params = params
	if err := s.validateSubmitTaskMediaParams(ctx, userID, req.Params); err != nil {
		return nil, nil, "", "", err
	}

	featureSlug := ""
	if req.Params != nil {
		featureSlug, err = workflow.ResolveWorkflowFeatureSlug(req.Params)
		if err != nil {
			return nil, nil, "", "", err
		}
		if featureSlug != "" {
			req.Params["feature"] = featureSlug
		}
	}
	accounts = filterAccountsByFeature(accounts, featureSlug)
	if len(accounts) == 0 {
		return nil, nil, "", "", errors.New(noAvailableAPIAccountMessage)
	}

	fullPrompt := applyTaskSystemPrompt(req.Prompt, req.Prompt, featureSlug, req.Params, func(slug string) (string, error) {
		if s.systemConfigRepo == nil {
			return "", fmt.Errorf("system config repo 未配置")
		}
		return s.systemConfigRepo.Get(ctx, prompts.FeaturePromptConfigKey(slug))
	})
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		return nil, nil, "", "", err
	}
	if err := validateSubmitTaskPrompt(fullPrompt, maxSubmitTaskFinalPromptBytes, "最终提示词"); err != nil {
		return nil, nil, "", "", err
	}

	return accounts, req.Params, featureSlug, fullPrompt, nil
}

func (s *AccountService) GetTask(ctx context.Context, taskID string) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	return task, nil
}

func (s *AccountService) GetUserTask(ctx context.Context, taskID string, userID int64) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	return task, nil
}

func (s *AccountService) ListTasks(ctx context.Context, req *tasks.TaskListRequest) ([]*tasks.Task, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	req.Keyword = strings.TrimSpace(req.Keyword)
	if len(req.Keyword) > maxTaskKeywordLength {
		req.Keyword = req.Keyword[:maxTaskKeywordLength]
	}
	items, total, err := s.taskRepo.List(ctx, req.UserID, req.AccountID, req.Status, req.Keyword, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}
	s.attachOutputsForTasks(ctx, items)
	return items, total, nil
}

func (s *AccountService) DeleteUserTask(ctx context.Context, taskID string, userID int64) error {
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return err
	}
	if s.cancelStore == nil {
		return fmt.Errorf("task cancellation store not configured")
	}
	if err := s.cancelStore.MarkCancelled(ctx, taskID, taskCancelTTL); err != nil {
		return err
	}
	if s.taskOutputCleaner != nil {
		if err := s.taskOutputCleaner.DeleteTaskOutputsByUserTask(ctx, task.UserID, task.TaskID); err != nil {
			return err
		}
	}
	return s.taskRepo.DeleteByTaskIDAndUserID(ctx, taskID, userID)
}

func (s *AccountService) GetUserTasks(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	items, total, err := s.taskRepo.GetByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	s.attachOutputsForTasks(ctx, items)
	return items, total, nil
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
