package taskqueue

import (
	"context"
	"errors"
	"time"

	"ideaboard/api/internal/runtime/queuetypes"
)

const (
	AssignStatusAssigned   = "assigned"
	AssignStatusNoCapacity = "no_capacity"
	AssignStatusDiscard    = "discard"
)

var ErrNoAccountCapacity = errors.New("no account capacity")

type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *queuetypes.TaskPayload) error
}

type QueueStatsStore interface {
	QueueLengths(ctx context.Context) (global int64, wait int64, err error)
}

type TaskAssigner interface {
	Assign(ctx context.Context, accountIDs []int64, task *queuetypes.TaskPayload) (TaskAssignment, error)
	ConfirmTaskLease(ctx context.Context, assignment TaskAssignment) error
}

type TaskAssignment struct {
	TaskID     string
	AccountID  int64
	Status     string
	LeaseToken string
}

type AccountConcurrencyStore interface {
	GetAccountConcurrent(ctx context.Context, accountID int64) (int, error)
	GetAccountMaxConcurrent(ctx context.Context, accountID int64) (int, error)
}

type AccountLoadStore interface {
	GetAccountLoadInfo(ctx context.Context, accountIDs []int64) (map[int64]*queuetypes.AccountLoad, error)
}

type TaskCancellationStore interface {
	MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error
	IsCancelled(ctx context.Context, taskID string) (bool, error)
	ClearCancelled(ctx context.Context, taskID string) error
}

type TaskMonitorStore interface {
	MonitorInfo(ctx context.Context, accountIDs []int64) (*queuetypes.MonitorInfo, error)
}
