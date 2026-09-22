# ADR-0009: AccountPool Retirement

## Context

ADR-0008 确立网关唯一出口后，AccountPool 的账号 key、失败计数、调度与管理面失去存在理由。但 `tasks`/`task_leases` 有 RESTRICT 外键咬住账号与端点表，且项目未上线、无历史包袱、无需规划回滚。

## Decision

1. 删除：账号 key、失败计数、调度代码、传输/鉴权/重试实现、Admin supplier-config vertical、executor 内 provider 重试循环、per-account 超时配置面。
2. 行保留停写：`api_accounts` 与 `provider_endpoints` 行保留为只读历史（删表等于删账单事实）；`api_account_secrets` 行保留、料 purge（payload 置空加 retired）。
3. 真删：`task_leases`（级联 `task_attempts`）由 execution-attempt 新表取代、不并存——含 task id、River job id、attempt 号、execution token、requested/resolved model、回执 id、时序、终态、分类失败；finalize 要求当前 token，generation 语义实施前写清。`provider_endpoint_capabilities`、`provider_endpoint_models`、`api_account_models` 发 migration DROP。
4. Lease 四元 fence 退役，替换为 task id 加网关 request id 的幂等判定。
5. 发布顺序：PR-1 建网关接缝，PR-2 切流，PR-3 在 burn-in 后删监控；`apps/api/CONTEXT.md` 的 AccountPool 描述随 PR-2 代码删除同步更新。

## Consequences

- 正面：账号体系零双写面；stale-worker 拒绝语义由新表显式承载，比隐式 lease 更易审计。
- 代价：无回滚数据保障（已接受）；旧表行永久保留，数据字典需标注只读。
- 排除项：任务表与账单事实不受影响；统计口径替换另行处理。

## Status

Accepted（2026-09-16，方案封版确认）。
