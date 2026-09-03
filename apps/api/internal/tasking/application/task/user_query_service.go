package task

import (
	"context"

	tasks "ideaboard/api/internal/tasking/domain"
)

// UserQueryService is the application boundary consumed by user-facing task
// HTTP handlers. It excludes admin reads and all command operations.
type UserQueryService interface {
	GetUserTask(context.Context, string, int64) (*tasks.Task, error)
	GetUserTasks(context.Context, int64, int, int) ([]*tasks.Task, int64, error)
}
