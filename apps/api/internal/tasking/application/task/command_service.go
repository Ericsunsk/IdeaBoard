package task

import (
	"context"

	tasks "ideaboard/api/internal/tasking/domain"
)

// CommandService is the application boundary for task mutations and billing
// readiness checks consumed by HTTP adapters.
type CommandService interface {
	SubmitTask(context.Context, int64, *tasks.SubmitTaskRequest) (string, error)
	CheckSubmitTaskReadiness(context.Context, int64, *tasks.SubmitTaskRequest) (SubmitTaskReadiness, error)
	CancelUserTask(context.Context, string, int64) error
	CancelAdminTask(context.Context, string, int64) error
}
