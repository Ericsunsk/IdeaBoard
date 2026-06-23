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

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/taskqueue"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/workflow"
)

type Service struct {
	accountRepo       AccountStore
	taskRepo          TaskStore
	submissionStore   TaskSubmissionStore
	cancelStore       taskqueue.TaskCancellationStore
	featurePrompts    FeaturePromptStore
	uploadBoards      storage.UploadBoardAccessResolver
	uploadOwners      storage.UploadOwnerStorageUIDResolver
	taskOutputCleaner TaskOutputObjectCleaner
	eventHub          *EventHub
	taskEvents        events.TaskStatusPublisher
	publicURLs        settings.PublicURLSettingProvider
}

type TaskOutputObjectCleaner interface {
	DeleteTaskOutputsByUserTask(ctx context.Context, userID int64, taskID string) error
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
	ListOutputsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]tasks.TaskOutput, error)
}

type TaskSubmissionStore interface {
	SubmitTask(ctx context.Context, task *tasks.Task, queueTask *tasks.TaskPayload) error
}

type FeaturePromptStore interface {
	GetPrompt(ctx context.Context, slug string) (string, error)
}

type ServiceDeps struct {
	AccountRepo       AccountStore
	TaskRepo          TaskStore
	SubmissionStore   TaskSubmissionStore
	CancelStore       taskqueue.TaskCancellationStore
	FeaturePrompts    FeaturePromptStore
	UploadBoards      storage.UploadBoardAccessResolver
	UploadOwners      storage.UploadOwnerStorageUIDResolver
	TaskOutputCleaner TaskOutputObjectCleaner
	PublicURLs        settings.PublicURLSettingProvider
}

type ServiceOptions struct {
	EventHub   *EventHub
	TaskEvents events.TaskStatusPublisher
}

const taskCancelTTL = 24 * time.Hour
const maxTaskKeywordLength = 200
const maxSubmitTaskPromptBytes = 8 * 1024
const maxSubmitTaskFinalPromptBytes = 32 * 1024
const maxSubmitTaskImageURLs = 16
const maxSubmitTaskImageURLBytes = 4096
const maxSubmitTaskParamsBytes = 2 * 1024 * 1024
const maxSubmitTaskParamsDepth = 24
const maxSubmitTaskMediaDataURLBytes = 10 * 1024 * 1024
const noAvailableAPIAccountMessage = "没有可用的 API 账号，请先添加账号"

func NewService(deps ServiceDeps, opts ServiceOptions) *Service {
	return &Service{
		accountRepo:       deps.AccountRepo,
		taskRepo:          deps.TaskRepo,
		submissionStore:   deps.SubmissionStore,
		cancelStore:       deps.CancelStore,
		featurePrompts:    deps.FeaturePrompts,
		uploadBoards:      deps.UploadBoards,
		uploadOwners:      deps.UploadOwners,
		taskOutputCleaner: deps.TaskOutputCleaner,
		eventHub:          opts.EventHub,
		taskEvents:        opts.TaskEvents,
		publicURLs:        deps.PublicURLs,
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
	apiAccounts, typedParams, sourceParams, featureSlug, fullPrompt, err := s.prepareSubmitTask(ctx, userID, req)
	if err != nil {
		return "", err
	}

	taskID := generateSecureTaskID("task")

	paramsJSON, err := marshalWorkflowTaskParams(typedParams, sourceParams)
	if err != nil {
		return "", err
	}

	requestedModel := typedParams.Model

	task := &tasks.Task{
		TaskID:         taskID,
		UserID:         userID,
		Prompt:         fullPrompt,
		UserPrompt:     req.Prompt,
		Params:         paramsJSON,
		FeatureSlug:    featureSlug,
		RequestedModel: requestedModel,
		Status:         tasks.StatusPending,
	}

	accountIDs := make([]int64, len(apiAccounts))
	for i, acc := range apiAccounts {
		accountIDs[i] = acc.ID
	}

	queueTask := &tasks.TaskPayload{
		TaskID:     taskID,
		UserID:     userID,
		Prompt:     fullPrompt,
		Params:     paramsJSON,
		AccountIDs: accountIDs,
		RetryCount: 0,
		CreatedAt:  time.Now().Unix(),
	}

	if s.submissionStore == nil {
		return "", fmt.Errorf("任务提交服务未配置")
	}
	if err := s.submissionStore.SubmitTask(ctx, task, queueTask); err != nil {
		slog.Warn("submit task transaction failed", "task_id", taskID, "err", err)
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	s.publishTaskStatusEvent(ctx, taskID, tasks.StatusPending)
	return taskID, nil
}

func (s *Service) CheckSubmitTaskReadiness(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("请求参数错误")
	}
	_, _, _, featureSlug, _, err := s.prepareSubmitTask(ctx, userID, req)
	if err != nil {
		return "", err
	}
	return featureSlug, nil
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
	if err := workflow.NormalizeStructuredPromptContractParams(req.Params, featureSlug); err != nil {
		return nil, nil, nil, "", "", err
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
	if len(apiAccounts) == 0 {
		return nil, nil, nil, "", "", errors.New(noAvailableAPIAccountMessage)
	}

	fullPrompt := applyTaskSystemPrompt(req.Prompt, req.Prompt, featureSlug, req.Params, func(slug string) (string, error) {
		if s.featurePrompts == nil {
			return "", fmt.Errorf("feature prompt repo 未配置")
		}
		return s.featurePrompts.GetPrompt(ctx, slug)
	})
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		return nil, nil, nil, "", "", err
	}
	if err := validateSubmitTaskPrompt(fullPrompt, maxSubmitTaskFinalPromptBytes, "最终提示词"); err != nil {
		return nil, nil, nil, "", "", err
	}

	return apiAccounts, typedParams, req.Params, featureSlug, fullPrompt, nil
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
