package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
)

// LeaseBelongsToRiverJob reports whether an execution token was minted for
// one River job. Tokens keep the river:{jobID}: prefix so logs and receipts
// stay correlatable across retries.
func LeaseBelongsToRiverJob(leaseToken string, riverJobID int64) bool {
	if riverJobID <= 0 {
		return false
	}
	return strings.HasPrefix(leaseToken, fmt.Sprintf("river:%d:", riverJobID))
}

type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *tasks.TaskPayload) error
}

type TaskCancellationStore interface {
	MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error
	IsCancelled(ctx context.Context, taskID string) (bool, error)
	ClearCancelled(ctx context.Context, taskID string) error
}
