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
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/tasksubmission"

	"github.com/google/uuid"
)

type Service struct {
	taskIntake          TaskIntake
	taskRepo            TaskStore
	submissionStore     TaskSubmissionStore
	cancelStore         taskqueue.TaskCancellationStore
	eventHub            *EventHub
	taskEvents          events.TaskStatusPublisher
	taskBilling         *taskbilling.Service
	adminBillingHistory AdminBillingHistoryStore
	adminTaskTrace      AdminTaskTraceStore
	adminTaskListRead   AdminTaskListReadStore
	billableItems       BillableItemLookup
	userResolver        AdminTaskUserResolver
}

type TaskStore interface {
	GetByTaskID(ctx context.Context, taskID string) (*tasks.Task, error)
	GetByTaskIDAndUserID(ctx context.Context, taskID string, userID int64) (*tasks.Task, error)
	List(ctx context.Context, userID int64, accountID int64, status string, keyword string, page, pageSize int) ([]*tasks.Task, tasks.TaskListCounts, error)
	GetByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*tasks.Task, int64, error)
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

type AdminTaskUserResolver interface {
	ResolveUser(context.Context, int64) (uuid.UUID, string, error)
}

type AdminTaskMonitorExecutionSnapshot = tasks.AdminTaskExecutionSnapshot

type AdminTaskMonitorListItem struct {
	TaskID          string                             `json:"task_id"`
	UserPublicID    uuid.UUID                          `json:"user_public_id"`
	UserDisplayName string                             `json:"user_display_name"`
	Prompt          string                             `json:"prompt"`
	Priority        int                                `json:"priority"`
	Status          string                             `json:"status"`
	ErrorMsg        string                             `json:"error_msg"`
	Result          string                             `json:"result"`
	CreatedAt       time.Time                          `json:"created_at"`
	CompletedAt     *time.Time                         `json:"completed_at"`
	LatestExecution *AdminTaskMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
	internalUserID  int64
}

type AdminTaskMonitorDetail struct {
	TaskID            string                             `json:"task_id"`
	CorrelationID     string                             `json:"correlation_id"`
	UserPublicID      uuid.UUID                          `json:"user_public_id"`
	UserDisplayName   string                             `json:"user_display_name"`
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
	CancelReason      string                             `json:"cancel_reason"`
	Billing           *tasks.AdminBillingSnapshot        `json:"billing,omitempty"`
	LatestExecution   *AdminTaskMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
	ExecutionAttempts []tasks.AdminTaskAttempt           `json:"execution_attempts"`
	internalUserID    int64
}

type BillableItemLookup interface {
	Contains(string) bool
}

type ServiceDeps struct {
	TaskIntake          TaskIntake
	TaskRepo            TaskStore
	SubmissionStore     TaskSubmissionStore
	CancelStore         taskqueue.TaskCancellationStore
	TaskBilling         *taskbilling.Service
	AdminBillingHistory AdminBillingHistoryStore
	AdminTaskTrace      AdminTaskTraceStore
	AdminTaskListRead   AdminTaskListReadStore
	BillableItems       BillableItemLookup
	UserResolver        AdminTaskUserResolver
}

type ServiceOptions struct {
	EventHub   *EventHub
	TaskEvents events.TaskStatusPublisher
}

const taskCancelTTL = 24 * time.Hour
const userTaskCancelReason = "用户取消任务"
const adminTaskCancelReason = "管理员取消任务"
const maxTaskKeywordLength = 200

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
		eventHub:            opts.EventHub,
		taskEvents:          opts.TaskEvents,
		taskBilling:         deps.TaskBilling,
		adminBillingHistory: deps.AdminBillingHistory,
		adminTaskTrace:      deps.AdminTaskTrace,
		adminTaskListRead:   deps.AdminTaskListRead,
		billableItems:       deps.BillableItems,
		userResolver:        deps.UserResolver,
	}
}

func (s *Service) EventHub() *EventHub {
	return s.eventHub
}

func (s *Service) ResolveAdminTaskUserID(ctx context.Context, rawPublicID string) (int64, error) {
	if s == nil || s.userResolver == nil {
		return 0, validation.New("user_public_id 无效")
	}
	publicID, err := uuid.Parse(strings.TrimSpace(rawPublicID))
	if err != nil || publicID == uuid.Nil {
		return 0, validation.New("user_public_id 无效")
	}
	// The resolver is intentionally keyed by the internal id for read models;
	// public-id parsing and lookup stay at the HTTP boundary.
	return s.resolveUserIDByPublicID(ctx, publicID)
}

type adminTaskPublicIDResolver interface {
	ResolvePublicID(context.Context, string) (int64, error)
}

func (s *Service) resolveUserIDByPublicID(ctx context.Context, publicID uuid.UUID) (int64, error) {
	resolver, ok := s.userResolver.(adminTaskPublicIDResolver)
	if !ok {
		return 0, validation.New("user_public_id 无效")
	}
	return resolver.ResolvePublicID(ctx, publicID.String())
}

func (s *Service) publishTaskStatusEvent(ctx context.Context, taskID, status string) {
	events.PublishTaskStatusLogWarn(ctx, s.taskEvents, taskID, status)
}

func (s *Service) SubmitTask(ctx context.Context, userID int64, req *tasks.SubmitTaskRequest) (string, error) {
	if s == nil || s.taskIntake == nil {
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", errTaskIntakeUnavailable)
	}
	canonical, err := s.taskIntake.Compile(ctx, userID, req)
	if err != nil {
		return "", err
	}
	if s.submissionStore == nil {
		return "", newSubmissionError("submission_failed", "任务提交失败，请稍后重试", errTaskSubmissionStoreUnavailable)
	}
	canonical, err = s.taskIntake.PrepareMedia(ctx, userID, canonical)
	if err != nil {
		return "", err
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
		if errors.Is(err, taskbilling.ErrWalletRestricted) {
			return "", taskbilling.ErrWalletRestricted
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
	canonical, err := s.taskIntake.Compile(ctx, userID, req)
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
	detail := adminTaskMonitorDetailFromTask(task, billingSnapshot, hasBilling, traces[task.TaskID])
	if err := s.enrichAdminTaskDetail(ctx, detail); err != nil {
		return nil, err
	}
	return detail, nil
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
		row := adminTaskMonitorListItemFromTask(item, latestExecution, hasLatestExecution)
		if err := s.enrichAdminTaskListItem(ctx, &row); err != nil {
			return nil, tasks.TaskListCounts{}, err
		}
		result = append(result, row)
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
		TaskID:         task.TaskID,
		internalUserID: task.UserID,
		Prompt:         task.Prompt,
		Priority:       task.Priority,
		Status:         task.Status,
		ErrorMsg:       task.ErrorMsg,
		Result:         task.Result,
		CreatedAt:      task.CreatedAt,
		CompletedAt:    task.CompletedAt,
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
		internalUserID:    task.UserID,
		CorrelationID:     task.EffectiveCorrelationID(),
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

func (s *Service) resolveAdminTaskUser(ctx context.Context, userID int64) (uuid.UUID, string, error) {
	if s == nil || s.userResolver == nil || userID <= 0 {
		return uuid.Nil, "", nil
	}
	publicID, displayName, err := s.userResolver.ResolveUser(ctx, userID)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("resolve task user identity: %w", err)
	}
	return publicID, displayName, nil
}

func (s *Service) enrichAdminTaskListItem(ctx context.Context, item *AdminTaskMonitorListItem) error {
	if item == nil {
		return nil
	}
	publicID, displayName, err := s.resolveAdminTaskUser(ctx, item.internalUserID)
	if err != nil {
		return err
	}
	item.UserPublicID = publicID
	item.UserDisplayName = displayName
	return nil
}

func (s *Service) enrichAdminTaskDetail(ctx context.Context, detail *AdminTaskMonitorDetail) error {
	if detail == nil {
		return nil
	}
	publicID, displayName, err := s.resolveAdminTaskUser(ctx, detail.internalUserID)
	if err != nil {
		return err
	}
	detail.UserPublicID = publicID
	detail.UserDisplayName = displayName
	return nil
}

func normalizeTaskListStatus(status string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(status)); normalized {
	case "", tasks.StatusPending, tasks.StatusRunning, tasks.StatusCompleted, tasks.StatusFailed, tasks.StatusCancelled, tasks.StatusCancelledWithResults:
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

func (s *Service) CancelUserTask(ctx context.Context, taskID string, userID int64) error {
	taskID = strings.TrimSpace(taskID)
	task, err := s.taskRepo.GetByTaskIDAndUserID(ctx, taskID, userID)
	if err != nil {
		return err
	}
	return s.cancelTask(ctx, task, userID, userTaskCancelReason, func() (*tasks.Task, error) {
		return s.taskRepo.GetByTaskIDAndUserID(ctx, task.TaskID, userID)
	})
}

func (s *Service) CancelAdminTask(ctx context.Context, taskID string, adminID int64) error {
	taskID = strings.TrimSpace(taskID)
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	return s.cancelTask(ctx, task, adminID, adminTaskCancelReason, func() (*tasks.Task, error) {
		return s.taskRepo.GetByTaskID(ctx, task.TaskID)
	})
}

func (s *Service) cancelTask(
	ctx context.Context,
	task *tasks.Task,
	cancelledBy int64,
	reason string,
	reload func() (*tasks.Task, error),
) error {
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
		CancelledBy: &cancelledBy,
		Reason:      reason,
	}, nil)
	if err != nil {
		return err
	}
	if rows > 0 {
		s.publishTaskStatusEvent(ctx, task.TaskID, tasks.StatusCancelled)
		return nil
	}

	latest, err := reload()
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
