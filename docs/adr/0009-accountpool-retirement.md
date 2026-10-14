# ADR-0009: AccountPool Retirement

## Context

ADR-0008 确立网关唯一出口后，AccountPool 的账号 key、失败计数、调度与管理面失去存在理由。但 `tasks`/`task_leases` 有 RESTRICT 外键咬住账号与端点表，且项目未上线、无历史包袱、无需规划回滚。

## Decision

1. 删除：账号 key、失败计数、调度代码、传输/鉴权/重试实现、Admin supplier-config vertical、executor 内 provider 重试循环、per-account 超时配置面。
2. 行保留停写：`api_accounts` 与 `provider_endpoints` 行保留为只读历史（删表等于删账单事实）；`api_account_secrets` 行保留、料 purge（payload 置空加 retired）。
3. `task_execution_attempts` 持久化执行事实：Task、River job、attempt 号、execution token、requested/resolved Model、回执、时序和分类结果。应用不维护第二套账号 lease 或 endpoint capability/model 调度事实。
4. Task execution 以 Task identity + execution token fencing；gateway request ID 只用于观测，不是 lease identity。Claim 与 Execution Attempt 原子创建，stale token 不得完成或覆盖当前 Task。

## Consequences

- 正面：账号体系零双写面；stale-worker 拒绝语义由新表显式承载，比隐式 lease 更易审计。
- 代价：无回滚数据保障（已接受）；旧表行永久保留，数据字典需标注只读。
- 排除项：任务表与账单事实不受影响；统计口径替换另行处理。

## Status

Accepted（2026-09-16，方案封版确认）。
