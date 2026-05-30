package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"api/internal/runtime/queuetypes"

	"github.com/redis/go-redis/v9"
)

const (
	KeyAccountConcurrent      = "drawing:api:concurrent"
	KeyRunningTasks           = "drawing:task:running"
	KeyTaskLeases             = "drawing:task:leases"
	KeyTaskLeaseReconcileLock = "drawing:task:lease_reconcile_lock"
	KeyTaskCancelPrefix       = "drawing:task:cancelled"

	AssignTaskStatusAssigned        = "assigned"
	AssignTaskStatusNoCapacity      = "no_capacity"
	AssignTaskStatusReconcileLocked = "reconcile_locked"
)

// Lua 脚本：原子分配任务（无等待队列副作用）
const luaTryAssignTask = `
local account_ids = cjson.decode(ARGV[1])
local max_concurrent_key = ARGV[2]
local task_id = ARGV[3]
local lease_key = ARGV[4]
local running_key = ARGV[5]
local reconcile_lock_key = ARGV[6]

if redis.call("EXISTS", reconcile_lock_key) == 1 then
    return "-2"
end

if redis.call("HEXISTS", lease_key, task_id) == 1 then
    return "-1"
end

local best_account = -1
local best_load = 999999

for _, aid in ipairs(account_ids) do
    local concurrent_key = max_concurrent_key .. ":" .. aid
    local current = tonumber(redis.call("HGET", concurrent_key, "count")) or 0
    local max = tonumber(redis.call("HGET", concurrent_key, "max")) or 3

    if current < max then
        local load = current / max
        if load < best_load then
            best_load = load
            best_account = aid
        end
    end
end

if best_account == -1 then
    return "-1"
end

local concurrent_key = max_concurrent_key .. ":" .. best_account
redis.call("HINCRBY", concurrent_key, "count", 1)
redis.call("HSET", lease_key, task_id, best_account)
redis.call("SADD", running_key, task_id)
return tostring(best_account)
`

// Lua 脚本：释放任务
const luaReleaseTask = `
local account_id = ARGV[1]
local task_id = ARGV[2]
local concurrent_key = ARGV[3]
local lease_key = ARGV[4]
local running_key = ARGV[5]

local leased_account = redis.call("HGET", lease_key, task_id)
if leased_account then
    if tostring(leased_account) ~= tostring(account_id) then
        return "wrong_account"
    end

    redis.call("HDEL", lease_key, task_id)
    redis.call("SREM", running_key, task_id)
    local current = redis.call("HINCRBY", concurrent_key, "count", -1)
    if tonumber(current) < 0 then
        redis.call("HSET", concurrent_key, "count", 0)
    end
    return "released"
end

local current = tonumber(redis.call("HGET", concurrent_key, "count")) or 0
if current < 0 then
    redis.call("HSET", concurrent_key, "count", 0)
end

return "no_lease"
`

const luaReleaseTaskLeaseReconcileLock = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`

const (
	defaultTaskLeaseReconcileLockTTL     = time.Minute
	taskLeaseReconcileLockRetryDelay     = 100 * time.Millisecond
	taskLeaseReconcileLockReleaseTimeout = 2 * time.Second
)

type TaskLease struct {
	TaskID    string
	AccountID uint64
}

type RedisManager struct {
	rdb *redis.Client
}

func NewRedisManager(rdb *redis.Client) *RedisManager {
	return &RedisManager{rdb: rdb}
}

func (r *RedisManager) GetClient() *redis.Client {
	return r.rdb
}

// EnsureAccountConcurrent 确保账号并发配置存在；不会重置已有 count。
func (r *RedisManager) EnsureAccountConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	pipe := r.rdb.Pipeline()
	pipe.HSet(ctx, key, "max", maxConcurrent)
	pipe.HSetNX(ctx, key, "count", 0)
	_, err := pipe.Exec(ctx)
	return err
}

// SetAccountMaxConcurrent 设置账号最大并发数
func (r *RedisManager) SetAccountMaxConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	return r.rdb.HSet(ctx, key, "max", maxConcurrent).Err()
}

// GetAccountConcurrent 获取账号当前并发数
func (r *RedisManager) GetAccountConcurrent(ctx context.Context, accountID uint64) (int, error) {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	count, err := r.rdb.HGet(ctx, key, "count").Int()
	if err == redis.Nil {
		return 0, nil
	}
	return count, err
}

// GetAccountMaxConcurrent 获取账号最大并发数
func (r *RedisManager) GetAccountMaxConcurrent(ctx context.Context, accountID uint64) (int, error) {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	max, err := r.rdb.HGet(ctx, key, "max").Int()
	if err == redis.Nil {
		return 3, nil
	}
	return max, err
}

// TryAssignTask 分配任务给账号（Lua脚本原子操作，无等待队列副作用）
func (r *RedisManager) TryAssignTask(ctx context.Context, accountIDs []uint64, taskID string) (uint64, string, error) {
	if len(accountIDs) == 0 {
		return 0, "", fmt.Errorf("没有可用的账号")
	}
	if taskID == "" {
		return 0, "", fmt.Errorf("task id is empty")
	}

	idsJSON, _ := json.Marshal(accountIDs)
	result, err := r.rdb.Eval(ctx, luaTryAssignTask, []string{},
		string(idsJSON),
		KeyAccountConcurrent,
		taskID,
		KeyTaskLeases,
		KeyRunningTasks,
		KeyTaskLeaseReconcileLock,
	).Result()
	if err != nil {
		return 0, "", err
	}

	resultStr, ok := result.(string)
	if !ok {
		return 0, "", fmt.Errorf("Lua脚本返回类型错误")
	}
	if resultStr == "-1" {
		return 0, AssignTaskStatusNoCapacity, nil
	}
	if resultStr == "-2" {
		return 0, AssignTaskStatusReconcileLocked, nil
	}

	accountID, parseErr := strconv.ParseUint(resultStr, 10, 64)
	if parseErr != nil {
		return 0, "", fmt.Errorf("parse assigned account id %q: %w", resultStr, parseErr)
	}
	return accountID, AssignTaskStatusAssigned, nil
}

func (r *RedisManager) WithTaskLeaseReconcileLock(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("task lease reconcile callback is nil")
	}
	token := fmt.Sprintf("%d-%p", time.Now().UnixNano(), r)
	for {
		locked, err := r.rdb.SetNX(ctx, KeyTaskLeaseReconcileLock, token, defaultTaskLeaseReconcileLockTTL).Result()
		if err != nil {
			return err
		}
		if locked {
			defer func() {
				if err := r.releaseTaskLeaseReconcileLock(ctx, token); err != nil {
					log.Printf("[Redis] release task lease reconcile lock failed: %v", err)
				}
			}()
			return fn(ctx)
		}

		timer := time.NewTimer(taskLeaseReconcileLockRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *RedisManager) releaseTaskLeaseReconcileLock(parent context.Context, token string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), taskLeaseReconcileLockReleaseTimeout)
	defer cancel()
	return r.rdb.Eval(ctx, luaReleaseTaskLeaseReconcileLock, []string{KeyTaskLeaseReconcileLock}, token).Err()
}

// ReleaseTask 释放任务
func (r *RedisManager) ReleaseTask(ctx context.Context, accountID uint64, taskID string) error {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	_, err := r.rdb.Eval(ctx, luaReleaseTask, []string{},
		fmt.Sprintf("%d", accountID),
		taskID,
		key,
		KeyTaskLeases,
		KeyRunningTasks,
	).Result()
	return err
}

func (r *RedisManager) GetTaskLease(ctx context.Context, taskID string) (uint64, bool, error) {
	if taskID == "" {
		return 0, false, fmt.Errorf("task id is empty")
	}
	accountID, err := r.rdb.HGet(ctx, KeyTaskLeases, taskID).Uint64()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return accountID, true, nil
}

func (r *RedisManager) HasActiveLease(ctx context.Context, taskID string, accountID uint64) (bool, error) {
	leasedAccountID, ok, err := r.GetTaskLease(ctx, taskID)
	if err != nil || !ok {
		return false, err
	}
	return leasedAccountID == accountID, nil
}

func (r *RedisManager) ListTaskLeases(ctx context.Context) ([]TaskLease, error) {
	values, err := r.rdb.HGetAll(ctx, KeyTaskLeases).Result()
	if err != nil {
		return nil, err
	}

	leases := make([]TaskLease, 0, len(values))
	for taskID, accountIDStr := range values {
		accountID, scanErr := strconv.ParseUint(accountIDStr, 10, 64)
		if scanErr != nil {
			return nil, fmt.Errorf("parse task lease %s=%q: %w", taskID, accountIDStr, scanErr)
		}
		leases = append(leases, TaskLease{TaskID: taskID, AccountID: accountID})
	}
	return leases, nil
}

func (r *RedisManager) RebuildRunningTaskLeases(ctx context.Context, leases []TaskLease, accountIDs []uint64) error {
	counts := make(map[uint64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		counts[accountID] = 0
	}
	for _, lease := range leases {
		if lease.TaskID == "" || lease.AccountID == 0 {
			return fmt.Errorf("invalid task lease: task_id=%q account_id=%d", lease.TaskID, lease.AccountID)
		}
		counts[lease.AccountID]++
	}

	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, KeyTaskLeases, KeyRunningTasks)
	for accountID, count := range counts {
		key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
		pipe.HSet(ctx, key, "count", count)
	}
	for _, lease := range leases {
		pipe.HSet(ctx, KeyTaskLeases, lease.TaskID, lease.AccountID)
		pipe.SAdd(ctx, KeyRunningTasks, lease.TaskID)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// GetAccountLoadInfo 获取所有账号的负载信息
func (r *RedisManager) GetAccountLoadInfo(ctx context.Context, accountIDs []uint64) (map[uint64]*queuetypes.AccountLoad, error) {
	result := make(map[uint64]*queuetypes.AccountLoad)

	for _, id := range accountIDs {
		key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, id)
		count, _ := r.rdb.HGet(ctx, key, "count").Int()
		max, _ := r.rdb.HGet(ctx, key, "max").Int()
		if max == 0 {
			max = 3
		}
		result[id] = &queuetypes.AccountLoad{
			CurrentConcurrent: count,
			MaxConcurrent:     max,
		}
	}

	return result, nil
}

// DeleteAccountConcurrent 删除账号并发计数
func (r *RedisManager) DeleteAccountConcurrent(ctx context.Context, accountID uint64) error {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	return r.rdb.Del(ctx, key).Err()
}

func (r *RedisManager) MarkTaskCancelled(ctx context.Context, taskID string, ttl time.Duration) error {
	return r.rdb.Set(ctx, taskCancelKey(taskID), "1", ttl).Err()
}

func (r *RedisManager) IsTaskCancelled(ctx context.Context, taskID string) (bool, error) {
	exists, err := r.rdb.Exists(ctx, taskCancelKey(taskID)).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

func (r *RedisManager) ClearTaskCancelled(ctx context.Context, taskID string) error {
	return r.rdb.Del(ctx, taskCancelKey(taskID)).Err()
}

func taskCancelKey(taskID string) string {
	return fmt.Sprintf("%s:%s", KeyTaskCancelPrefix, taskID)
}
