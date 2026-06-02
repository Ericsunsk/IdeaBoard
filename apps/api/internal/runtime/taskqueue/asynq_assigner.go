package taskqueue

import (
	"context"
	"fmt"
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

func (a *RedisAsynqAssigner) Assign(ctx context.Context, accountIDs []int64, task *queuetypes.TaskPayload) (TaskAssignment, error) {
	accountID, leaseToken, status, err := a.mgr.TryAssignTask(ctx, accountIDs, task.TaskID)
	if err != nil {
		return TaskAssignment{}, err
	}
	switch status {
	case redis.AssignTaskStatusNoCapacity, redis.AssignTaskStatusReconcileLocked, redis.AssignTaskStatusActiveLease:
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusNoCapacity}, nil
	case redis.AssignTaskStatusAssigned:
		return TaskAssignment{
			TaskID:     task.TaskID,
			AccountID:  accountID,
			Status:     AssignStatusAssigned,
			LeaseToken: leaseToken,
		}, nil
	default:
		return TaskAssignment{}, fmt.Errorf("unexpected assign status %q for task=%s", status, task.TaskID)
	}
}

func (a *RedisAsynqAssigner) ConfirmTaskRunning(ctx context.Context, assignment TaskAssignment) error {
	return a.mgr.ConfirmTaskRunning(ctx, assignment.AccountID, assignment.TaskID, assignment.LeaseToken)
}

func (a *RedisAsynqAssigner) Release(ctx context.Context, taskID string, accountID int64, leaseToken string) error {
	if accountID == 0 || taskID == "" || leaseToken == "" {
		return nil
	}
	return a.mgr.ReleaseTask(ctx, accountID, taskID, leaseToken)
}

func (a *RedisAsynqAssigner) GetTaskLease(ctx context.Context, taskID string) (int64, bool, error) {
	return a.mgr.GetTaskLease(ctx, taskID)
}

func (a *RedisAsynqAssigner) HasActiveLease(ctx context.Context, taskID string, accountID int64) (bool, error) {
	return a.mgr.HasActiveLease(ctx, taskID, accountID)
}

func (a *RedisAsynqAssigner) ListTaskLeases(ctx context.Context) ([]TaskLease, error) {
	leases, err := a.mgr.ListTaskLeases(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]TaskLease, 0, len(leases))
	for _, lease := range leases {
		result = append(result, TaskLease{TaskID: lease.TaskID, AccountID: lease.AccountID, LeaseToken: lease.LeaseToken})
	}
	return result, nil
}

func (a *RedisAsynqAssigner) RebuildRunningTaskLeases(ctx context.Context, leases []TaskLease, accountIDs []int64) error {
	redisLeases := make([]redis.TaskLease, 0, len(leases))
	for _, lease := range leases {
		redisLeases = append(redisLeases, redis.TaskLease{
			TaskID:     lease.TaskID,
			AccountID:  lease.AccountID,
			LeaseToken: lease.LeaseToken,
			State:      redis.TaskLeaseStateRunning,
		})
	}
	return a.mgr.RebuildRunningTaskLeases(ctx, redisLeases, accountIDs)
}

func (a *RedisAsynqAssigner) WithTaskLeaseReconcileLock(ctx context.Context, fn func(context.Context) error) error {
	return a.mgr.WithTaskLeaseReconcileLock(ctx, fn)
}

func (a *RedisAsynqAssigner) EnsureAccountConcurrent(ctx context.Context, accountID int64, maxConcurrent int) error {
	return a.mgr.EnsureAccountConcurrent(ctx, accountID, maxConcurrent)
}

func (a *RedisAsynqAssigner) SetAccountMaxConcurrent(ctx context.Context, accountID int64, maxConcurrent int) error {
	return a.mgr.SetAccountMaxConcurrent(ctx, accountID, maxConcurrent)
}

func (a *RedisAsynqAssigner) GetAccountLoadInfo(ctx context.Context, accountIDs []int64) (map[int64]*queuetypes.AccountLoad, error) {
	return a.mgr.GetAccountLoadInfo(ctx, accountIDs)
}

func (a *RedisAsynqAssigner) DeleteAccountConcurrent(ctx context.Context, accountID int64) error {
	return a.mgr.DeleteAccountConcurrent(ctx, accountID)
}

func (a *RedisAsynqAssigner) GetAccountConcurrent(ctx context.Context, accountID int64) (int, error) {
	return a.mgr.GetAccountConcurrent(ctx, accountID)
}

func (a *RedisAsynqAssigner) GetAccountMaxConcurrent(ctx context.Context, accountID int64) (int, error) {
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
