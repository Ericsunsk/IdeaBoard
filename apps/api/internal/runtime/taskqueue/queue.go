package taskqueue

import (
	"context"
	"errors"
	"time"

	"api/internal/runtime/queuetypes"
)

const (
	AssignStatusAssigned   = "assigned"
	AssignStatusNoCapacity = "no_capacity"
)

var ErrNoAccountCapacity = errors.New("no account capacity")

type TaskEnqueuer interface {
	Enqueue(ctx context.Context, task *queuetypes.TaskPayload) error
}

type QueueStatsStore interface {
	QueueLengths(ctx context.Context) (global int64, wait int64, err error)
}

type TaskAssigner interface {
	Assign(ctx context.Context, accountIDs []uint64, task *queuetypes.TaskPayload) (uint64, string, error)
	Release(ctx context.Context, accountID uint64, taskID string) error
	EnsureAccountConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error
	SetAccountMaxConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error
	GetAccountLoadInfo(ctx context.Context, accountIDs []uint64) (map[uint64]*queuetypes.AccountLoad, error)
}

type TaskLease struct {
	TaskID    string
	AccountID uint64
}

type TaskLeaseRebuilder interface {
	RebuildRunningTaskLeases(ctx context.Context, leases []TaskLease, accountIDs []uint64) error
}

type TaskLeaseReconcileLocker interface {
	WithTaskLeaseReconcileLock(ctx context.Context, fn func(context.Context) error) error
}

type AccountConcurrencyStore interface {
	DeleteAccountConcurrent(ctx context.Context, accountID uint64) error
	GetAccountConcurrent(ctx context.Context, accountID uint64) (int, error)
	GetAccountMaxConcurrent(ctx context.Context, accountID uint64) (int, error)
}

type TaskCancellationStore interface {
	MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error
	IsCancelled(ctx context.Context, taskID string) (bool, error)
	ClearCancelled(ctx context.Context, taskID string) error
}

type TaskMonitorStore interface {
	MonitorInfo(ctx context.Context, accountIDs []uint64) (*queuetypes.MonitorInfo, error)
}
