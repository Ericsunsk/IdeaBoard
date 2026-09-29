# Backend Context

Code under `apps/api` is organized as runtime/platform/business contexts. This file records backend-local context language, dependency direction, and extension rules only; system-level contracts live in [../../DESIGN.md](../../DESIGN.md).

## Contexts

| Context | Directories | Responsibility | Main Interfaces |
| ------- | ----------- | -------------- | --------------- |
| [Runtime](./internal/runtime/CONTEXT.md) | `cmd`, `internal/runtime` | Platform Runtime Context: explicit lifecycle state machine, sole Composition Root, HTTP/Worker interface adapters and platform adapters | `composition.Run`, `composition.BuildApp`, `interfaces/http`, `interfaces/worker` |
| Platform | `internal/platform`, `internal/db`, `internal/events` | config, database pools, response helpers, safe HTTP, settings, event bus, generated SQL | `config.Config`, sqlc queries, event publishers |
| Tasking | `internal/tasking` | Task model, submission, queueing, task billing orchestration, media validation, workspace execution lifecycle, persistence, statistics | `application/task.Service`, `application/runtime`, `application/billing.Service`, `interfaces/worker.WorkspaceTaskWorker`, `application/statistics.Service`, `infrastructure/postgres.Repository` |
| Identity | `internal/identity/auth`, `internal/identity/autherrors`, `internal/identity/users` | Auth middleware/client, user persistence, user/admin management | Auth handlers/middleware, `users.UserRepository`, `users.UserService` |
| AI Gateway | `internal/aigateway` | Sole Cloudflare AI Gateway egress config: gateway credentials, model catalog, Vision mapping, route preview and probing | `ConfigRepository.Resolve`, `ConfigRepository.ResolveVisionModel`, Admin gateway handler |
| Boards | `internal/boards` | Board ownership, access checks, board management API | `boards.Service`, board handler/repository |
| Media | `internal/media/storage`, `internal/media/derivatives`, `internal/media/assets`, `internal/media/imageformat`, `internal/media/mediarefs` | Object storage, upload serving, upload authorization, image derivatives materialization/repair/reconciliation, media references, image format validation, bundled business assets | `storage.Service`, media stores/authorizers, media reference helpers |
| Library | `internal/library` | Workspace-scoped Unsplash/Pexels external asset references (v1 API): folder management, idempotent save, typed batch resolve, materialization jobs; legacy user-scoped service removed (#809/#812); never copies objects into Media Storage | `library.WorkspaceLibraryStore` |
| Workflow/Prompts | `internal/workflow`, `internal/prompts` | Executable workflow feature contract, task parameter shaping, prompt templates, prompt assembly | Workflow helpers, prompt engine/repository/handler |
| Billing | `internal/billing` | Global SKU Catalog, versioned Credit pricing, server-side Quote snapshot. Layout: `domain` vocabulary, `application` catalog orchestration, `infrastructure/postgres` Store, `interfaces/http` handler | `application.BillableItemCatalog`, `application.CatalogStore`, `billing.Quote`, `billing.PriceSnapshot`, Admin billing handler |
| Billing Records | `internal/billingrecords` | User-facing lifetime Idea balance Read Model; composes Credits Ledger with immutable Task Billing Snapshot, owns no ledger writes | `billingrecords.Service.List`, authenticated billing records handler |
| Credits | `internal/credits` | Platform Credit Wallet (displayed to users as Idea), double-entry Ledger, balance projection, idempotent Commands | `credits.Commands`, `credits.WalletReader`, `credits.Service.InTx`, `credits.CommandsInTx` |
| [Rewards](./internal/rewards/CONTEXT.md) | `internal/rewards` | RewardCode, Redemption, user history, Credits Grant orchestration | `ports.AdminCommands`, `ports.AdminReads`, `ports.UserCommands`, `ports.UserReads` |
| [Payments](./internal/payments/CONTEXT.md) | `internal/payments` | Stripe config, signed Webhook Inbox, payment lifecycle; owns no Idea balance or Ledger | `payments.Service`, `payments.StripeGateway`, `payments.StripeWebhookVerifier` |
| Usage | `internal/usage` | User-facing Idea consumption lifetime summary, daily Capture Trend, durable logical AI Task submission read-only aggregates | `usage.Service`, caller-owned `CaptureUsageReader`, caller-owned `LogicalTaskUsageReader` |

## Dependency Direction

- `cmd` stays lightweight and delegates process wiring to `internal/runtime/composition`.
- `internal/runtime` is a Platform Runtime Context using DDD + Clean Architecture: `domain` owns explicit lifecycle state and invariants, `application` coordinates lifecycle transitions and draining budgets, and `composition` is the sole assembly root.
- Runtime interfaces (`interfaces/http`, `interfaces/worker`) and adapters (`adapters/*`) stay isolated; business contexts must never depend on `internal/runtime`.
- Business contexts may depend on `internal/platform` and `internal/db/sqlc` through their own repositories or adapters.
- `internal/platform` must not import business contexts.
- `internal/tasking` may contain multiple focused packages because Tasking covers model, submission, queueing, persistence, statistics, media validation and lifecycle state.

## Current Design Rules

- Task submission is transactional: insert the business task and enqueue the River job in the same PostgreSQL transaction.
- Canonical Task media uses ordered `AssetID + role` facts. Submit atomically creates `task_input` refs, every terminal transition releases them, reclamation protects them while active, and runtime resolves each Asset through its recorded Storage Profile into provider-only URLs; persisted Task Params never contain media URLs.
- S3-compatible credentials are control-plane only: upload, capability checks, listing, and deletion use the configured Endpoint; stored-object reads use the immutable Storage Profile `access_domain` delivery plane through `safehttp`, so Browser downloads and Provider URLs share one public delivery contract.
- Task submission idempotency uses `client_action_id` as the user-action boundary: retries of the same user action return the existing Task, while intentional reruns send a new `client_action_id`.
- River + PostgreSQL are the task queue and task state fact source.
- River redelivery recovers an interrupted `running` task by reusing its execution token to reacquire the lease; a different token remains fenced out, and no startup reaper is active.
- NATS JetStream is an event layer for task status, audit and realtime fanout; it is not a task queue.
- Worker concurrency reads only the deploy-level `RIVER_WORKER_CONCURRENCY`; retry/timeout policy is owned by the Worker implementation, NATS retention is fixed at 7 days by the Events implementation. Database pool sizing (`DB_POOL_MAX_*`, `RIVER_POOL_MAX/MIN_CONNS`) is deploy-level tuning validated at startup; per-queue River `MaxAttempts` live in `taskqueue.RiverRetryMatrix` as the single source of truth and every queue reports its own depth/lag gauges.
- Admin only manages the typed `OutboundPolicySetting`. The egress allowlist is read from DB before every outbound operation, takes effect dynamically, and fails closed on read errors; writes must update `system_configs` and the append-only actor audit in the same PostgreSQL transaction.
- Worker implementation stays in `internal/runtime/interfaces/worker` as the Runtime execution adapter. Its internal architecture is split into 5 isolated subpackages (`providers`, `river`, `billing`, `retention`, `visionbilling`) with a unified root facade (`worker.go`); `providers` decouples strictly through `runtime/ports`, while subpackage boundaries are enforced by architecture tests. (#854: the legacy `execution` drawing orchestrator is deleted; workspace task execution lives in `internal/tasking/interfaces/worker`.)
- Workflow owns executable feature contracts; Feature Prompt configuration only owns the current prompt template and is updated in place without version or revision history. Billing availability and Service Model Pool routing must both gate submission before Hold. Quote (task-quotes and billing-quotes for ai-image SKUs), readiness preflight, and submit share one `RoutingResolver` availability decision (`422 service_resolution_unavailable`); submit resolves and pins the routing snapshot atomically with Credit Hold, task insert, and River enqueue. Omitted resolutions are normalized to the Service policy's canonical `1k`/`2k`/`4k` value; `params.output_resolution` is the client source of truth and the top-level billing resolution must agree with it exactly. Only `1k`, `2k`, and `4k` are accepted; `default` is not a request or billing variant.
- `tasks.result` may hold a durable result envelope. Successful image tasks keep top-level `image_url`/`image_urls` for Canvas compatibility; workflow-specific parse failures may store `parsed_output_status`, `parsed_output` and a bounded `raw_provider_output_preview` so Admin/Canvas can diagnose retry or failed states without treating them as successful outputs.
- Unless schema or query contracts change, do not edit migrations or generated sqlc code.
- Credits Ledger shares PostgreSQL with Tasking; other contexts may only write ledger state through `credits.Commands`, never by touching balance projections or the Ledger directly.
- Rewards shares a caller-owned PostgreSQL transaction with Credits; Rewards only Grants through `credits.Service.InTx`, never writing the Ledger or balance projections directly.
- Rewards Redeem uses an endpoint-private per-user in-process limiter with a current/previous HMAC registry; startup must reject a missing current key, or removal of a previous key still depended on by redeemable/replayable Codes. This abuse control does not enter Platform interfaces or database schema.
- Billing selects an immutable price version by global SKU and generates a Quote; it does not read Provider Account routing, nor write the Credits Ledger directly.
- Credit Commands that must commit atomically with caller business state use caller-owned pgx transactions via `credits.Service.InTx(pgx.Tx)` or transaction-scoped `credits.CommandsInTx(pgx.Tx)`; both invoke the same typed Credit Command, the caller owns commit/rollback, and no double writes from outside.
- Canonical AI Image Workflow Submit completes Quote/Hold/snapshot/Task/River in one pgx transaction inside Tasking; terminal transition updates Task facts in one pgx transaction in the Task Store, and atomically Captures/Releases through transaction-scoped `settleTerminal`. The Task Repository holds no application-lifetime Billing dependency. Pure local edit operations stay outside this billing boundary.
- Task Billing settlement uses a typed `free | held | captured | released` state machine: `0 Ideas` Quotes enter `free` directly; paid tasks converge idempotently from `held` to `captured` or `released` only, never backwards or across terminal states.
- A River drawing job whose final execution fails while terminal convergence is temporarily unwritable persists a settlement-only marker and snoozes; retries after restart only run terminal convergence and never call the Provider again. If the marker record itself fails, a dedicated settlement-only River job is enqueued as durable fallback.
- Identity user lifecycle provisions/freezes Wallets and claims the Welcome Grant atomically through the typed `credits.WalletLifecycle` seam; Ledger writes and runtime privilege stay exclusively with Credits.
- Wallet lifecycle status and reasoned Restrictions are independent; `credits.WalletRestrictionCommands` is the sole Restriction write seam. New Grants/Deductions/Holds require both active lifecycle and no active Restriction; existing Holds must keep converging on Capture/Release, and authenticated Admin linked Refunds/Corrections stay available for repair, while Wallets start no new consumption.
- Reconciliation high-risk open Findings isolate consumption by binding the Finding's Ledger Integrity Restriction; confirmed-resolved scans, high→low transitions, or Wallet ownership changes release only the matching Restriction without touching Wallet lifecycle status.
- Reconciliation Runs distinguish manual/scheduled Triggers; Stale Hold is fixed at 24 hours and lives only as DB-internal per-Run snapshots, never entering Admin requests or public Run DTOs. River PeriodicJobs schedule the daily ScheduleJob at 03:00 Asia/Shanghai and use a catch-up ScheduleJob at leader start when no successful Run exists for over 24 hours; ScheduleJobs only call the Credits-internal transaction-bound start seam, snooze when another active Run exists, and failed target slots past due reuse the original Run with terminal fields cleared for re-enqueue.
- Admin Credit Health is Reconciliation's current Read Model: it composes the active Run, latest successful/failed Runs, open Finding aggregates, and exact Ledger Integrity Restriction Wallet counts inside one repeatable-read snapshot; an active Run never overrides the last trusted result, and no successful check for over 48 hours reads as stale.
- User deletion uses `user_deletion_requests` as durable state: freeze the business user and Wallet first, then idempotently converge Auth → Storage → tombstone phases; cross-service failures must never reactivate an active Wallet.
- Usage range is typed `7d | 30d | 1y`, mapping to 7 / 30 / 365 `Asia/Shanghai` daily buckets including today; range filters Trend only, Lifetime stays unchanged.
- Usage composes immutable Credit Captures with Task Billing snapshots in realtime, using a feature-local, capacity-2,048, ~30s TTL per-process LRU cache. The cache must isolate by `(user_id, range)` with copy-on-read/write; multi-instance deployments accept ~30s eventual freshness with no Redis invalidation or Daily Projection.
- Billing Records composes Credits Ledger with immutable Task Billing Snapshots read-only: it uses authenticated `user_id` with Ledger sequence keyset pagination, hides internal or not-yet-displayed flows like Hold/Release/Refund, emits stable backend `kind/title`, and the frontend must not parse Ledger `source/reason`.
- New routes render through the contract kernel: errors use `contract.Code` (callers branch on code, never message text; `StatusOf` owns the code-to-status mapping) with the single envelope (`contract.RenderSuccess` / `RenderCreated` / `RenderAccepted` / `RenderPaginated` for 2xx JSON, `contract.RenderError` / `RenderCode` for errors). The specs mirror this: 2xx JSON bodies are `{data, meta?}` (`meta` required on pages), errors are `ErrorEnvelope` / `ControlErrorEnvelope`, every response carries `Request-Id`. Frozen exceptions only: raw byte reads (media content/derivative `octet-stream`, control brand-logo SVG) and SSE streams (workspace events, control monitor) — never JSON outside the envelope.
- HTTP envelopes are three layers, and `platform/response` is not deprecated. Layer 1 is `response.*` (numeric `{code,msg,message_safe}` successes plus `MachineError` `{code,msg,message_safe}`) as the control-proxy translator input: legacy domain handlers keep rendering it, and the Control Plane translates it at the boundary (pinned by `TestControlProxy*` in `internal/runtime/interfaces/http/contract_control_proxy_envelope_test.go`). Layer 2 is the wire-stable machine envelope `{code,msg,message_safe}` edge tables (per-domain sentinel→code tables rendered via `errorrender`), frozen per the kernel code registry in `internal/platform/contract`. Layer 3 is the blessed protocol exceptions that are never JSON envelopes (health/readiness probes, Stripe webhook bytes, static upload bytes, SSE streams, cross-cutting middleware), pinned by `TestContractInventory*` in `internal/runtime/interfaces/http/contract_inventory_test.go`. New v1/control JSON handlers render via `platform/contract`; only the documented exceptions live outside it (pinned by `TestContractV1EnvelopeUnification` / `TestContractControlEnvelopeUnification`).
- Public feeds page with opaque cursor + `page_size` (`contract.ParsePageSize`: default 20, max 100; cursors come from `contract.EncodeCursor`, stay bound to principal/filters/sort, malformed input renders `pagination_cursor_invalid`). Admin/control lists bound size with `pagination.ParseLimit` (`limit`, default 50, max 200) plus an opaque `cursor`, or store-ordered `before_id` where the backing store is id-ordered (reconciliation runs/findings, reward redemptions). Never add `page` / `page_number` offsets or raw (non-opaque) cursors.
- `Idempotency-Key` is header-only (no body key): required on ledger-writing creates (checkout sessions, credit grants/deductions/refunds/corrections, task submit, reward redemption, payment refunds) and on every control mutation; optional on naturally-idempotent writes (renames, role updates, idempotent restores/revokes, read-like POSTs such as quotes, batch reads, search, route-preview) — but every mutation documents its status in the spec (the shared `IdempotencyKey` parameter when required, else an explicit `No Idempotency-Key …` description). Key reuse with the same request hash replays the first result; reuse with a different hash renders 409 `*_idempotency_conflict`.
- New ID scalar patterns must match the kernel alphabet `^<prefix>_[A-Za-z0-9_-]+$` (`contract.IsValidPublicID`) and pin the family in the `scripts/openapi-pattern-check.sh` canonical table. Reserved prefixes (`ba_`, `lmat_`, `rwd_`, `rdm_`, `rec_`) stay off the wire until their `public_id` columns land; rewards/reconciliation ids remain numeric by formal exception.
- Structural spec guard: `scripts/openapi-pattern-check.sh` asserts every operation has an `operationId` and every mutation (`POST`/`PUT`/`PATCH`/`DELETE`) states its Idempotency-Key status. Grandfathered gaps live in that script's allowlist; current spec-side residuals are report-only (fix via spec PR, never silently).

## Backend Copy Vocabulary

Backend user-facing copy lives in per-context tables (`internal/**/message.go` plus Han-literal `internal/**/errors.go` extensions); storage stays distributed, wording is managed here. New copy MUST use the canonical wording below; the banned variants MUST NOT reappear (pinned by `scripts/check-backend-copy.sh`).

| # | Concept | Canonical | Banned variants |
| - | ------- | --------- | --------------- |
| 1 | Service temporarily unavailable | `<Subject>暂不可用` (no `暂时`; ledger write drops it: `账本写入失败`) | `Stripe 服务暂时不可用`, `当前 Stripe 服务暂时不可用`, `搜索服务暂时不可用`, `视觉搜索暂时不可用`, `账本写入暂时失败` |
| 2 | Generic bad params | `参数错误` | `请求参数错误` |
| 3 | Invalid input | `<Subject>无效` | `分页游标非法`, `视觉搜索输入非法`, `page_size 非法` |
| 4 | Rate limited | `操作过于频繁，请稍后重试` | `操作频繁，请稍后重试` |
| 5 | Empty provider response | `供应商返回空响应` | `供应商返回为空响应` |
| 6 | Auth service down | `认证服务暂不可用` | `认证服务不可用` |
| 7 | Wallet restricted | `当前工作区钱包受限，暂不可用` (never truncate to `暂不可`) | `当前工作区钱包受限，暂不可` |
| 8 | Stripe denial scope | `当前 Stripe API Key 权限不足`, `当前 Stripe 账户不可用`, `当前 Stripe 账户暂不具备收款能力` (internal detail strings align to the typed contract/domain canonicals) | `Stripe API Key 权限不足`, `Stripe 账户不可用`, `Stripe 账户暂不具备收款能力` |
| 9 | System settings service missing | `系统设置服务未配置` | `系统设置服务未初始化` |
| 10 | Gateway model-mapping read | `读取网关模型映射失败` | `读取模型映射失败` |
| 11 | Terms acceptance | `请先同意服务条款与退款政策` (aligns to domain `ErrTermsAcceptanceRequired`) | `请先同意当前服务条款与退款政策` |
| 12 | Idempotency mismatch | `幂等请求与原请求不一致` | `幂等请求与原创建参数不一致` |
| 13 | Admin enum validation | `role 仅支持 admin 或 user`, `status 仅支持 0 或 1` (code-adjacent prefix, spaced digits) | `角色值无效，仅支持 admin 或 user`, `状态值无效，仅支持0或1` |

Deliberately NOT converged (context-specific, different code/state/scope — do not "fix" these):
- `未配置` vs `暂不可用` vs `不存在` (different states: never configured, transiently down, missing entity).
- `认证服务拒绝伪装登录` (`Rejected`/`Forbidden`) vs `认证服务伪装登录失败` (`Unavailable`): different codes.
- `不能操作…` (admin self-operation guard) vs `不能模拟…` (impersonation): different semantics.
- `需要管理员权限` (admin-required) vs `权限不足` (generic forbidden): different nuance.
- `*不存在` / `*失败` / `*不能为空` / `*无效` with different subjects: entity-specific, keep the subject.
- `社区模型目录*` (detect/fetch operation) vs `models.dev *` (reference-load operation): different operations.
- Suffix-shared but cause-specific strings (`请联系管理员`, `请检查配置或稍后重试`, `请稍后重试` with different verbs/subjects).

## When To Expand A Context

Create or expand a backend context only when at least one condition is true:

- A domain area has multiple packages whose names become clearer when grouped.
- Callers need to understand sequencing, status transitions, persistence rules or error handling that should live behind a smaller module interface.
- Testing a business workflow requires assembling too many low-level collaborators.
- A group of packages already shares stable domain language that is clearer than technical-layer naming.

If none of these apply, keep the current package location and improve the local module interface directly.
