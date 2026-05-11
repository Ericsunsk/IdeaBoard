package taskqueue

import (
	"context"
	"time"

	"api/internal/platform/redis"
)

type RedisQueue struct {
	mgr *redis.RedisManager
}

func NewRedisQueue(mgr *redis.RedisManager) *RedisQueue {
	return &RedisQueue{mgr: mgr}
}

func (q *RedisQueue) Enqueue(ctx context.Context, task *redis.GlobalQueueTask) error {
	return q.mgr.PushGlobalQueue(ctx, task)
}

func (q *RedisQueue) EnqueueFront(ctx context.Context, task *redis.GlobalQueueTask) error {
	return q.mgr.PushGlobalQueueFront(ctx, task)
}

func (q *RedisQueue) Dequeue(ctx context.Context) (*redis.GlobalQueueTask, error) {
	return q.mgr.PopGlobalQueue(ctx)
}

func (q *RedisQueue) DequeueWait(ctx context.Context) (*redis.GlobalQueueTask, error) {
	return q.mgr.PopWaitQueue(ctx)
}

func (q *RedisQueue) QueueLengths(ctx context.Context) (int64, int64, error) {
	global, err := q.mgr.GetGlobalQueueLength(ctx)
	if err != nil {
		return 0, 0, err
	}
	wait, err := q.mgr.GetWaitQueueLength(ctx)
	if err != nil {
		return 0, 0, err
	}
	return global, wait, nil
}

func (q *RedisQueue) Assign(ctx context.Context, accountIDs []uint64, task *redis.GlobalQueueTask) (uint64, string, error) {
	accountID, status, err := q.mgr.AssignTask(ctx, accountIDs, task)
	if err != nil {
		return 0, "", err
	}
	switch status {
	case "wait_queue":
		return 0, AssignStatusWaitQueue, nil
	case "queue_full":
		return 0, AssignStatusQueueFull, nil
	default:
		return accountID, AssignStatusAssigned, nil
	}
}

func (q *RedisQueue) Release(ctx context.Context, accountID uint64, taskID string) error {
	return q.mgr.ReleaseTask(ctx, accountID, taskID)
}

func (q *RedisQueue) EnsureAccountConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error {
	return q.mgr.EnsureAccountConcurrent(ctx, accountID, maxConcurrent)
}

func (q *RedisQueue) SetAccountMaxConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error {
	return q.mgr.SetAccountMaxConcurrent(ctx, accountID, maxConcurrent)
}

func (q *RedisQueue) GetAccountLoadInfo(ctx context.Context, accountIDs []uint64) (map[uint64]*redis.AccountLoad, error) {
	return q.mgr.GetAccountLoadInfo(ctx, accountIDs)
}

func (q *RedisQueue) DeleteAccountConcurrent(ctx context.Context, accountID uint64) error {
	return q.mgr.DeleteAccountConcurrent(ctx, accountID)
}

func (q *RedisQueue) GetAccountConcurrent(ctx context.Context, accountID uint64) (int, error) {
	return q.mgr.GetAccountConcurrent(ctx, accountID)
}

func (q *RedisQueue) GetAccountMaxConcurrent(ctx context.Context, accountID uint64) (int, error) {
	return q.mgr.GetAccountMaxConcurrent(ctx, accountID)
}

func (q *RedisQueue) MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error {
	return q.mgr.MarkTaskCancelled(ctx, taskID, ttl)
}

func (q *RedisQueue) IsCancelled(ctx context.Context, taskID string) (bool, error) {
	return q.mgr.IsTaskCancelled(ctx, taskID)
}

func (q *RedisQueue) ClearCancelled(ctx context.Context, taskID string) error {
	return q.mgr.ClearTaskCancelled(ctx, taskID)
}

func (q *RedisQueue) MonitorInfo(ctx context.Context, accountIDs []uint64) (*redis.MonitorInfo, error) {
	return q.mgr.GetMonitorInfo(ctx, accountIDs)
}
