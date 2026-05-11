# Asynq Migration Notes (Phase 2A, Asynq-Only)

更新日期：2026-05-11

## 基线决策

1. 生产队列底座统一为 Asynq（`drawing:default` + `drawing:generate`）。
2. 不保留 legacy Redis list queue 生产兼容路径，不提供 backend 切换开关。
3. Redis 继续用于账号并发、取消标记、账号负载等状态存储。

## Retry 对齐

- DB 业务重试上限仍由 `TaskRuntimeService` 控制（当前默认 3 次）。
- Asynq `MaxRetry` 仅覆盖基础设施级重投（worker crash/ack 丢失等）。
- 账号容量不足返回 typed `ErrNoAccountCapacity`，由 Asynq `IsFailure/RetryDelayFunc` 延迟重试，不标记 DB failed。

## Timeout 对齐

- Asynq task timeout 采用账号超时上限之上的保护值（默认 180s，可配置）。
- 避免 Asynq 先于业务调用超时而中断正常执行。

## Cancel 与 Terminal 语义

- 取消标记继续使用 Redis `drawing:task:cancelled:<task_id>`。
- handler 路径对 cancel/terminal 脏任务应跳过 provider 调用并安全收敛。

## Monitor 映射

- 队列统计来源改为 Asynq Inspector。
- 兼容旧字段语义：
  - `global_queue_len = pending + scheduled + retry`
  - `wait_queue_len = active`
- 账号负载仍来自 Redis 并发槽位。

## 明确禁用项

- Asynq 路径禁止调用会写 `drawing:task:wait_queue` 的 legacy `AssignTask`。
- 生产启动路径禁止 legacy Redis list consumer。
