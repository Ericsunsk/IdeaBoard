# River Migration Notes

更新日期：2026-06-02

## 基线决策

1. 生产任务队列与一致性核心统一为 River + PostgreSQL。
2. 不保留 Redis list queue、Asynq、Dragonfly 或 Garnet 生产兼容路径，不提供 backend 切换开关。
3. NATS JetStream 仅作为后续事件/audit 层候选，不承担任务队列职责。
4. 任务状态、取消标记、账号占用和队列监控均以 PostgreSQL/River 为准。

## Retry 对齐

- DB 业务重试上限仍由 `TaskRuntimeService` 控制（当前默认 3 次）。
- River `MaxAttempts` 仅覆盖基础设施级重投（worker crash/ack 丢失等）。
- 账号容量不足返回 typed `ErrNoAccountCapacity`，由 River snooze/retry 延迟重试，不标记 DB failed。

## Timeout 对齐

- River task timeout 采用账号超时上限之上的保护值（默认 180s，可配置）。
- 避免 River 先于业务调用超时而中断正常执行。

## Cancel 与 Terminal 语义

- 取消标记落在 PostgreSQL，不再使用 Redis key。
- handler 路径对 cancel/terminal 脏任务应跳过 provider 调用并安全收敛。

## Monitor 映射

- 队列统计来源改为 River `river_job` 状态聚合与 PostgreSQL 业务表运行态计数。
- API 返回字段保留面向前端的语义名，但不再映射 Redis/Asynq 内部结构：
  - `global_queue_len` 表示 River queued states 聚合，即 `available`、`scheduled`、`retryable` 状态任务总量。
  - `wait_queue_len` 是旧前端字段名的保留字段；River 后端固定返回 `0`，不表示 Redis wait queue 或 River 待执行任务量。
  - `running_tasks` 来自 PostgreSQL `tasks.running` 运行态计数。
  - `account_loads` 来自 PostgreSQL 运行态任务计数与 `api_accounts.max_concurrent` 容量配置。
- AccountService 对外保留 `WaitQueueCount` 字段名；River 后端将全局 queued 聚合映射到该字段，语义为 River 总待处理量，不是 Redis wait queue 长度。

## 明确禁用项

- 禁止恢复 Redis list queue、Asynq worker 或兼容 backend 开关。
- 禁止以 Dragonfly/Garnet 作为 Redis 替代来继续承载任务队列。
