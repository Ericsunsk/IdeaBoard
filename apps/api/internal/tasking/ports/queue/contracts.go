package queue

import (
	"context"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *tasks.TaskPayload) error
}

type TaskCancellationStore interface {
	MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error
	IsCancelled(ctx context.Context, taskID string) (bool, error)
}
