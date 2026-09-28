package task

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/platform/validation"
	tasks "ideaboard/api/internal/tasking/domain"
	billingfacts "ideaboard/api/internal/tasking/ports/billing"

	"github.com/google/uuid"
)

const maxTaskKeywordLength = 200

// AdminQueryService is the application boundary consumed by admin HTTP
// handlers. It deliberately excludes user commands and submission flows.
type AdminQueryService interface {
	GetTask(context.Context, string) (*AdminMonitorDetail, error)
	ListTasks(context.Context, *tasks.TaskListRequest) ([]AdminMonitorListItem, tasks.TaskListCounts, error)
	ResolveAdminTaskUserID(context.Context, string) (int64, error)
}

type AdminMonitorListItem struct {
	TaskID          string                         `json:"task_id"`
	UserPublicID    uuid.UUID                      `json:"user_public_id"`
	UserDisplayName string                         `json:"user_display_name"`
	Prompt          string                         `json:"prompt"`
	Service         string                         `json:"service"`
	Credits         int64                          `json:"credits"`
	Priority        int                            `json:"priority"`
	Status          string                         `json:"status"`
	ErrorMsg        string                         `json:"error_msg"`
	Result          string                         `json:"result"`
	CreatedAt       time.Time                      `json:"created_at"`
	CompletedAt     *time.Time                     `json:"completed_at"`
	LatestExecution *AdminMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
	internalUserID  int64
}

type AdminMonitorDetail struct {
	TaskID            string                         `json:"task_id"`
	CorrelationID     string                         `json:"correlation_id"`
	UserPublicID      uuid.UUID                      `json:"user_public_id"`
	UserDisplayName   string                         `json:"user_display_name"`
	Prompt            string                         `json:"prompt"`
	UserPrompt        string                         `json:"user_prompt"`
	Params            string                         `json:"params"`
	Service           string                         `json:"service"`
	Priority          int                            `json:"priority"`
	Status            string                         `json:"status"`
	RetryCount        int                            `json:"retry_count"`
	ErrorMsg          string                         `json:"error_msg"`
	Result            string                         `json:"result"`
	Outputs           []tasks.TaskOutput             `json:"outputs,omitempty"`
	ExecutionMetrics  *tasks.TaskExecutionMetrics    `json:"execution_metrics,omitempty"`
	CreatedAt         time.Time                      `json:"created_at"`
	UpdatedAt         time.Time                      `json:"updated_at"`
	CompletedAt       *time.Time                     `json:"completed_at"`
	CancelledAt       *time.Time                     `json:"cancelled_at"`
	CancelReason      string                         `json:"cancel_reason"`
	Billing           *tasks.AdminBillingSnapshot    `json:"billing,omitempty"`
	LatestExecution   *AdminMonitorExecutionSnapshot `json:"latest_execution,omitempty"`
	ExecutionAttempts []tasks.AdminTaskAttempt       `json:"execution_attempts"`
	internalUserID    int64
}

type AdminMonitorExecutionSnapshot = tasks.AdminTaskExecutionSnapshot

func (s *Service) GetTask(ctx context.Context, taskID string) (*AdminMonitorDetail, error) {
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
	detail := adminMonitorDetailFromTask(task, billingSnapshot, hasBilling, traces[task.TaskID])
	if err := s.enrichAdminTaskDetail(ctx, detail); err != nil {
		return nil, err
	}
	return detail, nil
}

func (s *Service) ListTasks(ctx context.Context, req *tasks.TaskListRequest) ([]AdminMonitorListItem, tasks.TaskListCounts, error) {
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
	billings, err := s.loadAdminBillingSnapshots(ctx, taskIDsFromTasks(items))
	if err != nil {
		return nil, tasks.TaskListCounts{}, err
	}
	result := make([]AdminMonitorListItem, 0, len(items))
	for _, item := range items {
		latestExecution, hasLatestExecution := latest[item.TaskID]
		billing, hasBilling := billings[item.TaskID]
		row := adminMonitorListItemFromTask(item, latestExecution, hasLatestExecution, billing, hasBilling)
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

func adminMonitorExecutionSnapshotFromAttempt(attempt tasks.AdminTaskAttempt) *AdminMonitorExecutionSnapshot {
	return &AdminMonitorExecutionSnapshot{
		ResolvedModel: attempt.ResolvedModel,
		Provider:      attempt.Provider,
		GatewayStep:   attempt.GatewayStep,
	}
}

func adminMonitorListItemFromTask(task *tasks.Task, latestExecution tasks.AdminTaskExecutionSnapshot, hasLatestExecution bool, billing tasks.AdminBillingSnapshot, hasBilling bool) AdminMonitorListItem {
	if task == nil {
		return AdminMonitorListItem{}
	}
	item := AdminMonitorListItem{
		TaskID:         task.TaskID,
		internalUserID: task.UserID,
		Prompt:         task.Prompt,
		Service:        task.Service,
		Priority:       task.Priority,
		Status:         task.Status,
		ErrorMsg:       task.ErrorMsg,
		Result:         task.Result,
		CreatedAt:      task.CreatedAt,
		CompletedAt:    task.CompletedAt,
	}
	if hasBilling && billing.SettlementState == string(billingfacts.SettlementStateCaptured) {
		item.Credits = billing.CapturedCredits
	}
	if hasLatestExecution {
		item.LatestExecution = &latestExecution
	}
	return item
}

func adminMonitorDetailFromTask(task *tasks.Task, billing tasks.AdminBillingSnapshot, hasBilling bool, attempts []tasks.AdminTaskAttempt) *AdminMonitorDetail {
	if task == nil {
		return nil
	}
	if attempts == nil {
		attempts = []tasks.AdminTaskAttempt{}
	}
	detail := &AdminMonitorDetail{
		TaskID:            task.TaskID,
		internalUserID:    task.UserID,
		CorrelationID:     task.EffectiveCorrelationID(),
		Prompt:            task.Prompt,
		UserPrompt:        task.UserPrompt,
		Params:            task.Params,
		Service:           task.Service,
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
		detail.LatestExecution = adminMonitorExecutionSnapshotFromAttempt(attempts[len(attempts)-1])
	}
	return detail
}

func normalizeTaskListStatus(status string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(status)); normalized {
	case "", tasks.StatusPending, tasks.StatusRunning, tasks.StatusCompleted, tasks.StatusFailed, tasks.StatusCancelled, tasks.StatusCancelledWithResults:
		return normalized, nil
	default:
		return "", validation.New(msgNormalizeTaskListStatus)
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
