package taskservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

type Service struct {
	accountRepo        AccountStore
	taskRepo           TaskStore
	submissionStore    TaskSubmissionStore
	cancelStore        taskqueue.TaskCancellationStore
	featurePrompts     FeaturePromptStore
	mediaValidator     taskmedia.Validator
	taskOutputCleaner  taskmedia.TaskOutputCleaner
	generationSettings settings.GoogleNativeGenerationSettingProvider
	eventHub           *EventHub
	taskEvents         events.TaskStatusPublisher
}

type AccountStore interface {
	GetAllEnabled(ctx context.Context) ([]*accounts.ApiAccount, error)
}

type TaskStore interface {
	GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error)
	GetByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) (*tasks.Task, error)
	List(ctx context.Context, userID int64, accountID int64, status string, keyword string, page, pageSize int) ([]*tasks.Task, tasks.TaskListCounts, error)
	GetByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error)
	DeleteByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) error
	MarkTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	ListOutputsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]tasks.TaskOutput, error)
}

type TaskSubmissionStore interface {
	SubmitTask(ctx context.Context, task *tasks.Task, queueTask *tasks.TaskPayload) (tasks.TaskSubmissionResult, error)
}

type FeaturePromptStore interface {
	GetPrompt(ctx context.Context, slug string) (string, error)
	GetPromptRuntimeConfig(ctx context.Context, slug string) (prompts.FeaturePromptRuntimeConfig, error)
}

type ServiceDeps struct {
	AccountRepo        AccountStore
	TaskRepo           TaskStore
	SubmissionStore    TaskSubmissionStore
	CancelStore        taskqueue.TaskCancellationStore
	FeaturePrompts     FeaturePromptStore
	MediaValidator     taskmedia.Validator
	TaskOutputCleaner  taskmedia.TaskOutputCleaner
	GenerationSettings settings.GoogleNativeGenerationSettingProvider
}

type ServiceOptions struct {
	EventHub   *EventHub
	TaskEvents events.TaskStatusPublisher
}

const taskCancelTTL = 24 * time.Hour
const userDeletedTaskCancelReason = "用户删除节点"
const maxTaskKeywordLength = 200
const maxSubmitTaskPromptBytes = 8 * 1024
const maxSubmitTaskFinalPromptBytes = 32 * 1024
const maxSubmitTaskImageURLs = 16
const maxSubmitTaskImageURLBytes = 4096
const maxSubmitTaskClientActionIDBytes = 128
const maxSubmitTaskParamsBytes = 2 * 1024 * 1024
const maxSubmitTaskParamsDepth = 24
const noAvailableAPIAccountMessage = "没有可用的 API 账号，请先添加账号"

func NewService(deps ServiceDeps, opts ServiceOptions) *Service {
	return &Service{
		accountRepo:        deps.AccountRepo,
		taskRepo:           deps.TaskRepo,
		submissionStore:    deps.SubmissionStore,
		cancelStore:        deps.CancelStore,
		featurePrompts:     deps.FeaturePrompts,
		mediaValidator:     deps.MediaValidator,
		taskOutputCleaner:  deps.TaskOutputCleaner,
		generationSettings: deps.GenerationSettings,
		eventHub:           opts.EventHub,
		taskEvents:         opts.TaskEvents,
	}
}

func (s *Service) EventHub() *EventHub {
	return s.eventHub
}

func (s *Service) publishTaskStatusEvent(ctx context.Context, taskID, status string) {
	events.PublishTaskStatusLogWarn(ctx, s.taskEvents, taskID, status)
}

func (s *Service) SubmitTask(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("请求参数错误")
	}
	clientActionID, err := normalizeSubmitTaskClientActionID(req.ClientActionID)
	if err != nil {
		return "", err
	}
	priority := tasks.EffectiveTaskPriority(req.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return "", validation.New("priority 仅支持 1-4")
	}
	apiAccounts, typedParams, sourceParams, featureSlug, fullPrompt, err := s.prepareSubmitTask(ctx, userID, req)
	if err != nil {
		return "", err
	}

	taskID := generateSecureTaskID("task")
	correlationID := tasks.CorrelationIDForTaskID(taskID)

	paramsJSON, err := marshalWorkflowTaskParams(typedParams, sourceParams)
	if err != nil {
		return "", err
	}

	requestedModel := typedParams.Model

	task := &tasks.Task{
		TaskID:         taskID,
		CorrelationID:  correlationID,
		UserID:         userID,
		ClientActionID: clientActionID,
		Prompt:         fullPrompt,
		UserPrompt:     req.Prompt,
		Params:         paramsJSON,
		FeatureSlug:    featureSlug,
		RequestedModel: requestedModel,
		Priority:       priority,
		Status:         tasks.StatusPending,
	}

	accountIDs := make([]int64, len(apiAccounts))
	for i, acc := range apiAccounts {
		accountIDs[i] = acc.ID
	}

	queueTask := &tasks.TaskPayload{
		TaskID:        taskID,
		CorrelationID: correlationID,
		UserID:        userID,
		Prompt:        fullPrompt,
		Params:        paramsJSON,
		Priority:      priority,
		AccountIDs:    accountIDs,
		RetryCount:    0,
		CreatedAt:     time.Now().Unix(),
	}

	if s.submissionStore == nil {
		return "", fmt.Errorf("任务提交服务未配置")
	}
	result, err := s.submissionStore.SubmitTask(ctx, task, queueTask)
	if err != nil {
		slog.Warn("submit task transaction failed", "task_id", taskID, "correlation_id", correlationID, "err", err)
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	if result.Created {
		s.publishTaskStatusEvent(ctx, result.TaskID, tasks.StatusPending)
	}
	return result.TaskID, nil
}

func (s *Service) CheckSubmitTaskReadiness(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("请求参数错误")
	}
	if _, err := normalizeSubmitTaskClientActionID(req.ClientActionID); err != nil {
		return "", err
	}
	_, _, _, featureSlug, _, err := s.prepareSubmitTask(ctx, userID, req)
	if err != nil {
		return "", err
	}
	return featureSlug, nil
}

func normalizeSubmitTaskClientActionID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > maxSubmitTaskClientActionIDBytes {
		return "", fmt.Errorf("client_action_id 过长")
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '_', '.', ':':
			continue
		default:
			return "", fmt.Errorf("client_action_id 只能包含字母、数字、横线、下划线、点或冒号")
		}
	}
	return value, nil
}

func (s *Service) prepareSubmitTask(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) ([]*accounts.ApiAccount, *tasks.WorkflowTaskParams, map[string]interface{}, string, string, error) {
	if err := validateSubmitTaskPrompt(req.Prompt, maxSubmitTaskPromptBytes, "提示词"); err != nil {
		return nil, nil, nil, "", "", err
	}
	if err := s.validateSubmitTaskImageURLs(ctx, userID, req.ImageURLs); err != nil {
		return nil, nil, nil, "", "", err
	}

	apiAccounts, err := s.accountRepo.GetAllEnabled(ctx)
	if err != nil {
		return nil, nil, nil, "", "", fmt.Errorf("任务提交失败，请稍后重试")
	}
	if len(apiAccounts) == 0 {
		return nil, nil, nil, "", "", errors.New(noAvailableAPIAccountMessage)
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
		return nil, nil, nil, "", "", err
	}

	featureSlug := ""
	if req.Params != nil {
		featureSlug, err = workflow.ResolveWorkflowFeatureSlug(req.Params)
		if err != nil {
			return nil, nil, nil, "", "", err
		}
		if featureSlug != "" {
			req.Params["feature"] = featureSlug
		}
	}

	var runtimePromptConfig *prompts.FeaturePromptRuntimeConfig
	if s.featurePrompts != nil {
		if config, err := s.featurePrompts.GetPromptRuntimeConfig(ctx, featureSlug); err == nil {
			runtimePromptConfig = &config
			applyFeaturePromptModelDefaults(req.Params, config.ModelDefaults)
		}
	}
	if err := workflow.NormalizeStructuredPromptContractParams(req.Params, featureSlug); err != nil {
		return nil, nil, nil, "", "", err
	}
	if featureSlug == workflow.FeatureSlugPatternGen {
		if err := workflow.ValidatePatternGenSubmitContract(req.Prompt, req.Params); err != nil {
			return nil, nil, nil, "", "", err
		}
	}
	if featureSlug == workflow.FeatureSlugStyleFusion {
		if err := normalizeStyleFusionParams(req.Params); err != nil {
			return nil, nil, nil, "", "", err
		}
	}
	typedParams, err := workflowTaskParamsFromMap(req.Params, featureSlug)
	if err != nil {
		return nil, nil, nil, "", "", err
	}
	apiAccounts = accounts.FilterAccountsForRequest(apiAccounts, featureSlug, req.Params)
	apiAccounts, err = s.filterGoogleNativeAccountsForSubmit(ctx, userID, apiAccounts)
	if err != nil {
		return nil, nil, nil, "", "", err
	}
	if len(apiAccounts) == 0 {
		return nil, nil, nil, "", "", errors.New(noAvailableAPIAccountMessage)
	}

	fullPrompt := req.Prompt
	if runtimePromptConfig != nil {
		fullPrompt = prompts.ApplyFeatureSystemPromptWithConfig(req.Prompt, req.Prompt, featureSlug, req.Params, *runtimePromptConfig)
	}
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		return nil, nil, nil, "", "", err
	}
	if err := validateSubmitTaskPrompt(fullPrompt, maxSubmitTaskFinalPromptBytes, "最终提示词"); err != nil {
		return nil, nil, nil, "", "", err
	}

	return apiAccounts, typedParams, req.Params, featureSlug, fullPrompt, nil
}

func (s *Service) filterGoogleNativeAccountsForSubmit(ctx context.Context, userID int64, apiAccounts []*accounts.ApiAccount) ([]*accounts.ApiAccount, error) {
	if len(apiAccounts) == 0 || s.generationSettings == nil {
		return apiAccounts, nil
	}
	hasGoogleNative := false
	for _, account := range apiAccounts {
		if accountUsesGoogleNativeAPI(account) {
			hasGoogleNative = true
			break
		}
	}
	if !hasGoogleNative {
		return apiAccounts, nil
	}

	setting, err := s.generationSettings.GetGoogleNativeGeneration(ctx)
	if err != nil {
		return nil, fmt.Errorf("生成策略设置读取失败，请稍后重试")
	}
	evaluation := settings.EvaluateGoogleNativeGeneration(setting, userID)
	if evaluation.Allowed {
		return apiAccounts, nil
	}

	if evaluation.Message != "" {
		return nil, errors.New(evaluation.Message)
	}
	return nil, errors.New(noAvailableAPIAccountMessage)
}

func accountUsesGoogleNativeAPI(account *accounts.ApiAccount) bool {
	if account == nil {
		return false
	}
	apiType, ok := workflow.ValidateProviderAPIType(account.ApiType, account.ProxyURL)
	return ok && workflow.IsGoogleNativeAPIType(apiType)
}

func applyFeaturePromptModelDefaults(params map[string]interface{}, defaults prompts.FeaturePromptModelDefaults) {
	if params == nil {
		return
	}
	if _, exists := params[paramkeys.Model]; exists {
		return
	}
	if defaults.Model != "" {
		params[paramkeys.Model] = defaults.Model
	}
}

func (s *Service) GetTask(ctx context.Context, taskID string) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	return task, nil
}

func (s *Service) GetUserTask(ctx context.Context, taskID string, userID int64) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	return task, nil
}

func (s *Service) ListTasks(ctx context.Context, req *tasks.TaskListRequest) ([]*tasks.Task, tasks.TaskListCounts, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	status, err := normalizeTaskListStatus(req.Status)
	if err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	req.Status = status
	req.Keyword = normalizeTaskListKeyword(req.Keyword)
	items, counts, err := s.taskRepo.List(ctx, req.UserID, req.AccountID, req.Status, req.Keyword, req.Page, req.PageSize)
	if err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	s.attachOutputsForTasks(ctx, items)
	return items, counts, nil
}

func normalizeTaskListStatus(status string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(status)); normalized {
	case "", tasks.StatusPending, tasks.StatusRunning, tasks.StatusCompleted, tasks.StatusFailed, tasks.StatusCancelled:
		return normalized, nil
	default:
		return "", validation.New("status 仅支持 pending、running、completed、failed 或 cancelled")
	}
}

func normalizeTaskListKeyword(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	if utf8.RuneCountInString(keyword) <= maxTaskKeywordLength {
		return keyword
	}
	runes := []rune(keyword)
	return string(runes[:maxTaskKeywordLength])
}

func (s *Service) DeleteUserTask(ctx context.Context, taskID string, userID int64) error {
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return err
	}
	// Cancel and delete the task row first (idempotent DB operations).
	// Object-storage cleanup runs best-effort afterward to avoid leaving
	// a half-deleted task entry when output files cannot be removed.
	if s.cancelStore == nil {
		return fmt.Errorf("task cancellation store not configured")
	}
	if err := s.cancelStore.MarkCancelled(ctx, taskID, taskCancelTTL); err != nil {
		return err
	}
	if err := s.taskRepo.DeleteByTaskIDAndUserID(ctx, taskID, userID); err != nil {
		return err
	}
	if s.taskOutputCleaner != nil {
		if err := s.taskOutputCleaner.DeleteTaskOutputsByUserTask(ctx, task.UserID, task.TaskID); err != nil {
			slog.Warn("delete task outputs after task row deleted", "task_id", taskID, "user_id", userID, "err", err)
		}
	}
	return nil
}

func (s *Service) CancelUserTask(ctx context.Context, taskID string, userID int64) error {
	taskID = strings.TrimSpace(taskID)
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return err
	}
	if tasks.IsTerminalStatus(task.Status) {
		return nil
	}

	if s.cancelStore == nil {
		return fmt.Errorf("task cancellation store not configured")
	}
	if err := s.cancelStore.MarkCancelled(ctx, task.TaskID, taskCancelTTL); err != nil {
		return err
	}

	now := time.Now()
	rows, err := s.taskRepo.MarkTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: now,
		CancelledBy: &userID,
		Reason:      userDeletedTaskCancelReason,
	}, nil)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, task.TaskID, tasks.StatusCancelled)
		return nil
	}

	latest, err := s.taskRepo.GetByTaskIDAndUserID(ctx, task.TaskID, userID)
	if err != nil {
		return err
	}
	if tasks.IsTerminalStatus(latest.Status) {
		return nil
	}
	return fmt.Errorf("task cancellation did not apply")
}

func (s *Service) GetUserTasks(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error) {
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

func generateSecureTaskID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
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
	return prompts.ApplyFeatureSystemPrompt(basePrompt, userPrompt, featureSlug, params, loadPrompt)
}

func validateExecutablePrompt(fullPrompt string) error {
	if strings.TrimSpace(fullPrompt) == "" {
		return fmt.Errorf("缺少提示词，请填写 Prompt 或配置后台 system prompt")
	}
	return nil
}
