package task

import (
	"context"

	tasks "ideaboard/api/internal/tasking/domain"
)

type Service struct {
	taskRepo            TaskStore
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
	ListOutputsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]tasks.TaskOutput, error)
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

type BillableItemLookup interface {
	Contains(string) bool
}

type ServiceDeps struct {
	TaskRepo            TaskStore
	AdminBillingHistory AdminBillingHistoryStore
	AdminTaskTrace      AdminTaskTraceStore
	AdminTaskListRead   AdminTaskListReadStore
	BillableItems       BillableItemLookup
	UserResolver        AdminTaskUserResolver
}

func NewService(deps ServiceDeps) *Service {
	return &Service{
		taskRepo:            deps.TaskRepo,
		adminBillingHistory: deps.AdminBillingHistory,
		adminTaskTrace:      deps.AdminTaskTrace,
		adminTaskListRead:   deps.AdminTaskListRead,
		billableItems:       deps.BillableItems,
		userResolver:        deps.UserResolver,
	}
}
