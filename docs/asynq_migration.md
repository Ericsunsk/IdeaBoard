# Asynq Migration Notes (Phase 2 Input)

更新日期：2026-05-11

## Queue Name

- 规划队列名：`drawing:default`（对应 `taskqueue.AsynqQueueDefault`）。

## Retry 对齐

- 当前 DB 侧重试上限：`defaultTaskRetryLimit = 3`（`TaskRuntimeService`）。
- Phase 2 建议：Asynq `MaxRetry` 作为传输层兜底，不替代 DB 业务重试计数。
- 预期关系：DB retry 仍是业务真值，Asynq retry 只覆盖 worker crash/ack 丢失等基础设施故障。

## Timeout 对齐

- 当前超时来源：账号 timeout + provider HTTP timeout。
- Phase 2 建议：Asynq task timeout 设置为“略大于账号超时”的基础上限，避免 worker 层先被 Asynq 超时中断。

## Unique 任务

- 建议使用 `task_id` 做 `Unique` key（有限 TTL），减少重复 submit 的并发重复执行窗口。
- 仍需保留 DB 状态 guard（pending/running/completed/failed）以应对 at-least-once。

## Cancel 过渡

- Phase 1 仍使用 Redis `drawing:task:cancelled:<task_id>`。
- Phase 2 可先保留该 key 作为取消真值，再逐步迁到 Asynq Inspector + 自定义取消索引。

## Admin Monitor 迁移

- 现阶段 monitor 读取 `RedisManager.GetMonitorInfo()`。
- Phase 2 迁移方向：使用 Asynq Inspector（queue 长度、active/retry/scheduled）+ 账号负载（仍来自现有并发槽位或新指标）合并展示。
