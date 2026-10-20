# ADR-0023: Workspace Hard Delete

## Context

工作区删除原来是两段式：`POST /workspaces/{workspace_id}/archive` 写 `archived_at` 软删，非 Default workspace 7 天后由 daily purge（`workspace_purge_schedule`，`internal/tenancy/application/purge`）硬删，boards 与文件连带清理、billing 记录保留。

这个 7 天是“假恢复期”：workspace 没有 un-archive/restore 路径（board 有 `restore`，workspace 没有），archived 状态下不可见、不可操作、不可自助找回——7 天只推迟删除，不提供恢复。同时 archived 中间态要长期维护：读写 guards、purge 调度与保留窗口、超时矩阵里的保留窗口排障项，都要为一个不提供恢复的中间态买单；用户侧“7 天后彻底删除”文案反而暗示期间可找回。

约束不变：Default workspace 永不删除/归档/改名；账单与账本事实不可动；备份是整库 dump（见 data-protection runbook）。

## Decision

1. 取消 7 天恢复期：用户可观察语义改为删除确认后即硬删、即不可用、不可恢复；不再存在 archived 等待态与 7 天窗口。
2. 删除范围与红线沿用 `internal/tenancy/application/purge` 的既有划分：workspace-owned 行（tasks、boards 含 revisions、library、media、uploads、memberships、events/subscriptions、outbox 等）连同已收集的存储对象一次性清理；存储对象先收 keys、行提交后 best-effort 删除（失败只记日志/计数，不回滚行删除）。
3. 红线永不删除：`credit_*`（immutable trigger）、`task_billing_snapshots`/`settlements`、users、payments、rewards、`credit_wallets`、statistics aggregates；billing records 作为对账与法务事实保留。
4. 删除前在 Workspace 行锁下检查活动 Task：存在 pending/running Task 时返回 409 `workspace_active_tasks`，由现有 Task lifecycle 完成或取消并原子结算 Credit Hold 后才能删除；不在 Tenancy 复制 settlement 或绕过账本。Workspace FK 锁与该行锁串行化并发提交，避免检查后插入新 Task。
5. Default workspace 保护不变：删除路径在行锁下复核 `is_default`，Default 永不删除。
6. 误删=不可恢复：不提供单 workspace 粒度恢复；唯一兜底是整库 restore（PostgreSQL dump + uploads backup 联动、人工确认），不为单 workspace 执行。运维流程与工单话术见 [data-protection](../runbooks/data-protection.md)。

## Consequences

- 正面：删除语义一次讲清；archived 分支、purge 窗口与“可找回”误导一起消失。
- 代价：误删不可恢复（已接受；窗口内原本也无自助恢复，见 Context）；“冷静期”消失，删除确认必须一次做对（destructive 二次确认，确认后不可撤销）。
- 有意排除：软删 + 自助恢复、单 workspace 粒度 restore、延迟删除队列。
- 实现侧 follow-up（不属本文档）：OpenAPI、generated frontend、前端文案与后端注释里的 7 天/purge 窗口表述随实现改动收敛；purge 已整体下线（`internal/tenancy/application/purge` 包、`task_retention` 队列、`WorkspacePurgeWorker` 均已删除），不再保留 purge 执行路径与命名，文档只改语义行。

## Status

Accepted
