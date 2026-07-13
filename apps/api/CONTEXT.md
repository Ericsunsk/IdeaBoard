# Backend Context

`apps/api` 的后端代码按 runtime/platform/business contexts 组织。本文只记录 backend-local context language、dependency direction 和扩展规则；系统级设计 contract 见 [../../DESIGN.md](../../DESIGN.md)。

## Contexts

| Context | Directories | Responsibility | Main Interfaces |
| ------- | ----------- | -------------- | --------------- |
| Runtime | `cmd`, `internal/runtime` | 启动 API 进程、注册 HTTP routes、装配 worker adapters、服务 embedded frontend dist | `bootstrap`, `httpapi.RegisterRoutes`, `worker.RiverTaskWorker` |
| Platform | `internal/platform`, `internal/db`, `internal/events` | config、database pools、response helpers、safe HTTP、settings、event bus、generated SQL | `config.Config`, sqlc queries, event publishers |
| Tasking | `internal/tasking` | Task model、submission、queueing、Task Billing orchestration、media validation、runtime lifecycle、persistence、statistics | `taskservice.Service`, `tasksubmission.Store`, `taskbilling.Service`, `taskqueue.TaskAssigner`, `taskruntime.Service`, `taskstats.Service`, `taskstore.Repository` |
| Identity | `internal/identity/auth`, `internal/identity/autherrors`, `internal/identity/users` | Auth middleware/client、user persistence、user/admin management | Auth handlers/middleware, `users.UserRepository`, `users.UserService` |
| AccountPool | `internal/accountpool/accounts`, `internal/accountpool/channels` | Account pool、channel config、account load、task scheduling visibility | `accounts.AccountService`, account/channel handlers and repositories |
| Boards | `internal/boards` | Board ownership、access checks、board management API | `boards.BoardService`, board handler/repository |
| Media | `internal/media/storage`, `internal/media/assets`, `internal/media/imageformat`, `internal/media/mediarefs` | Object storage、upload serving、upload authorization、media references、image format validation、bundled business assets | `storage.Service`, media stores/authorizers, media reference helpers |
| Workflow/Prompts | `internal/workflow`, `internal/prompts` | Executable workflow feature contract、task parameter shaping、prompt templates、prompt assembly | Workflow helpers, prompt engine/repository/handler |
| Billing | `internal/billing` | 全局 SKU Catalog、versioned Credit pricing、server-side Quote snapshot | `billing.Service.Quote`, `billing.PriceSnapshot`, Admin billing handler |
| Credits | `internal/credits` | 平台点子 Wallet、Double-entry Ledger、余额 projection、幂等 Commands | `credits.Commands`, `credits.WalletReader`, `credits.Service.InTx`, `credits.Service.InSQLTx` |

## Dependency Direction

- `cmd` stays lightweight and delegates process wiring to `internal/runtime/bootstrap`.
- `internal/runtime` may compose platform modules and business contexts.
- Runtime adapters may depend on Tasking interfaces, but business contexts must not depend on `internal/runtime/worker` implementation details.
- Business contexts may depend on `internal/platform` and `internal/db/sqlc` through their own repositories or adapters.
- Business contexts should avoid importing each other's implementation details. Prefer a small service/repository interface at the caller boundary.
- `internal/platform` must not import business contexts.
- `internal/tasking` may contain multiple focused packages because Tasking covers model, submission, queueing, persistence, statistics, media validation and lifecycle state.

## Current Design Rules

- Task submission is transactional: insert the business task and enqueue the River job in the same PostgreSQL transaction.
- Task submission idempotency uses `client_action_id` as the user-action boundary: retries of the same user action return the existing Task, while intentional reruns send a new `client_action_id`.
- River + PostgreSQL are the task queue and task state fact source.
- API 启动会在 River worker 启动前取消遗留的 `running` tasks；`pending` tasks 保持队列语义，active leases 会释放，旧 River jobs 通过 River public API 做 best-effort cleanup。
- NATS JetStream is an event layer for task status, audit and realtime fanout; it is not a task queue.
- Operational System Settings 分为三个 typed groups：`TaskRuntimeSetting`、`OutboundPolicySetting` 与 `EventRetentionSetting`。River 与 NATS retention 在进程启动时形成 effective snapshot；Admin 更新后通过 `restart_required` 暴露 configured/effective 差异。Egress allowlist 在每次 outbound operation 前从 DB 读取，动态生效且读取失败时 fail closed。
- Admin operational settings write 必须携带 actor 与 reason，并在同一 PostgreSQL transaction 内更新 `system_configs` 与 append-only `system_settings_admin_actions`；禁止实现通用 env/string settings 编辑器。
- Worker implementation stays in `internal/runtime/worker` as an execution adapter.
- Workflow owns executable feature contracts; Prompts owns template storage and rendering.
- `tasks.result` may hold a durable result envelope. Successful image tasks keep top-level `image_url`/`image_urls` for Canvas compatibility; workflow-specific parse failures may store `parsed_output_status`, `parsed_output` and a bounded `raw_provider_output_preview` so Admin/Canvas can diagnose retry or failed states without treating them as successful outputs.
- Unless schema or query contracts change, do not edit migrations or generated sqlc code.
- Credits Ledger 与 Tasking 共用 PostgreSQL；其他 context 只能通过 `credits.Commands` 写账，禁止直接改 balance projection 或 Ledger。
- Billing 只根据全局 SKU 选择 immutable price version 并生成 Quote；不读取 Provider Account routing，也不直接写 Credits Ledger。
- Credit Commands 需要与 caller 的业务状态原子提交时，pgx caller 使用 `credits.Service.InTx(pgx.Tx)`，database/sql caller 使用 `credits.Service.InSQLTx(*sql.Tx)`；两者都只调用同一个 typed Credit Command，且 caller 拥有 commit/rollback，禁止外部双写。
- canonical AI Image Workflow Submit 由 Tasking 在一个 pgx transaction 内完成 Quote/Hold/snapshot/Task/River；terminal transition 由 Task Store 在一个 database/sql transaction 内完成 Capture/Release 与 Task result/status。纯本地编辑操作不进入该计费边界。
- Task Billing settlement 使用 typed `free | held | captured | released` state machine：0 点子 Quote 直接进入 `free`；收费任务只能从 `held` 幂等收敛到 `captured` 或 `released`，禁止反向或跨终态转换。
- River drawing job 在最终执行失败且 terminal convergence 暂时不可写时，会持久化 settlement-only marker 并 snooze；重启后的 retry 只做 terminal convergence，不再调用 Provider。若 marker 记录本身失败，则 enqueue 专用 settlement-only River job 作为 durable fallback。
- Identity 的用户生命周期可通过 `credits.WalletLifecycle` typed seam 原子 provision/freeze Wallet 与领取 Welcome Grant；该 seam 仍由 Credits 独占 Ledger 写入和 runtime privilege。
- 用户删除以 `user_deletion_requests` 为 durable state：先冻结业务用户与 Wallet，再按 Auth → Storage → tombstone phase 幂等收敛；跨服务失败不得恢复 active Wallet。

## When To Expand A Context

Create or expand a backend context only when at least one condition is true:

- A domain area has multiple packages whose names become clearer when grouped.
- Callers need to understand sequencing, status transitions, persistence rules or error handling that should live behind a smaller module interface.
- Testing a business workflow requires assembling too many low-level collaborators.
- A group of packages already shares stable domain language that is clearer than technical-layer naming.

If none of these apply, keep the current package location and improve the local module interface directly.
