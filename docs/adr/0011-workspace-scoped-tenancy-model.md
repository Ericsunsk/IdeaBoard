# ADR-0011: Workspace-Scoped Tenancy Model

## Context

Workspace 是 Boards、Tasks、Library Assets 与 Media Assets 的租户隔离边界。
业务用户身份、Workspace membership 与计费 identity 各有职责；共享素材与协作操作
必须保持 Workspace 隔离，同时保留跨域 transaction 的原子性。

## Decision

1. `workspaces.public_id` (`ws_…`) is the only tenancy key that crosses the
   API boundary. Internal numeric ids never appear in URLs, payloads, or
   events (`users.public_id` stays the public identity; `users.id` is Join-only).
2. Membership gating lives at the HTTP handler layer
   (`RequireWorkspaceCapability`): hidden workspaces and cross-workspace ids
   converge to 404 for anti-enumeration. Visible Workspaces with insufficient
   capabilities may return 403. Stores additionally key every statement by resolved
   `workspace_id`, so a missed gate still cannot leak a cross-workspace row.
3. Each module owns its workspace-scoped persistence behind a narrow seam
   (`WorkspaceLibraryStore`, `WorkspaceMediaStore`,
   `workspacetasks.WorkspaceTaskStore`); repository methods take
   `context.Context` first and use the `dbWithContext` pattern.
4. Tasking owns the submission transaction: submission claim, routing selection,
   Quote, Credit Hold, Task insert, Routing Snapshot persistence, and River
   enqueue commit together. Missing transaction seams fail closed. Terminal
   Task transitions converge Capture/Release in the same transaction.
5. Library assets store provider URL + source metadata only (never bytes);
   Media owns managed bytes and materialization. Task outputs reference Media Assets by `asset_…` public id or
   by the explicit remote-output contract (URL + checksum, no storage internals).

## Consequences

Workspace 隔离由 HTTP membership/capability 检查与持久化查询的 scope 共同保证。
Task submission 与结算依赖同一 PostgreSQL transaction，不能拆成跨库 dual write。
Library 保存外部引用不等于 Media 已准备完成；materialization 由 Media 完成原图入库，
Display Tier 在读取时渲染。Workspace worker 只消费已固定的 Routing Snapshot。

## Status

Accepted。Task transaction 契约见 [DESIGN.md](../../DESIGN.md) §3 与
[Tasking Context](../../apps/api/internal/tasking/CONTEXT.md)；Routing Snapshot
契约见 [ADR-0012](./0012-service-model-pool-routing-contract.md)，Media delivery
契约见 [ADR-0007](./0007-media-delivery-pull-model.md)。
