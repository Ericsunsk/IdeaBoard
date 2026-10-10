# ExecPlans

## 未提交 Diff 冗余审计与提交（2026-10-10）

- Goal: 审计当前未提交 diff，删除有证据支持的冗余，并在当前 branch 提交验证后的完整改动。
- Scope: 以当前 HEAD 为基线，subagents 并行审计 Retouch、Board/Inpaint/Image Tools 及其余 API/Account/Admin/governance/docs；按互不重叠的文件范围清理。保留已接受的 feature-first layout、retouchStack hard cut 和前轮四项修复。
- Success criteria: 每项删除有调用链或 contract 证据；相关 tests、lint、type/build、generated consistency 通过，独立复核无遗留问题；commit 后工作区干净。
- Risks: 不删除 public seam、必要 boundary adapter、兼容约束或真实 regression coverage；generated files 仅通过 generator 更新；不改变 runtime behavior 或外部数据。
- Rollback: commit 前仅撤回本轮清理 patches；commit 后可 revert 本轮 commit。不切换 branch、创建 worktree 或 push。
- Results: 12 个代码/文档文件净减少 84 行；删除无消费者的私有 types/test-only helpers、重复 patch 委托与条件、无效类型包装和过时说明。保留公开 seam、默认工厂、错误分类差异及真实 regression coverage。
- Verification: Web 全量 524 files / 3441 tests、build（含 bundle budget）、test typecheck、完整 lint、style、generated API consistency、browser 3 files / 7 tests 和 Go build 全部通过。独立复核 PASS，并自行通过 6 specs / 117 tests、55 naming self-tests 与 diff check。
- Status: COMPLETE；冗余审计、清理与验证全部完成。本批通过当前 branch 的一个 commit 交付，commit identity 以 Git history 为准。

## 未提交 Diff 审计修复（2026-10-10）

- Goal: 修复本轮确认的 4 项 P2，并保留现有 staged、unstaged 和 untracked 工作。
- Scope: 当前 checkout 与 branch，由 subagents 依次修复：
  1. [x] 同步 `/user` profile fields 的 canonical OpenAPI，并重新生成 client；response/schema regression、HTTP package tests 与 generated drift check 通过。
  2. [x] 移除 Retouch `core/` 对 runtime i18n 的依赖，在 session/UI 翻译 label keys，并收紧 purity guard；166 targeted tests、dependency fixtures、lint、test typecheck 通过。
  3. [x] 保留 Sharpen visually neutral draft 参数，同时保持延迟 commit；4 个 regression 先失败后通过，100 targeted tests、lint、guardrails 通过。
  4. [x] 隔离 Gallery refresh 的旧 scope completion，防止 Workspace 切换产生虚假失败提示；11 个 regression 先失败后通过，72 targeted tests 及 boundary/contract checks 通过。
- Success criteria: 新增回归测试在修复前失败、修复后通过；相关 contract、test、lint、type/build checks 通过，独立复核无遗漏。
- Risks: 保持已有 API wire fields、layer labels、pending state machine 和真实同步错误反馈；generated client 仅通过 Orval 更新，不改 schema、auth policy 或外部数据。
- Rollback: 仅撤回本轮 targeted patches；不重置现有 diff，不切换 branch，不提交或 push。
- Verification: Web 全量 524 files / 3441 tests、build（含 bundle budget）、test typecheck、完整 lint、style、generated API consistency 及 browser 3 files / 7 tests 全部通过；Go HTTP package tests 通过。独立 subagent 复核 PASS，并自行通过 9 files / 150 targeted tests、build、test typecheck、修改文件 ESLint、dependency fixtures、generated drift 与 Go HTTP package tests。
- Status: COMPLETE；4 项已依次修复并通过整体验证与独立复核。保留现有工作区改动，当前 branch 与 HEAD 不变，未提交或 push。

## Canvas Session Save Recovery (2026-10-10)

- Goal: fix the false missing-session failure when autosaving an authenticated Canvas.
- Scope: CanvasPage identity wiring and its regression tests; preserve existing workspace diffs.
- Success criteria: the page passes the authoritative session storage namespace to the real save path; missing identity retains a local draft; refreshed identity permits saving without remounting; relevant tests, lint, formatting and type/build checks pass.
- Risk: do not invent a namespace from public_id or bypass the missing-session guard.
- Rollback: reverse only this task's patches; no schema changes or external writes.
- Status: COMPLETE; two regression tests reproduced AUTH_UNAVAILABLE before the fix and pass afterward. All 42 targeted tests, ESLint, Prettier, test typecheck and Web build passed. Browser verification confirmed import and drag autosave, durable MediaRef and matching remote positions (revision 4 to 6) without a failure toast.

## Archived Settings Diff Audit and Cleanup (2026-10-09)

- Status: implementation, redundancy cleanup and independent review complete.
- Goal: resolve the five settings audit findings, remove confirmed redundancy
  from the current batch and commit the reviewed result.
- Scope: strict Control request schemas; regenerated Auth Mail contracts; OAuth
  Settings ownership; optional Mail enabled requests and required responses;
  Mail toggle draft retention; unused resource wrappers/types/options, duplicate
  SKU validation, unreachable defaults, no-op factory and stale descriptions.
- Success criteria: valid request payloads pass while unknown fields fail;
  generated contracts have no drift; omitted enabled preserves disabled Mail;
  toggles neither save nor discard drafts; cleanup preserves existing behavior.
- Results: independent review found no remaining defect. Final verification
  passed 235 Web tests across 42 files, full frontend lint, Web build/test
  typecheck, generated API consistency, focused Auth tests/build, complete Go
  Auth/HTTP package tests, formatting and diff whitespace checks. Earlier audit
  verification also passed the full Auth suite and Go build.
- Risks: preserve operator reasons, strict binding, secret masking, Mail
  readiness and UI contracts. No deployment or live data writes.
- Rollback: revert the resulting commit; no database rollback is needed for
  cleanup. Retain the additive Mail enabled backfill if reverting application
  behavior separately.

## Archived Audit and Acceptance for #904–#914

- Status (2026-10-08): implementation and cleanup verification complete. Git
  history and issue acceptance comments record the commit and closure.

- Goal: fix the six audit defects, remove confirmed redundancy, verify the batch,
  commit on the current branch and close accepted issues.
- Scope: current batch files only. Fence Control attempts and unknown dispatch
  outcomes; restore Library's original schema/recovery scope; complete policy
  parity, persisted avatar projection and actual-folder Search feedback. Remove
  unused wrappers, duplicate gates, unreachable checks and stale descriptions.
- Success criteria: uncertain external effects never restart automatically; old
  attempts cannot settle newer claims; runtime policy matches the spec; avatar
  survives cold login; deduplicated saves report their actual folder. Preserve
  ownership seams and behavior tests, pass affected checks, commit and close issues.
- Verification: parallel Standards/Spec review; focused Control/Library/avatar/
  Search tests; frontend test typecheck/build/lint and backend build; relevant
  PostgreSQL tests; sqlc/schema/migration/API drift and formatting checks.
- Results: batch verification passed 504 frontend tests and 106 fixture-related
  tests plus relevant PostgreSQL tests and schema/generated gates. After cleanup,
  92 frontend tests, typecheck/lint/build, targeted Go tests, backend build,
  generated API drift and formatting checks passed.
- Rollback: revert the batch commit; retain additive database columns and the
  unknown-outcome fence. Reopen issues if acceptance is invalidated; never revert
  persistent databases automatically.

## Archived Image Catalog Structure

- Goal: concentrate existing Unsplash/Pexels knowledge and supplier HTTP handling.
- Scope: shared frontend provider vocabulary consumed by Search, Library and Admin;
  separate backend supplier adapters from shared HTTP handling; remove duplicate
  provider guards and unused search constants. No new supplier, UI or schema.
- Verification: existing provider IDs, errors, ordering, attribution, cursor
  encoding, URL allowlists and request behavior remain compatible; focused tests,
  frontend type/build/lint and backend build/tests pass.
- Rollback: revert only this refactor's working-tree changes; no data migration.

## Archived Workspace Task Restoration ExecPlans

> Status (2026-10-08): **ARCHIVED**. The plans below describe completed work.
> Issue acceptance comments and git history are the source of truth; do not use
> this file as an active queue. New work needs a new plan or an issue-specific
> execution note.

## Archived 8xx Batch ExecPlan (#886–#897)

This scheduler plan is archived. Issues #886–#897 were completed, verified, and
committed; the former working-tree-only constraint and lane sequencing are no
longer active. The issue comments and git history are the source of truth for
their acceptance evidence and commits. The detailed lane checklist was removed
because it duplicated the issue tracker and could imply that this work is still
pending.

## Archived Issue #886 ExecPlan: Payment Refund Persistence Boundary

- Goal: persist the admin refund reservation, `PurchaseRefundHold`, and phase
  audit before any Stripe I/O; make provider result convergence observe the
  committed refund and reuse its durable identity after retries.
- Scope: isolate the refund reservation and post-provider settlement writes
  from the Control guard's still-open transaction; preserve pending recovery,
  existing Credits commands, and the Stripe idempotency key derived from the
  durable refund ID; add focused production-path tests with a fake gateway.
- Success criteria: no Stripe call occurs before the reservation commit; a
  provider result can be recorded and settled from a fresh database connection;
  commit/audit failures leave a recoverable pending fact; retries reuse the same
  refund ID and Stripe idempotency key without a second cash refund.
- Rollback: revert the issue commit. No schema changes are expected.

## Archived Support Channels ExecPlan

- Goal: expose operator-configured external support channels from the account menu.
- Scope: typed runtime settings in the existing system config store; audited
  Control Plane reads/writes; anonymous Public API read; General settings editor;
  channel menu with a third-level QR Popover and external QR link.
- Success criteria: disabled channels never appear publicly, empty configuration
  hides Support, safe HTTP(S) links generate QR locally, channel order persists,
  and mouse/keyboard/touch interactions remain usable within the viewport.
- Verification: settings persistence and ambient rollback on isolated PostgreSQL
  schemas, route/control contract tests, frontend focused tests, browser geometry,
  type/build/lint and generated API drift.
- Rollback: revert the feature changes; no schema migration is required. The
  optional `support_channels` system config key can remain inert.

---

## Archived Workspace Task Restoration (#825–#830)

The workspace task production path, the #809 hard cut, and the runtime-grant
audit were completed and committed. The original detailed scope is retained
below as a historical acceptance summary.

### Goal

Complete the workspace task production path for issues #825 through #830:
submit, execute, render, settle, and materialize while preserving the #809
hard cut without weakening release guards.

### Scope

- #825: compile and persist a durable workflow execution plan for v1 submits.
- #826: persist task billing snapshots and settlement facts in the existing
  settlement transaction.
- #827: wire the production result fetcher and derivative renderer.
- #828: wire the library saver and make materialization billing/link state
  converge correctly on errors.
- #829: preserve the intentional #809 hard cut. No legacy routes, handlers,
  adapters, aliases, or dual-read/dual-write compatibility paths are kept.
- #830: retain the grant audit as documentation only.
- Restore the Board state contract guard removed by this diff.
- Cleanup: remove the unreferenced pre-canonical `TaskIntake` readiness,
  media-preparation, and submit-validation path. Keep the canonical workspace
  compiler, Admin monitor DTOs, and event hubs that remain production-owned.
- Routing: construct one `RoutingResolver` at the composition root and share it
  with worker settlement, task submit/readiness, task quotes, and billing
  quotes. Availability checks and routing snapshot persistence fail closed.
- Consistency: read model pools from one database transaction snapshot and keep
  task billing, routing snapshot, task insert, and River enqueue in the same
  settlement transaction.

### Success Criteria

- A submitted task carries a valid durable execution plan and can reach the
  provider gateway.
- Task state, Credits commands, and task billing facts commit atomically.
- Remote provider outputs become media assets and materialize into Quick Saves.
- The #809 hard cut is documented and tested.
- Targeted Go and Web checks pass with no generated-code hand edits.

### Risks And Rollback

- Transactional billing changes affect financial facts; all writes use the
  existing pgx transaction and Credits command seams.
- Remote output fetching is constrained to trusted provider origins and size
  limits. A fetch or render failure remains terminal and releases task billing.
- Each issue is committed independently, so rollback is a commit revert.
- Legacy cleanup is a hard cut: deleted symbols and files have no compatibility
  aliases, dual reads, dual writes, or fail-open fallbacks. Rollback is the
  commit revert before deployment.

---

## 本轮审计修复 ExecPlan

## Goal

修复本轮审计确认的 5 项缺陷，并使新增测试符合现有 guardrails。

## Scope

保持当前 checkout 与 branch；保留已有 diff。由 subagents 依次修复：

1. [x] Auth login lockout 并发计数丢失；真实 PostgreSQL 与 Auth 209/209 tests、build、lint 通过。
2. [x] Schema / migration 对固定 runtime role 的依赖；fresh snapshot、Atlas replay/parity、sqlc、DB targeted tests、Auth privilege tests 通过。
3. [x] Admin z-index utilities 未生成；Tailwind compile regression、production CSS、typecheck、style lint、build 通过。
4. [x] Storage toggle 成功后状态回跳；workspace/page 29/29 regression tests、scope lint、Prettier 通过。
5. [x] Search provider 同一行并发写入状态失真；同时保护父级列表乱序回填，page/component/resource/helper 49/49 tests、typecheck、scope lint、Prettier 通过。
6. [x] 修复新增测试的 guardrails 问题，完成整体验证与独立复核。

## Verification Results

- Auth 210/210 tests（真实 PostgreSQL，0 skipped）、build、lint 通过。
- Web 全量 3282/3282 tests、lint、test typecheck、style lint、build 通过；追加乱序回填补修后，相关 49/49 tests、typecheck、build 通过。
- OpenAPI client 经 Orval 生成，api:check-generated 通过；schema release、Atlas replay/parity、sqlc 及真实 DB settings/http tests 通过，Go build 通过。
- 独立 verifier 最终 PASS；补修后独立 page/table 36/36 tests 通过。
- Go 全量仍有原有 worker integration fixture 缺少 task_status_event_outbox 的失败；该目录与 events 无本轮 diff。Identity 全量出现一次失败，单独复跑通过。
- 全局 Prettier 仍有 4 个 pristine 文件警告：routes.ts、pricing/demoData.ts、canvas/CONTEXT.md、SiteBrandLogo.vue；当前 diff 的格式检查已修正。

## Success Criteria

每项有覆盖实际触发场景的 regression check。相关 tests、typecheck/build、lint
通过；Auth 并发与 PostgreSQL schema/runtime privileges 尽可能使用 isolated local
database 验证。说明任何未完成的环境验证，不将 skipped tests 视为通过。

## Risks

Auth 并发必须覆盖多实例，而非仅进程内锁。Schema 修改不手改 generated snapshot；
不改写已应用 migration。保留 fail-open policy 与 runtime least privilege。

## Rollback

仅撤销本轮针对目标文件的补丁，保留用户已有改动。数据库验证使用 isolated schema / 临时
database；本轮不执行生产 migration、部署、branch 切换或 remote push。

---

## Fixture 与 Diff 收尾 ExecPlan

### Goal

修复 worker integration fixture 缺少 task_status_event_outbox，审计并清理当前 diff
中的确定性冗余，完成验证后提交当前已审计的改动。

### Scope

- [x] 对齐 worker fixture 与既有 outbox schema，真实 PostgreSQL worker tests 通过；追加成功提交与失败清理断言。
- [x] 由 subagents 审计 Web 与 API/Auth diff 的重复、无调用代码、过时注释及多余分支，按证据做最小清理。
- [x] 完成 Go 全量及受影响 Web/Auth checks，独立复核。
- [x] 创建 local commit（与本 ExecPlan 一并提交）。

### Success Criteria

worker integration tests 不再缺表；Go 全量使用真实 UTF8 PostgreSQL；冗余清理保留行为
与 regression coverage；生成代码使用生成器更新。提交前检查 staged diff 与敏感内容。

### Risks And Rollback

当前 diff 含 auth/permission 与 migration 改动，必须保留既有 least privilege、transaction
和 schema replay 契约。本轮 fixture 不变更生产 schema。仅在当前 branch 提交，不部署或 push；
回滚本轮补丁或 local commit 时保留用户其余工作。

### Progress

Go 全量再次复现 Identity sweep fixture 的毫秒 rounding 竞赛：TIMESTAMPTZ(3)
DEFAULT now() 可能晚于立即执行的 app clock。测试 seed 改为明确已到期 intent，
保留严格行为断言；目标测试连续 100 次、Identity 全套均通过，无 production 改动。

### Verification Results

- Go 全量 `-count=1 -p 2 ./...` 使用真实 UTF8 PostgreSQL 且设置 required，通过；Go build、schema release 通过。
- Auth 210/210 tests（真实 PostgreSQL，0 skipped）、build、lint 通过。
- Web 516 files / 3287 tests、build、lint、test typecheck、style lint、generated drift 通过。
- 冗余清理：删除未发布重复 REVOKE migration；StatusBadge 成为单一状态渲染实现；删除 18 个同值 dark overrides；16 处 routine headers 构造收口，保留 reason/retry-key contracts；清理空分支和过时注释。
- 独立 verifier PASS；focused Web 38/38、真实 PostgreSQL settlement test、diff check 通过。
- 全局 Prettier 的 4 个原有 pristine 文件警告未纳入本轮清理；当前修改文件的格式检查通过。
