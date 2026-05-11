package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"api/internal/runtime/queuetypes"

	"github.com/redis/go-redis/v9"
)

const (
	KeyAccountConcurrent = "drawing:api:concurrent"
	KeyRunningTasks      = "drawing:task:running"
	KeyTaskCancelPrefix  = "drawing:task:cancelled"

	AssignTaskStatusAssigned   = "assigned"
	AssignTaskStatusNoCapacity = "no_capacity"
)

// Lua 脚本：原子分配任务（无等待队列副作用）
const luaTryAssignTask = `
local account_ids = cjson.decode(ARGV[1])
local max_concurrent_key = ARGV[2]

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
redis.call("SADD", "drawing:task:running", ARGV[3])
return tostring(best_account)
`

// Lua 脚本：释放任务
const luaReleaseTask = `
local account_id = ARGV[1]
local task_id = ARGV[2]
local concurrent_key = ARGV[3]

local current = redis.call("HINCRBY", concurrent_key, "count", -1)
if tonumber(current) < 0 then
    redis.call("HSET", concurrent_key, "count", 0)
end

redis.call("SREM", "drawing:task:running", task_id)

return "ok"
`

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

	var accountID uint64
	fmt.Sscanf(resultStr, "%d", &accountID)
	return accountID, AssignTaskStatusAssigned, nil
}

// ReleaseTask 释放任务
func (r *RedisManager) ReleaseTask(ctx context.Context, accountID uint64, taskID string) error {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	_, err := r.rdb.Eval(ctx, luaReleaseTask, []string{},
		fmt.Sprintf("%d", accountID),
		taskID,
		key,
	).Result()
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
