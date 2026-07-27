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
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
	"ideaboard/api/internal/workflow/paramkeys"
)

type Service struct {
	accountRepo         AccountStore
	taskRepo            TaskStore
	submissionStore     TaskSubmissionStore
	cancelStore         taskqueue.TaskCancellationStore
	workflowFeatures    WorkflowFeatureStore
	mediaValidator      taskmedia.Validator
	taskOutputCleaner   taskmedia.TaskOutputCleaner
	generationSettings  settings.GoogleNativeGenerationSettingProvider
	eventHub            *EventHub
	taskEvents          events.TaskStatusPublisher
	taskBilling         *taskbilling.Service
	adminBillingHistory AdminBillingHistoryStore
	billableItems       BillableItemLookup
}

type AccountStore interface {
	GetAllEnabled(ctx context.Context) ([]*accounts.ApiAccount, error)
}

type TaskStore interface {
	GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error)
	GetByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) (*tasks.Task, error)
	ListByTaskIDs(ctx context.Context, taskIDs []string) ([]*tasks.Task, error)
	List(ctx context.Context, userID int64, accountID int64, status string, keyword string, page, pageSize int) ([]*tasks.Task, tasks.TaskListCounts, error)
	GetByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error)
	DeleteByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) error
	DeleteByTaskIDs(ctx context.Context, taskIDs []string) (int64, error)
	MarkTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) (int64, error)
	ListOutputsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]tasks.TaskOutput, error)
}

type TaskSubmissionStore interface {
	SubmitTask(ctx context.Context, task *tasks.Task, queueTask *tasks.TaskPayload) (tasks.TaskSubmissionResult, error)
}

type WorkflowFeatureStore interface {
	GetRuntimeConfig(ctx context.Context, slug string) (prompts.WorkflowFeatureRuntimeConfig, error)
}

type AdminBillingHistoryStore interface {
	ListAdminBillingSnapshots(context.Context, []string) (map[string]tasks.AdminBillingSnapshot, error)
}

type BillableItemLookup interface {
	Contains(string) bool
}

type ServiceDeps struct {
	AccountRepo         AccountStore
	TaskRepo            TaskStore
	SubmissionStore     TaskSubmissionStore
	CancelStore         taskqueue.TaskCancellationStore
	WorkflowFeatures    WorkflowFeatureStore
	MediaValidator      taskmedia.Validator
	TaskOutputCleaner   taskmedia.TaskOutputCleaner
	GenerationSettings  settings.GoogleNativeGenerationSettingProvider
	TaskBilling         *taskbilling.Service
	AdminBillingHistory AdminBillingHistoryStore
	BillableItems       BillableItemLookup
}

type ServiceOptions struct {
	EventHub   *EventHub
	TaskEvents events.TaskStatusPublisher
}

const taskCancelTTL = 24 * time.Hour
const userDeletedTaskCancelReason = "用户删除节点"
const maxTaskKeywordLength = 200
const maxDeleteTaskIDs = 200
const maxSubmitTaskPromptBytes = 8 * 1024
const maxSubmitTaskFinalPromptBytes = 32 * 1024
const maxSubmitTaskImageURLs = 16
const maxSubmitTaskImageURLBytes = 4096
const maxSubmitTaskClientActionIDBytes = 128
const maxSubmitTaskParamsBytes = 2 * 1024 * 1024
const maxSubmitTaskParamsDepth = 24
const providerUnavailableMessage = "生图服务暂不可用，请稍后重试"

var errNoAvailableAPIAccount = errors.New("no available API account")

func NewService(deps ServiceDeps, opts ServiceOptions) *Service {
	return &Service{
		accountRepo:         deps.AccountRepo,
		taskRepo:            deps.TaskRepo,
		submissionStore:     deps.SubmissionStore,
		cancelStore:         deps.CancelStore,
		workflowFeatures:    deps.WorkflowFeatures,
		mediaValidator:      deps.MediaValidator,
		taskOutputCleaner:   deps.TaskOutputCleaner,
		generationSettings:  deps.GenerationSettings,
		eventHub:            opts.EventHub,
		taskEvents:          opts.TaskEvents,
		taskBilling:         deps.TaskBilling,
		adminBillingHistory: deps.AdminBillingHistory,
		billableItems:       deps.BillableItems,
	}
}

func (s *Service) EventHub() *EventHub {
	return s.eventHub
}

func (s *Service) publishTaskStatusEvent(ctx context.Context, taskID, status string) {
	events.PublishTaskStatusLogWarn(ctx, s.taskEvents, taskID, status)
}

func (s *Service) SubmitTask(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	plan, err := s.PlanSubmission(ctx, userID, req)
	if err != nil {
		return "", err
	}

	taskID := generateSecureTaskID("task")
	correlationID := tasks.CorrelationIDForTaskID(taskID)

	task := &tasks.Task{
		TaskID:         taskID,
		CorrelationID:  correlationID,
		UserID:         userID,
		ClientActionID: plan.ClientActionID,
		Prompt:         plan.FullPrompt,
		UserPrompt:     plan.UserPrompt,
		Params:         plan.ParamsJSON,
		FeatureSlug:    plan.FeatureSlug,
		RequestedModel: plan.CanonicalModel,
		Priority:       plan.Priority,
		Status:         tasks.StatusPending,
	}
	if plan.BillingRequest != nil {
		task.BillingRequest = &tasks.TaskBillingRequest{
			Feature: plan.BillingRequest.Feature, Model: plan.BillingRequest.Model, Quantity: plan.BillingRequest.Quantity,
		}
	}

	queueTask := &tasks.TaskPayload{
		TaskID:        taskID,
		CorrelationID: correlationID,
		UserID:        userID,
		Prompt:        plan.FullPrompt,
		Params:        plan.ParamsJSON,
		Priority:      plan.Priority,
		AccountIDs:    plan.EligibleAccountIDs,
		RetryCount:    0,
		CreatedAt:     time.Now().Unix(),
	}

	if s.submissionStore == nil {
		return "", fmt.Errorf("任务提交服务未配置")
	}
	result, err := s.submissionStore.SubmitTask(ctx, task, queueTask)
	if err != nil {
		slog.Warn("submit task transaction failed", "task_id", taskID, "correlation_id", correlationID, "err", err)
		if errors.Is(err, taskbilling.ErrInsufficientCredits) {
			return "", taskbilling.ErrInsufficientCredits
		}
		if errors.Is(err, taskbilling.ErrPricingUnavailable) {
			return "", taskbilling.ErrPricingUnavailable
		}
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	if result.Created {
		s.publishTaskStatusEvent(ctx, result.TaskID, tasks.StatusPending)
	}
	return result.TaskID, nil
}

type SubmitTaskReadiness struct {
	Feature string
	Billing *taskbilling.Readiness
}

func (s *Service) CheckSubmitTaskReadiness(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest, billingQuantity int64) (SubmitTaskReadiness, error) {
	plan, err := s.PlanSubmission(ctx, userID, req)
	if err != nil {
		return SubmitTaskReadiness{}, err
	}
	result := SubmitTaskReadiness{Feature: plan.FeatureSlug}
	if plan.BillingRequest == nil {
		return result, nil
	}
	if billingQuantity <= 0 || billingQuantity > int64(maxSubmitTaskImageURLs) {
		return SubmitTaskReadiness{}, validation.New("billing_quantity 必须为 1-16")
	}
	billingRequest := *plan.BillingRequest
	billingRequest.Quantity = billingQuantity
	if s.taskBilling == nil {
		return SubmitTaskReadiness{}, fmt.Errorf("Idea计费服务暂不可用")
	}
	readiness, err := s.taskBilling.Readiness(ctx, userID, billingRequest)
	if err != nil {
		return SubmitTaskReadiness{}, err
	}
	result.Billing = &readiness
	return result, nil
}

func resolveTaskBillingRequest(feature, resolvedModel string) (*taskbilling.Request, error) {
	if !taskbilling.RequiresBilling(feature) {
		return nil, nil
	}
	request, err := taskbilling.AIImageRequest(feature, resolvedModel)
	if err != nil {
		return nil, err
	}
	return &request, nil
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

type SubmissionPlan struct {
	FeatureSlug        string
	CanonicalModel     string
	UserPrompt         string
	FullPrompt         string
	TypedParams        *tasks.WorkflowTaskParams
	ParamsJSON         string
	ClientActionID     string
	Priority           int
	EligibleAccountIDs []int64
	BillingRequest     *taskbilling.Request
}

func (s *Service) PlanSubmission(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (*SubmissionPlan, error) {
	if req == nil {
		return nil, validation.New("请求参数错误")
	}
	clientActionID, err := normalizeSubmitTaskClientActionID(req.ClientActionID)
	if err != nil {
		return nil, err
	}
	priority := tasks.EffectiveTaskPriority(req.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return nil, validation.New("priority 仅支持 1-4")
	}
	if err := validateSubmitTaskPrompt(req.Prompt, maxSubmitTaskPromptBytes, "提示词"); err != nil {
		return nil, err
	}
	if err := s.validateSubmitTaskImageURLs(ctx, userID, req.ImageURLs); err != nil {
		return nil, err
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
	if err := s.validateSubmitTaskMediaParams(ctx, userID, params); err != nil {
		return nil, err
	}

	featureSlug, err := workflow.ResolveWorkflowFeatureSlug(params)
	if err != nil {
		return nil, err
	}
	if featureSlug == "" {
		return nil, validation.New("feature 不能为空")
	}
	params["feature"] = featureSlug
	if s.workflowFeatures == nil {
		return nil, newSubmissionPlanError("workflow_configuration_unavailable", "该功能暂不可用", errors.New("workflow feature store is nil"))
	}
	runtimeConfig, err := s.workflowFeatures.GetRuntimeConfig(ctx, featureSlug)
	if err != nil {
		switch {
		case errors.Is(err, prompts.ErrWorkflowFeatureNotConfigured):
			return nil, newSubmissionPlanError("workflow_not_configured", "该功能尚未配置", err)
		case errors.Is(err, prompts.ErrWorkflowFeatureDisabled):
			return nil, newSubmissionPlanError("workflow_disabled", "该功能当前未启用", err)
		default:
			return nil, newSubmissionPlanError("workflow_configuration_unavailable", "该功能暂不可用", err)
		}
	}
	delete(params, paramkeys.Model)
	params[paramkeys.Model] = runtimeConfig.CanonicalModel
	if err := workflow.NormalizeStructuredPromptContractParams(params, featureSlug); err != nil {
		return nil, err
	}
	if featureSlug == workflow.FeatureSlugPatternGen {
		if err := workflow.ValidatePatternGenSubmitContract(req.Prompt, params); err != nil {
			return nil, err
		}
	}
	if featureSlug == workflow.FeatureSlugStyleFusion {
		if err := normalizeStyleFusionParams(params); err != nil {
			return nil, err
		}
	}
	typedParams, err := workflowTaskParamsFromMap(params, featureSlug)
	if err != nil {
		return nil, err
	}

	apiAccounts, err := s.accountRepo.GetAllEnabled(ctx)
	if err != nil {
		return nil, newSubmissionPlanError("provider_unavailable", providerUnavailableMessage, err)
	}
	apiAccounts = accounts.FilterAccountsForRequest(apiAccounts, featureSlug, params)
	if len(apiAccounts) == 0 {
		return nil, newSubmissionPlanError("workflow_model_unavailable", "该功能配置的模型当前不可用", errNoAvailableAPIAccount)
	}
	apiAccounts, err = s.filterGoogleNativeAccountsForSubmit(ctx, userID, apiAccounts)
	if err != nil {
		return nil, err
	}
	if len(apiAccounts) == 0 {
		return nil, newSubmissionPlanError("workflow_model_unavailable", "该功能配置的模型当前不可用", errNoAvailableAPIAccount)
	}

	fullPrompt := prompts.ApplyFeatureSystemPromptWithConfig(req.Prompt, req.Prompt, featureSlug, params, runtimeConfig)
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		return nil, err
	}
	if err := validateSubmitTaskPrompt(fullPrompt, maxSubmitTaskFinalPromptBytes, "最终提示词"); err != nil {
		return nil, err
	}
	billingRequest, err := resolveTaskBillingRequest(featureSlug, runtimeConfig.CanonicalModel)
	if err != nil {
		return nil, err
	}
	if billingRequest != nil && clientActionID == "" {
		return nil, validation.New("AI 生图任务必须提供 client_action_id")
	}
	paramsJSON, err := marshalWorkflowTaskParams(typedParams, params)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]int64, len(apiAccounts))
	for i, account := range apiAccounts {
		accountIDs[i] = account.ID
	}
	return &SubmissionPlan{
		FeatureSlug: featureSlug, CanonicalModel: runtimeConfig.CanonicalModel,
		UserPrompt: req.Prompt, FullPrompt: fullPrompt, TypedParams: typedParams, ParamsJSON: paramsJSON,
		ClientActionID: clientActionID, Priority: priority, EligibleAccountIDs: accountIDs,
		BillingRequest: billingRequest,
	}, nil
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
	return nil, errNoAvailableAPIAccount
}

func accountUsesGoogleNativeAPI(account *accounts.ApiAccount) bool {
	if account == nil {
		return false
	}
	apiType, ok := workflow.ValidateProviderAPIType(account.ApiType, account.ProxyURL)
	return ok && workflow.IsGoogleNativeAPIType(apiType)
}

func (s *Service) GetTask(ctx context.Context, taskID string) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	if err := s.attachAdminBillingSnapshots(ctx, []*tasks.Task{task}); err != nil {
		return nil, err
	}
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
	if err := s.attachAdminBillingSnapshots(ctx, items); err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	return items, counts, nil
}

func (s *Service) attachAdminBillingSnapshots(ctx context.Context, items []*tasks.Task) error {
	if s.adminBillingHistory == nil || len(items) == 0 {
		return nil
	}
	taskIDs := make([]string, 0, len(items))
	byTaskID := make(map[string]*tasks.Task, len(items))
	for _, task := range items {
		if task == nil || task.TaskID == "" {
			continue
		}
		taskIDs = append(taskIDs, task.TaskID)
		byTaskID[task.TaskID] = task
	}
	snapshots, err := s.adminBillingHistory.ListAdminBillingSnapshots(ctx, taskIDs)
	if err != nil {
		return fmt.Errorf("load Admin Task Billing history: %w", err)
	}
	for taskID, snapshot := range snapshots {
		if s.billableItems != nil {
			snapshot.FeatureExists = s.billableItems.Contains(snapshot.SKU)
		}
		value := snapshot
		byTaskID[taskID].BillingSnapshot = &value
	}
	return nil
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
	if !tasks.IsTerminalStatus(task.Status) {
		if _, err := s.taskRepo.MarkTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
			TaskID: taskID, CancelledAt: time.Now(), CancelledBy: &userID, Reason: userDeletedTaskCancelReason,
		}, nil); err != nil {
			return err
		}
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

type DeleteTasksResult struct {
	DeletedCount int64 `json:"deleted_count"`
}

func (s *Service) DeleteTasks(ctx context.Context, rawTaskIDs []string) (DeleteTasksResult, error) {
	taskIDs, err := normalizeDeleteTaskIDs(rawTaskIDs)
	if err != nil {
		return DeleteTasksResult{}, err
	}
	items, err := s.taskRepo.ListByTaskIDs(ctx, taskIDs)
	if err != nil {
		return DeleteTasksResult{}, err
	}
	activeTaskIDs := make([]string, 0, len(items))
	for _, task := range items {
		if task == nil || tasks.IsTerminalStatus(task.Status) {
			continue
		}
		activeTaskIDs = append(activeTaskIDs, task.TaskID)
	}
	if len(activeTaskIDs) > 0 {
		if s.cancelStore == nil {
			return DeleteTasksResult{}, fmt.Errorf("task cancellation store not configured")
		}
		for _, taskID := range activeTaskIDs {
			if err := s.cancelStore.MarkCancelled(ctx, taskID, taskCancelTTL); err != nil {
				return DeleteTasksResult{}, err
			}
			if _, err := s.taskRepo.MarkTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
				TaskID: taskID, CancelledAt: time.Now(), Reason: "管理员删除任务",
			}, nil); err != nil {
				return DeleteTasksResult{}, err
			}
		}
	}
	deletedCount, err := s.taskRepo.DeleteByTaskIDs(ctx, taskIDs)
	if err != nil {
		return DeleteTasksResult{}, err
	}
	s.cleanupDeletedTaskOutputs(ctx, items)
	return DeleteTasksResult{DeletedCount: deletedCount}, nil
}

func normalizeDeleteTaskIDs(rawTaskIDs []string) ([]string, error) {
	if len(rawTaskIDs) == 0 {
		return nil, validation.New("请选择要删除的任务")
	}
	taskIDs := make([]string, 0, len(rawTaskIDs))
	seen := make(map[string]struct{}, len(rawTaskIDs))
	for _, raw := range rawTaskIDs {
		taskID := strings.TrimSpace(raw)
		if taskID == "" {
			continue
		}
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		taskIDs = append(taskIDs, taskID)
	}
	if len(taskIDs) == 0 {
		return nil, validation.New("请选择要删除的任务")
	}
	if len(taskIDs) > maxDeleteTaskIDs {
		return nil, validation.New("单次最多删除 200 个任务")
	}
	return taskIDs, nil
}

func (s *Service) cleanupDeletedTaskOutputs(ctx context.Context, items []*tasks.Task) {
	if s.taskOutputCleaner == nil {
		return
	}
	for _, task := range items {
		if task == nil {
			continue
		}
		if err := s.taskOutputCleaner.DeleteTaskOutputsByUserTask(ctx, task.UserID, task.TaskID); err != nil {
			slog.Warn("delete task outputs after task row deleted", "task_id", task.TaskID, "user_id", task.UserID, "err", err)
		}
	}
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

func validateExecutablePrompt(fullPrompt string) error {
	if strings.TrimSpace(fullPrompt) == "" {
		return fmt.Errorf("缺少提示词，请填写 Prompt 或配置后台 system prompt")
	}
	return nil
}
