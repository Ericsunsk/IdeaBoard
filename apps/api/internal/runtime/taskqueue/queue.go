package taskqueue

import (
	"context"
	"time"

	"api/internal/platform/redis"
)

const (
	AssignStatusAssigned  = "assigned"
	AssignStatusWaitQueue = "wait_queue"
	AssignStatusQueueFull = "queue_full"
)

type TaskQueue interface {
	Enqueue(ctx context.Context, task *redis.GlobalQueueTask) error
	EnqueueFront(ctx context.Context, task *redis.GlobalQueueTask) error
	Dequeue(ctx context.Context) (*redis.GlobalQueueTask, error)
	DequeueWait(ctx context.Context) (*redis.GlobalQueueTask, error)
	QueueLengths(ctx context.Context) (global int64, wait int64, err error)
}

type TaskAssigner interface {
	Assign(ctx context.Context, accountIDs []uint64, task *redis.GlobalQueueTask) (uint64, string, error)
	Release(ctx context.Context, accountID uint64, taskID string) error
	EnsureAccountConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error
	SetAccountMaxConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error
	GetAccountLoadInfo(ctx context.Context, accountIDs []uint64) (map[uint64]*redis.AccountLoad, error)
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
	MonitorInfo(ctx context.Context, accountIDs []uint64) (*redis.MonitorInfo, error)
}
