package taskqueue

import (
	"context"
	"time"

	"api/internal/platform/redis"
	"api/internal/runtime/queuetypes"
)

type RedisAsynqAssigner struct {
	mgr *redis.RedisManager
}

func NewRedisAsynqAssigner(mgr *redis.RedisManager) *RedisAsynqAssigner {
	return &RedisAsynqAssigner{mgr: mgr}
}

func (a *RedisAsynqAssigner) Assign(ctx context.Context, accountIDs []uint64, task *queuetypes.TaskPayload) (uint64, string, error) {
	accountID, status, err := a.mgr.TryAssignTask(ctx, accountIDs, task.TaskID)
	if err != nil {
		return 0, "", err
	}
	switch status {
	case redis.AssignTaskStatusNoCapacity:
		return 0, AssignStatusNoCapacity, nil
	default:
		return accountID, AssignStatusAssigned, nil
	}
}

func (a *RedisAsynqAssigner) Release(ctx context.Context, accountID uint64, taskID string) error {
	return a.mgr.ReleaseTask(ctx, accountID, taskID)
}

func (a *RedisAsynqAssigner) EnsureAccountConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error {
	return a.mgr.EnsureAccountConcurrent(ctx, accountID, maxConcurrent)
}

func (a *RedisAsynqAssigner) SetAccountMaxConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error {
	return a.mgr.SetAccountMaxConcurrent(ctx, accountID, maxConcurrent)
}

func (a *RedisAsynqAssigner) GetAccountLoadInfo(ctx context.Context, accountIDs []uint64) (map[uint64]*queuetypes.AccountLoad, error) {
	return a.mgr.GetAccountLoadInfo(ctx, accountIDs)
}

func (a *RedisAsynqAssigner) DeleteAccountConcurrent(ctx context.Context, accountID uint64) error {
	return a.mgr.DeleteAccountConcurrent(ctx, accountID)
}

func (a *RedisAsynqAssigner) GetAccountConcurrent(ctx context.Context, accountID uint64) (int, error) {
	return a.mgr.GetAccountConcurrent(ctx, accountID)
}

func (a *RedisAsynqAssigner) GetAccountMaxConcurrent(ctx context.Context, accountID uint64) (int, error) {
	return a.mgr.GetAccountMaxConcurrent(ctx, accountID)
}

func (a *RedisAsynqAssigner) MarkCancelled(ctx context.Context, taskID string, ttl time.Duration) error {
	return a.mgr.MarkTaskCancelled(ctx, taskID, ttl)
}

func (a *RedisAsynqAssigner) IsCancelled(ctx context.Context, taskID string) (bool, error) {
	return a.mgr.IsTaskCancelled(ctx, taskID)
}

func (a *RedisAsynqAssigner) ClearCancelled(ctx context.Context, taskID string) error {
	return a.mgr.ClearTaskCancelled(ctx, taskID)
}
