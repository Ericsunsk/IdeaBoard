package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	KeyAccountConcurrent = "drawing:api:concurrent"
	KeyGlobalQueue      = "drawing:task:global_queue"
	KeyWaitQueue        = "drawing:task:wait_queue"
	KeyRunningTasks     = "drawing:task:running"
	MaxGlobalQueueLen   = 10000
	MaxWaitQueueLen     = 100
)

// Lua 脚本：原子分配任务
const luaAssignTask = `
local account_ids = cjson.decode(ARGV[1])
local max_concurrent_key = ARGV[2]

-- 找最小负载的账号
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
    -- 所有账号都繁忙，检查等待队列
    local wait_len = redis.call("LLEN", "drawing:task:wait_queue")
    if wait_len >= 100 then
        return "-2"  -- 等待队列满
    end
    -- 加入等待队列
    redis.call("RPUSH", "drawing:task:wait_queue", ARGV[3])
    return "-1"  -- 进入等待队列
end

-- 分配给最佳账号，原子增加并发计数
local concurrent_key = max_concurrent_key .. ":" .. best_account
redis.call("HINCRBY", concurrent_key, "count", 1)
redis.call("SADD", "drawing:task:running", ARGV[4])

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

// InitAccountConcurrent 初始化账号并发计数
func (r *RedisManager) InitAccountConcurrent(ctx context.Context, accountID uint64, maxConcurrent int) error {
	key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, accountID)
	pipe := r.rdb.Pipeline()
	pipe.HSet(ctx, key, "count", 0)
	pipe.HSet(ctx, key, "max", maxConcurrent)
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

// PushGlobalQueue 推送任务到全局队列
func (r *RedisManager) PushGlobalQueue(ctx context.Context, task *GlobalQueueTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	length, err := r.rdb.LLen(ctx, KeyGlobalQueue).Result()
	if err != nil {
		return err
	}
	if length >= MaxGlobalQueueLen {
		return fmt.Errorf("队列已满，请稍后再试")
	}

	return r.rdb.RPush(ctx, KeyGlobalQueue, string(data)).Err()
}

func (r *RedisManager) PushGlobalQueueFront(ctx context.Context, task *GlobalQueueTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return r.rdb.LPush(ctx, KeyGlobalQueue, string(data)).Err()
}

// PopGlobalQueue 弹出全局队列任务
func (r *RedisManager) PopGlobalQueue(ctx context.Context) (*GlobalQueueTask, error) {
	data, err := r.rdb.LPop(ctx, KeyGlobalQueue).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var task GlobalQueueTask
	if err := json.Unmarshal([]byte(data), &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *RedisManager) PopWaitQueue(ctx context.Context) (*GlobalQueueTask, error) {
	data, err := r.rdb.LPop(ctx, KeyWaitQueue).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var task GlobalQueueTask
	if err := json.Unmarshal([]byte(data), &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// GetGlobalQueueLength 获取全局队列长度
func (r *RedisManager) GetGlobalQueueLength(ctx context.Context) (int64, error) {
	return r.rdb.LLen(ctx, KeyGlobalQueue).Result()
}

// GetWaitQueueLength 获取等待队列长度
func (r *RedisManager) GetWaitQueueLength(ctx context.Context) (int64, error) {
	return r.rdb.LLen(ctx, KeyWaitQueue).Result()
}

// AssignTask 分配任务给账号（Lua脚本原子操作）
func (r *RedisManager) AssignTask(ctx context.Context, accountIDs []uint64, task *GlobalQueueTask) (uint64, string, error) {
	if len(accountIDs) == 0 {
		return 0, "", fmt.Errorf("没有可用的账号")
	}

	idsJSON, _ := json.Marshal(accountIDs)
	taskJSON, _ := json.Marshal(task)

	result, err := r.rdb.Eval(ctx, luaAssignTask, []string{},
		string(idsJSON),
		KeyAccountConcurrent,
		string(taskJSON),
		task.TaskID,
	).Result()

	if err != nil {
		return 0, "", err
	}

	resultStr, ok := result.(string)
	if !ok {
		return 0, "", fmt.Errorf("Lua脚本返回类型错误")
	}

	switch resultStr {
	case "-1":
		return 0, "wait_queue", nil
	case "-2":
		return 0, "queue_full", nil
	default:
		var accountID uint64
		fmt.Sscanf(resultStr, "%d", &accountID)
		return accountID, "assigned", nil
	}
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
func (r *RedisManager) GetAccountLoadInfo(ctx context.Context, accountIDs []uint64) (map[uint64]*AccountLoad, error) {
	result := make(map[uint64]*AccountLoad)

	for _, id := range accountIDs {
		key := fmt.Sprintf("%s:%d", KeyAccountConcurrent, id)
		count, _ := r.rdb.HGet(ctx, key, "count").Int()
		max, _ := r.rdb.HGet(ctx, key, "max").Int()
		if max == 0 {
			max = 3
		}
		result[id] = &AccountLoad{
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

// GetAllRunningTasks 获取所有运行中的任务ID
func (r *RedisManager) GetAllRunningTasks(ctx context.Context) ([]string, error) {
	return r.rdb.SMembers(ctx, KeyRunningTasks).Result()
}

// 全局队列任务结构
type GlobalQueueTask struct {
	TaskID     string   `json:"task_id"`
	UserID     uint64   `json:"user_id"`
	Prompt     string   `json:"prompt"`
	Params     string   `json:"params"`
	AccountIDs []uint64 `json:"account_ids"`
	RetryCount int      `json:"retry_count"`
	CreatedAt  int64    `json:"created_at"`
}

// 账号负载信息
type AccountLoad struct {
	CurrentConcurrent int `json:"current_concurrent"`
	MaxConcurrent     int `json:"max_concurrent"`
}

// 监控信息
type MonitorInfo struct {
	GlobalQueueLen int64                  `json:"global_queue_len"`
	WaitQueueLen   int64                  `json:"wait_queue_len"`
	RunningTasks   int64                  `json:"running_tasks"`
	AccountLoads   map[uint64]*AccountLoad `json:"account_loads"`
}

// GetMonitorInfo 获取监控信息
func (r *RedisManager) GetMonitorInfo(ctx context.Context, accountIDs []uint64) (*MonitorInfo, error) {
	info := &MonitorInfo{
		AccountLoads: make(map[uint64]*AccountLoad),
	}

	info.GlobalQueueLen, _ = r.GetGlobalQueueLength(ctx)
	info.WaitQueueLen, _ = r.GetWaitQueueLength(ctx)
	info.RunningTasks, _ = r.rdb.SCard(ctx, KeyRunningTasks).Result()

	loads, _ := r.GetAccountLoadInfo(ctx, accountIDs)
	info.AccountLoads = loads

	return info, nil
}
