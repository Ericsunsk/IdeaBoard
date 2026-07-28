package taskservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/tasksubmission"
)

type Service struct {
	taskIntake          TaskIntake
	taskRepo            TaskStore
	submissionStore     TaskSubmissionStore
	cancelStore         taskqueue.TaskCancellationStore
	taskOutputCleaner   taskmedia.TaskOutputCleaner
	eventHub            *EventHub
	taskEvents          events.TaskStatusPublisher
	taskBilling         *taskbilling.Service
	adminBillingHistory AdminBillingHistoryStore
	adminTaskTrace      AdminTaskTraceStore
	adminTaskListRead   AdminTaskListReadStore
	billableItems       BillableItemLookup
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
	SubmitTask(ctx context.Context, canonical tasksubmission.CanonicalRequest) (tasks.TaskSubmissionResult, error)
}

type AdminBillingHistoryStore interface {
	ListAdminBillingSnapshots(context.Context, []string) (map[string]tasks.AdminBillingSnapshot, error)
}

type AdminTaskTraceStore interface {
	ListAdminTaskAttempts(context.Context, []string) (map[string][]tasks.AdminTaskAttempt, error)
}

type AdminTaskListReadStore interface {
	ListAdminTaskLatestExecutions(context.Context, []string) (map[string]tasks.AdminTaskExecutionSnapshot, error)
}

type AdminTaskMonitorExecutionSnapshot = tasks.AdminTaskExecutionSnapshot

type AdminTaskMonitorListItem struct {
	TaskID          string                             `json:"task_id"`
	UserID          int64                              `json:"user_id"`
	Prompt          string                             `json:"prompt"`
	Priority        int                                `json:"priority"`
	Status          string                             `json:"status"`
	ErrorMsg        string                             `json:"error_msg"`
	Result          string                             `json:"result"`
	CreatedAt       time.Time                          `json:"created_at"`
	CompletedAt     *time.Time                         `json:"completed_at"`
	LatestExecution *AdminTaskMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
}

type AdminTaskMonitorDetail struct {
	TaskID            string                             `json:"task_id"`
	CorrelationID     string                             `json:"correlation_id"`
	UserID            int64                              `json:"user_id"`
	Prompt            string                             `json:"prompt"`
	UserPrompt        string                             `json:"user_prompt"`
	Params            string                             `json:"params"`
	FeatureSlug       string                             `json:"feature_slug"`
	Priority          int                                `json:"priority"`
	Status            string                             `json:"status"`
	RetryCount        int                                `json:"retry_count"`
	ErrorMsg          string                             `json:"error_msg"`
	Result            string                             `json:"result"`
	Outputs           []tasks.TaskOutput                 `json:"outputs,omitempty"`
	ExecutionMetrics  *tasks.TaskExecutionMetrics        `json:"execution_metrics,omitempty"`
	CreatedAt         time.Time                          `json:"created_at"`
	UpdatedAt         time.Time                          `json:"updated_at"`
	CompletedAt       *time.Time                         `json:"completed_at"`
	CancelledAt       *time.Time                         `json:"cancelled_at"`
	CancelledBy       *int64                             `json:"cancelled_by"`
	CancelReason      string                             `json:"cancel_reason"`
	Billing           *tasks.AdminBillingSnapshot        `json:"billing,omitempty"`
	LatestExecution   *AdminTaskMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
	ExecutionAttempts []tasks.AdminTaskAttempt           `json:"execution_attempts"`
}

type BillableItemLookup interface {
	Contains(string) bool
}

type ServiceDeps struct {
	TaskIntake          TaskIntake
	TaskRepo            TaskStore
	SubmissionStore     TaskSubmissionStore
	CancelStore         taskqueue.TaskCancellationStore
	TaskOutputCleaner   taskmedia.TaskOutputCleaner
	TaskBilling         *taskbilling.Service
	AdminBillingHistory AdminBillingHistoryStore
	AdminTaskTrace      AdminTaskTraceStore
	AdminTaskListRead   AdminTaskListReadStore
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

var (
	errTaskIntakeUnavailable          = errors.New("task intake not configured")
	errTaskSubmissionStoreUnavailable = errors.New("task submission store not configured")
	errTaskBillingUnavailable         = errors.New("task billing service not configured")
)

func NewService(deps ServiceDeps, opts ServiceOptions) *Service {
	return &Service{
		taskIntake:          deps.TaskIntake,
		taskRepo:            deps.TaskRepo,
		submissionStore:     deps.SubmissionStore,
		cancelStore:         deps.CancelStore,
		taskOutputCleaner:   deps.TaskOutputCleaner,
		eventHub:            opts.EventHub,
		taskEvents:          opts.TaskEvents,
		taskBilling:         deps.TaskBilling,
		adminBillingHistory: deps.AdminBillingHistory,
		adminTaskTrace:      deps.AdminTaskTrace,
		adminTaskListRead:   deps.AdminTaskListRead,
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
	if s == nil || s.taskIntake == nil {
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", errTaskIntakeUnavailable)
	}
	canonical, err := s.taskIntake.Prepare(ctx, userID, req)
	if err != nil {
		return "", err
	}

	if s.submissionStore == nil {
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", errTaskSubmissionStoreUnavailable)
	}
	result, err := s.submissionStore.SubmitTask(ctx, canonical)
	if err != nil {
		slog.Warn("submit task transaction failed", "user_id", userID, "feature", canonical.Feature(), "err", err)
		if errors.Is(err, taskbilling.ErrInsufficientCredits) {
			return "", taskbilling.ErrInsufficientCredits
		}
		if errors.Is(err, taskbilling.ErrPricingUnavailable) {
			return "", taskbilling.ErrPricingUnavailable
		}
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", err)
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
	if s == nil || s.taskIntake == nil {
		return SubmitTaskReadiness{}, newSubmissionError(
			"submission_failed", "任务提交检查暂不可用，请稍后重试", errTaskIntakeUnavailable,
		)
	}
	canonical, err := s.taskIntake.Prepare(ctx, userID, req)
	if err != nil {
		return SubmitTaskReadiness{}, err
	}
	result := SubmitTaskReadiness{Feature: canonical.Feature()}
	billingRequest, err := canonical.BillingRequest(billingQuantity)
	if err != nil {
		return SubmitTaskReadiness{}, err
	}
	if billingRequest == nil {
		return result, nil
	}
	if s.taskBilling == nil {
		return SubmitTaskReadiness{}, newSubmissionError(
			"submission_failed",
			"任务提交检查暂不可用，请稍后重试",
			errTaskBillingUnavailable,
		)
	}
	readiness, err := s.taskBilling.Readiness(ctx, userID, *billingRequest)
	if err != nil {
		return SubmitTaskReadiness{}, err
	}
	result.Billing = &readiness
	return result, nil
}

func (s *Service) GetTask(ctx context.Context, taskID string) (*AdminTaskMonitorDetail, error) {
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	billing, err := s.loadAdminBillingSnapshots(ctx, []string{task.TaskID})
	if err != nil {
		return nil, err
	}
	traces, err := s.loadAdminTaskAttempts(ctx, []string{task.TaskID})
	if err != nil {
		return nil, err
	}
	billingSnapshot, hasBilling := billing[task.TaskID]
	return adminTaskMonitorDetailFromTask(task, billingSnapshot, hasBilling, traces[task.TaskID]), nil
}

func (s *Service) GetUserTask(ctx context.Context, taskID string, userID int64) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	s.attachOutputsForTasks(ctx, []*tasks.Task{task})
	return task, nil
}

func (s *Service) ListTasks(ctx context.Context, req *tasks.TaskListRequest) ([]AdminTaskMonitorListItem, tasks.TaskListCounts, error) {
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
	latest, err := s.loadAdminTaskLatestExecutions(ctx, taskIDsFromTasks(items))
	if err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	result := make([]AdminTaskMonitorListItem, 0, len(items))
	for _, item := range items {
		latestExecution, hasLatestExecution := latest[item.TaskID]
		result = append(result, adminTaskMonitorListItemFromTask(item, latestExecution, hasLatestExecution))
	}
	return result, counts, nil
}

func (s *Service) loadAdminTaskLatestExecutions(ctx context.Context, taskIDs []string) (map[string]tasks.AdminTaskExecutionSnapshot, error) {
	if s.adminTaskListRead == nil || len(taskIDs) == 0 {
		return map[string]tasks.AdminTaskExecutionSnapshot{}, nil
	}
	latestByTaskID, err := s.adminTaskListRead.ListAdminTaskLatestExecutions(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("load Admin Task latest executions: %w", err)
	}
	return latestByTaskID, nil
}

func (s *Service) loadAdminBillingSnapshots(ctx context.Context, taskIDs []string) (map[string]tasks.AdminBillingSnapshot, error) {
	if s.adminBillingHistory == nil || len(taskIDs) == 0 {
		return map[string]tasks.AdminBillingSnapshot{}, nil
	}
	snapshots, err := s.adminBillingHistory.ListAdminBillingSnapshots(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("load Admin Task Billing history: %w", err)
	}
	for taskID, snapshot := range snapshots {
		if s.billableItems != nil {
			snapshot.FeatureExists = s.billableItems.Contains(snapshot.SKU)
		}
		snapshots[taskID] = snapshot
	}
	return snapshots, nil
}

func (s *Service) loadAdminTaskAttempts(ctx context.Context, taskIDs []string) (map[string][]tasks.AdminTaskAttempt, error) {
	if s.adminTaskTrace == nil || len(taskIDs) == 0 {
		return map[string][]tasks.AdminTaskAttempt{}, nil
	}
	attemptsByTaskID, err := s.adminTaskTrace.ListAdminTaskAttempts(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("load Admin Task execution trace: %w", err)
	}
	return attemptsByTaskID, nil
}

func taskIDsFromTasks(items []*tasks.Task) []string {
	taskIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			taskIDs = append(taskIDs, item.TaskID)
		}
	}
	return taskIDs
}

func adminTaskMonitorExecutionSnapshotFromAttempt(attempt tasks.AdminTaskAttempt) *AdminTaskMonitorExecutionSnapshot {
	return &AdminTaskMonitorExecutionSnapshot{
		AccountID:            attempt.AccountID,
		ProviderEndpointID:   attempt.ProviderEndpointID,
		ProviderEndpointName: attempt.ProviderEndpointName,
		ResolvedModel:        attempt.ResolvedModel,
		APIType:              attempt.APIType,
	}
}

func adminTaskMonitorListItemFromTask(task *tasks.Task, latestExecution tasks.AdminTaskExecutionSnapshot, hasLatestExecution bool) AdminTaskMonitorListItem {
	if task == nil {
		return AdminTaskMonitorListItem{}
	}
	item := AdminTaskMonitorListItem{
		TaskID:      task.TaskID,
		UserID:      task.UserID,
		Prompt:      task.Prompt,
		Priority:    task.Priority,
		Status:      task.Status,
		ErrorMsg:    task.ErrorMsg,
		Result:      task.Result,
		CreatedAt:   task.CreatedAt,
		CompletedAt: task.CompletedAt,
	}
	if hasLatestExecution {
		item.LatestExecution = &latestExecution
	}
	return item
}

func adminTaskMonitorDetailFromTask(task *tasks.Task, billing tasks.AdminBillingSnapshot, hasBilling bool, attempts []tasks.AdminTaskAttempt) *AdminTaskMonitorDetail {
	if task == nil {
		return nil
	}
	if attempts == nil {
		attempts = []tasks.AdminTaskAttempt{}
	}
	detail := &AdminTaskMonitorDetail{
		TaskID:            task.TaskID,
		CorrelationID:     task.EffectiveCorrelationID(),
		UserID:            task.UserID,
		Prompt:            task.Prompt,
		UserPrompt:        task.UserPrompt,
		Params:            task.Params,
		FeatureSlug:       task.FeatureSlug,
		Priority:          task.Priority,
		Status:            task.Status,
		RetryCount:        task.RetryCount,
		ErrorMsg:          task.ErrorMsg,
		Result:            task.Result,
		Outputs:           task.Outputs,
		ExecutionMetrics:  task.ExecutionMetrics,
		CreatedAt:         task.CreatedAt,
		UpdatedAt:         task.UpdatedAt,
		CompletedAt:       task.CompletedAt,
		CancelledAt:       task.CancelledAt,
		CancelledBy:       task.CancelledBy,
		CancelReason:      task.CancelReason,
		ExecutionAttempts: attempts,
	}
	if hasBilling {
		detail.Billing = &billing
	}
	if len(attempts) > 0 {
		detail.LatestExecution = adminTaskMonitorExecutionSnapshotFromAttempt(attempts[len(attempts)-1])
	}
	return detail
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
