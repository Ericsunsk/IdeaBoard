package task

import "context"

import tasks "ideaboard/api/internal/tasking/domain"

// AdminQueryService is the application boundary consumed by admin HTTP
// handlers. It deliberately excludes user commands and submission flows.
type AdminQueryService interface {
	GetTask(context.Context, string) (*AdminTaskMonitorDetail, error)
	ListTasks(context.Context, *tasks.TaskListRequest) ([]AdminTaskMonitorListItem, tasks.TaskListCounts, error)
	ResolveAdminTaskUserID(context.Context, string) (int64, error)
}
