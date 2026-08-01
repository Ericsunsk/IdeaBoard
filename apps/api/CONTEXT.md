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
| Billing Records | `internal/billingrecords` | 用户侧最终 Idea 收支 Read Model；组合 Credits Ledger 与 immutable Task Billing Snapshot，不拥有账务写入 | `billingrecords.Service.List`, authenticated billing records handler |
| Credits | `internal/credits` | 平台 Credit Wallet（用户展示为 Idea）、Double-entry Ledger、余额 projection、幂等 Commands | `credits.Commands`, `credits.WalletReader`, `credits.Service.InTx`, `credits.CommandsInSQLTx` |
| Rewards | `internal/rewards` | RewardCode、Redemption、用户 history 与 Credits Grant 编排 | `rewards.AdminCommands`, `rewards.AdminReads`, `rewards.UserCommands`, `rewards.UserReads` |
| [Payments](./internal/payments/CONTEXT.md) | `internal/payments` | Stripe 配置、签名 Webhook Inbox 与支付生命周期；不拥有 Idea 余额或 Ledger | `payments.Service`, `payments.StripeGateway`, `payments.StripeWebhookVerifier` |
| Usage | `internal/usage` | 用户侧 Idea 消耗的 Lifetime 摘要、每日 Capture Trend 与 durable logical AI Task submission 只读聚合 | `usage.Service`, caller-owned `CaptureUsageReader`, caller-owned `LogicalTaskUsageReader` |

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
- API 启动会在 River worker 启动前把遗留的 `running` tasks 经 durable final-failure convergence 收敛为 `failed`；`pending` tasks 保持队列语义，active leases 与 Credit Hold 在同一 transaction 释放，旧 River jobs 通过 River public API 做 best-effort cleanup。
- NATS JetStream is an event layer for task status, audit and realtime fanout; it is not a task queue.
- Worker concurrency 只读取部署级 `RIVER_WORKER_CONCURRENCY`；retry/timeout policy 由 Worker implementation 持有，NATS retention 由 Events implementation 固定为 7 天。
- Admin 只管理 typed `OutboundPolicySetting`。Egress allowlist 在每次 outbound operation 前从 DB 读取，动态生效且读取失败时 fail closed；写入必须在同一 PostgreSQL transaction 内更新 `system_configs` 与 append-only actor audit。
- Worker implementation stays in `internal/runtime/worker` as an execution adapter.
- Workflow owns executable feature contracts; Feature Prompt configuration only owns the current prompt template and is updated in place without version or revision history. Billing availability gates submission, while the worker resolves the current enabled Provider Account and Model at execution time.
- `tasks.result` may hold a durable result envelope. Successful image tasks keep top-level `image_url`/`image_urls` for Canvas compatibility; workflow-specific parse failures may store `parsed_output_status`, `parsed_output` and a bounded `raw_provider_output_preview` so Admin/Canvas can diagnose retry or failed states without treating them as successful outputs.
- Unless schema or query contracts change, do not edit migrations or generated sqlc code.
- Credits Ledger 与 Tasking 共用 PostgreSQL；其他 context 只能通过 `credits.Commands` 写账，禁止直接改 balance projection 或 Ledger。
- Rewards 与 Credits 共用 caller-owned PostgreSQL transaction；Rewards 只通过 `credits.Service.InTx` Grant，不直接写 Ledger 或 balance projection。
- Rewards Redeem 使用 endpoint-private per-user in-process limiter 与 current/previous HMAC registry；startup 必须拒绝缺少 current key，或移除仍被可兑换/可 replay Code 依赖的 previous key。该 abuse control 不进入 Platform interface 或 database schema。
- Billing 只根据全局 SKU 选择 immutable price version 并生成 Quote；不读取 Provider Account routing，也不直接写 Credits Ledger。
- Credit Commands 需要与 caller 的业务状态原子提交时，pgx caller 使用 `credits.Service.InTx(pgx.Tx)`，database/sql caller 使用 transaction-scoped `credits.CommandsInSQLTx(*sql.Tx)`；两者都只调用同一个 typed Credit Command，且 caller 拥有 commit/rollback，禁止外部双写。
- canonical AI Image Workflow Submit 由 Tasking 在一个 pgx transaction 内完成 Quote/Hold/snapshot/Task/River；terminal transition 由 Task Store 在一个 database/sql transaction 内更新 Task facts，并通过 transaction-scoped `taskbilling.SettleTerminal` 原子完成 Capture/Release。Task Repository 不持有 application-lifetime Billing dependency。纯本地编辑操作不进入该计费边界。
- Task Billing settlement 使用 typed `free | held | captured | released` state machine：`0 Ideas` Quote 直接进入 `free`；收费任务只能从 `held` 幂等收敛到 `captured` 或 `released`，禁止反向或跨终态转换。
- River drawing job 在最终执行失败且 terminal convergence 暂时不可写时，会持久化 settlement-only marker 并 snooze；重启后的 retry 只做 terminal convergence，不再调用 Provider。若 marker 记录本身失败，则 enqueue 专用 settlement-only River job 作为 durable fallback。
- Identity 的用户生命周期可通过 `credits.WalletLifecycle` typed seam 原子 provision/freeze Wallet 与领取 Welcome Grant；该 seam 仍由 Credits 独占 Ledger 写入和 runtime privilege。
- Wallet lifecycle status 与原因化 Restriction 独立；`credits.WalletRestrictionCommands` 是 Restriction 唯一写入 seam。新 Grant/Deduction/Hold 同时受 lifecycle 与 active Restriction gate 约束，既有 Hold 的 Capture/Release 和 authenticated Admin repair commands 必须可收敛。
- Reconciliation 的 high-risk open Finding 通过绑定 Finding 的 Ledger Integrity Restriction 隔离消费；成功扫描确认 resolved、high→low 或 Wallet 归属变化时只解除对应 Restriction，不修改 Wallet lifecycle status。
- Reconciliation Run 明确区分 manual/scheduled Trigger；Stale Hold 固定 24 小时并仅保留为 DB 内部 per-Run snapshot，不进入 Admin request 或 public Run DTO。River PeriodicJobs 每天 03:00 Asia/Shanghai 安排 daily ScheduleJob，并在 leader 启动时用 catch-up ScheduleJob 检查是否超过 24 小时无成功 Run；ScheduleJob 只调用 Credits 内部 transaction-bound start seam，遇到其他 active Run 时 snooze，failed target slot 可复用原 Run 重新入队。
- Admin Credit Health 是 Reconciliation 的当前 Read Model：在单个 repeatable-read snapshot 内组合 active Run、最近成功/失败 Run、open Finding 汇总和精确 Ledger Integrity Restriction Wallet 数；active Run 不覆盖上一次可信结果，超过 48 小时无成功检查为 stale。
- 用户删除以 `user_deletion_requests` 为 durable state：先冻结业务用户与 Wallet，再按 Auth → Storage → tombstone phase 幂等收敛；跨服务失败不得恢复 active Wallet。
- Usage range 是 typed `7d | 30d | 1y`，对应包含今天的 7 / 30 / 365 个 `Asia/Shanghai` daily buckets；range 只过滤 Trend，Lifetime 口径不变。
- Usage 实时组合 immutable Credit Capture 与 Task Billing snapshot，并使用 feature-local、容量 2,048、TTL 约 30 秒的 per-process LRU cache。cache 必须按 `(user_id, range)` 隔离并 copy-on-read/write；多实例最多约 30 秒 eventual freshness，不引入 Redis invalidation 或 Daily Projection。
- Billing Records 只读组合 Credits Ledger 与 immutable Task Billing Snapshot：使用 authenticated `user_id` 与 Ledger sequence keyset pagination，隐藏 Hold/Release/Refund 等内部或暂不展示的流水，并由后端输出稳定 `kind/title`，前端不得解析 Ledger `source/reason`。

## When To Expand A Context

Create or expand a backend context only when at least one condition is true:

- A domain area has multiple packages whose names become clearer when grouped.
- Callers need to understand sequencing, status transitions, persistence rules or error handling that should live behind a smaller module interface.
- Testing a business workflow requires assembling too many low-level collaborators.
- A group of packages already shares stable domain language that is clearer than technical-layer naming.

If none of these apply, keep the current package location and improve the local module interface directly.
