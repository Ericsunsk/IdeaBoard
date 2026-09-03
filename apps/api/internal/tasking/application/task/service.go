package task

import (
	"context"

	"ideaboard/api/internal/events"
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	tasksubmission "ideaboard/api/internal/tasking/application/submission"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
)

type Service struct {
	taskIntake          TaskIntake
	taskRepo            TaskStore
	submissionStore     TaskSubmissionStore
	cancelStore         taskqueueports.TaskCancellationStore
	taskEvents          events.TaskStatusPublisher
	taskBilling         BillingReadiness
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

type BillableItemLookup interface {
	Contains(string) bool
}

type BillingReadiness interface {
	Readiness(context.Context, int64, taskbilling.Request) (taskbilling.Readiness, error)
}

type ServiceDeps struct {
	TaskIntake          TaskIntake
	TaskRepo            TaskStore
	SubmissionStore     TaskSubmissionStore
	CancelStore         taskqueueports.TaskCancellationStore
	TaskBilling         BillingReadiness
	AdminBillingHistory AdminBillingHistoryStore
	AdminTaskTrace      AdminTaskTraceStore
	AdminTaskListRead   AdminTaskListReadStore
	BillableItems       BillableItemLookup
	UserResolver        AdminTaskUserResolver
}

type ServiceOptions struct {
	TaskEvents events.TaskStatusPublisher
}

func NewService(deps ServiceDeps, opts ServiceOptions) *Service {
	return &Service{
		taskIntake:          deps.TaskIntake,
		taskRepo:            deps.TaskRepo,
		submissionStore:     deps.SubmissionStore,
		cancelStore:         deps.CancelStore,
		taskEvents:          opts.TaskEvents,
		taskBilling:         deps.TaskBilling,
		adminBillingHistory: deps.AdminBillingHistory,
		adminTaskTrace:      deps.AdminTaskTrace,
		adminTaskListRead:   deps.AdminTaskListRead,
		billableItems:       deps.BillableItems,
		userResolver:        deps.UserResolver,
	}
}
