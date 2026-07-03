# IdeaBoard Triage Audit

## 状态

- Phase: Triage Audit
- Fixed point: `be0070d test(web): prune low-signal specs`
- Last updated: 2026-07-03
- Goal: 识别未来 1-2 个月最可能影响生产稳定性的系统风险。
- Bias: production stability、security boundary、data consistency、async runtime、deployability。

本文记录 triage 证据。`Unknown` 表示当前审计还没有证明该项安全或不安全，不自动等同于缺陷。

## 范围

- Baseline & CI
- Auth / Permission / Ownership
- Task / Async Runtime
- Media / Storage
- Deployment / Ops
- Frontend State Integrity
- Architecture / Maintainability

## 非目标

- 不做 broad refactor。
- 不做 UI/style audit。
- 不做 speculative rewrites。
- triage 阶段不修问题，除非出现很小且阻塞安全运行的 P0。

## Baseline

| Area | Command | Result | Notes |
| --- | --- | --- | --- |
| Web quality gate | `cd apps/web && npm run lint && npm run lint:style && npm run test:run && npm run build` | Pass | `132` 个 spec files、`655` 个 tests 通过；Vite build 通过。 |
| Web generated API drift | `cd apps/web && npm run api:check-generated` | Pass | Generated API client up to date。 |
| Web targeted audit specs | Targeted Vitest subset | Pass | Explorer 跑了 frontend state/architecture 子集：`12` 个 spec files、`60` 个 tests 通过。 |
| Auth service | `cd apps/auth && npm run build && npm test` | Pass | `13` 个 tests 通过。测试输出提示 `BETTER_AUTH_SECRET` entropy/length 偏低，作为 production guard 证据保留。 |
| API | `cd apps/api && go test ./... && go build ./... && go tool sqlc generate && git diff --exit-code -- internal/db/sqlc` | Pass | Go tests/build 通过；sqlc generation 无 drift。 |
| CI files | `.github/workflows/{web,docker-images,release,ops}.yml` | Covered | CI workflow files 存在；本轮未证明 CI matrix 与本地 Baseline 完全等价。 |

## Finding Format

后续把 finding 提升成 GitHub Issue 时，使用以下结构：

- Evidence: 具体 file、function、test、command。
- Impact: 对用户或 operator 的破坏方式。
- Recommended Fix: 最小兼容修复。
- Verification: test、smoke、drill 或 command。
- Owner Context: 可能 owner module 和相关 docs。

Triage status：

- Covered: 已有 code、test、runbook 或 command 证据。
- Gap: 证据显示可能缺少 test、guard 或 operational proof。
- Unknown: 需要 deep audit 后才能判断。

## Track 0: Baseline & CI

### Covered

- Web、Auth、API 的本地 Baseline 全绿。
- `apps/web` 有 lint、stylelint、Vitest、Vue type/build 和 generated API drift check。
- `apps/api` 有 Go tests/build 和 sqlc drift verification。
- CI workflow files 覆盖 web、docker images、release、ops。

### Gap

- 本地 Baseline 没有证明 Docker/release/ops workflows 执行相同 gate。
- 本轮没有跑 restore、object storage、NATS-backed integration smoke。

### Unknown

- production CI 是否会阻断所有本审计列出的高风险 gate。
- deployment smoke 结果是否会作为可审计 artifact 留存。

## Track 1: Auth / Permission / Ownership

### Covered

- API auth/admin middleware 位于 `apps/api/internal/identity/auth/middleware.go`，入口是 `AuthMiddleware` 和 `AdminMiddleware`。
- Authenticated user routes 通过 `apps/api/internal/runtime/httpapi/routes_user.go` 的 `registerAuthenticatedUserRoutes` 挂载。
- Authenticated admin routes 通过 `apps/api/internal/runtime/httpapi/routes_admin.go` 的 `registerAuthenticatedAdminRoutes` 挂载。
- Board ownership checks 通过 `apps/api/internal/boards/service.go` 的 `resolveOwnerBoard` / `requireOwnerBoard`，底层是 `boards.Repository.GetByOwnerAndBoardID`。
- Upload read authorization 收口在 `apps/api/internal/runtime/httpapi/upload_static_handler.go` 和 `apps/api/internal/media/storage/upload_authorization.go`。
- Auth service 的 entry/CORS/env guard 位于 `apps/auth/src/env.ts`、`apps/auth/src/authEntryGuard.ts`、`apps/auth/src/authApp.ts`。
- 运维边界记录在 `docs/auth-operation-runbook.md`。

### Gap

- 未找到 `AdminMiddleware` 的直接测试。
- `apps/auth/src/env.ts` 的 production secret 和 trusted-origin validation 需要直接测试。
- Auth test 输出里的 production guard warning 应转成 deterministic production-startup test。

### Unknown

- 所有 admin、board、upload routes 是否都没有 bypass path。
- `site/assets` public access policy 是否已经有意图说明和测试。
- deployment proxy/CORS 配置是否比 local defaults 更严格。

## Track 2: Task / Async Runtime

### Covered

- User task flow 可追踪：`routes_user.go` -> `taskservice.Handler.SubmitTask` -> `taskservice.Service.SubmitTask` -> `tasksubmission.Store.SubmitTask`。
- Task submission 在 `apps/api/internal/tasking/tasksubmission/store.go` 中保持 transactional：task insert 和 River enqueue 位于同一个 PostgreSQL transaction。
- River enqueue 和 options 收口在 `apps/api/internal/tasking/taskqueue/river_queue.go`。
- Runtime wiring 位于 `apps/api/internal/runtime/bootstrap/task_wiring.go`；worker execution 位于 `apps/api/internal/runtime/worker/river_worker.go`。
- Failure convergence 和 failover logic 位于 `taskruntime/service.go`、`task_execution_orchestrator.go`、`provider_errors.go`。
- NATS task status fanout 位于 `apps/api/internal/events/nats_jetstream.go` 和 `task_status.go`。
- River/NATS 运行语义记录在 `docs/river-migration.md`。
- Tests 覆盖 submission rollback、River enqueue options、worker final failure/panic/no-capacity、failover classification、task status publishing，以及 env-gated NATS smoke path。

### Gap

- 默认 test 没有证明真实 River `InsertTx` success chain。
- 未识别到默认 submit -> River -> worker integration test。
- NATS smoke 依赖 `API_TEST_NATS_URL`，默认本地 Baseline 不能证明 JetStream fanout。
- `defaultTaskRetryLimit` 与 River `MaxAttempts` 的一致性需要小测试或 documented invariant 固化。

### Unknown

- nil event bus / nil JetStream path 是否可能静默丢失 production-significant events。
- post-commit best-effort task event publishing 是否会让 UI state stale，且没有清晰 recovery signal。
- final failure convergence 失败时是否只是 log，以及 operator 该如何处理。

## Track 3: Media / Storage

### Covered

- Upload handlers 位于 `apps/api/internal/runtime/httpapi/upload_handlers.go`。
- Upload serving 和 authorization 位于 `apps/api/internal/runtime/httpapi/upload_static_handler.go` 与 `apps/api/internal/media/storage`。
- Media/storage unit coverage 包括 upload resource parsing、upload prefixes、public URL resolution、profile save/apply/delete、object delete、profile avatar cleanup、user upload cleanup、result key cleanup。
- Image validation 拆分在 `apps/api/internal/runtime/httpapi/upload_validation.go` 和 `apps/api/internal/media/imageformat`。
- Media references 和 storage profiles 已在 database schema/migrations 中表示。

### Gap

- 默认 test 没有证明真实 S3/R2-compatible storage 行为。
- Upload restore 和 object reference integrity 尚未被 end-to-end 证明。

### Unknown

- Orphan media cleanup strategy 和 operational cadence。
- restored storage profiles 和 object credentials 是否会在 restore 后 rotate 并 smoke-tested。

## Track 4: Deployment / Ops

### Covered

- `scripts/deploy-stack.sh` 覆盖 env validation、backup、migrations、rollback helpers、Nginx rendering。
- `DEPLOYMENT.md` 和 `docs/data-protection-runbook.md` 提供生产部署与数据保护 runbook。
- `.github/workflows/{docker-images,release,ops}.yml` 提供 ops/release 相关 workflow surface。

### Gap

- 本轮没有执行 restore drill 或 backup decrypt drill。
- 本轮没有执行 migration-after-rollback drill。
- deployment smoke 没有覆盖真实 object storage、NATS JetStream 和 restore 后 credential rotation。

### Unknown

- Cloudflare WAF/cache/rate-limit 行为。
- production Nginx merge behavior after template rendering。
- backup restore 后的 operator checklist 是否足够防止 secret artifact 泄露。

## Track 5: Frontend State Integrity

### Covered

- API error/state boundary 收口在 `apps/web/src/shared/api/client.ts` 的 `apiRequest`、`createApiError`、`handleAuthFailure`；Orval requests 通过 `apps/web/src/shared/api/orvalMutator.ts`。
- User session state 收口在 `apps/web/src/app/stores/user.ts`；tests 覆盖 logout cache clearing、identity-change cache clearing、same-identity refresh cache retention、fresh session fail-closed。
- Shared async action lifecycle 收口在 `apps/web/src/shared/composables/useAsyncAction.ts`；tests 覆盖 success、handled errors、unhandled error cleanup。
- Canvas domain language 和 state concepts 记录在 `apps/web/src/features/canvas/CONTEXT.md`。
- Board session save flow 收口在 `apps/web/src/features/canvas/board-session/useCanvasBoardSession.ts`，并通过 repository / upload lifecycle seams 协调。
- Board session tests 覆盖 stabilize -> save -> preview -> sync -> recycle sequencing、missing board guard、missing user identity guard、preview waiting、board id normalization。
- Autosave 收口在 `apps/web/src/features/canvas/composables/useFlowAutosave.ts`；tests 覆盖 gated autosave 和 later flush。
- Canvas graph mutation 通过 `apps/web/src/features/canvas/composables/useFlowTransactions.ts` 收口；tests 覆盖 no-op mutation、history、dirty marking、snapshot refresh。
- Board persistence 在 `useFlowBoardPersistence.ts` 中有 load/save sequence guards、current-board checks、full-graph snapshot saving、stale load handling。
- Board persistence tests 覆盖 board changes during save/load、full graph saving without dropping hidden nodes/edges、graph replacement semantics。
- Board sync 在 `apps/web/src/features/canvas-board/catalog/boardSync.ts` 中处理 local-newer preservation、local-only boards、conflict-target rollback、hydrate failure without clearing local boards。
- Canvas board state persistence 使用 `valibot` schema validation；schema tests reject unknown fields 并 preserve pending task metadata。
- Task result waiting 在 `apps/web/src/shared/api/taskEvents.ts` 中支持 SSE with polling fallback；tests 覆盖 missing `EventSource`、SSE error fallback、backend close fallback、cancelled tasks。
- Pending image result lifecycle 收口在 `apps/web/src/features/canvas/execution/task-runner/useFlowImageDirectActionRunner.ts`；tests 覆盖 single-image pending node lifecycle 和 pending task id autosave。
- Admin task monitor 在 `apps/web/src/features/admin/task-monitor/useAdminTaskRuntimeSnapshot.ts` 有 stale-stream fallback；tests 覆盖 SSE degraded fallback、REST error exposure、aggregate counts。
- Admin mutation invalidation 在 `adminUserCommandsInvalidation.spec.ts` 和 `useStorageProfilesResource.spec.ts` 中有直接测试。

### Gap

- `taskEvents.spec.ts` 还没有直接证明 SSE completed-event happy path。
- `taskEvents.spec.ts` 应 pin malformed event payload 和 direct failed-event messaging。
- `boardSessionUploadLifecycle.spec.ts` 只覆盖 stable URL normalization；需要补 data URL/remote URL upload、recycle failure、cross-board reference protection。
- Pending image result coverage 应从 reuse/placement/runtime layout 扩到 update、remove、multi-output、reattach paths。
- Generated API endpoint imports 集中在 feature resource/command layers 和 `apps/web/src/app/stores/user.ts`；总体符合约束，但 app-layer exception 需要明确意图。
- `apps/web/eslint.config.js` 没有 enforce `no-restricted-imports` 或等价 import-boundary rule。
- Mock smoke scripts 存在于 `apps/web/scripts/{user-route-smoke.ts,admin-mock-smoke.ts}`，但本轮未运行 browser smoke。
- `createBoardSessionRepository.save` 先 local write 再 server sync；local-write success / remote-sync failure 的 recovery contract 需要测试或用户可见处理说明。

### Unknown

- Multi-tab board editing 行为未被证明：本轮未发现 `storage` event 或 `BroadcastChannel` coordination。
- timestamp-based local/remote merge 在 clock skew 或 concurrent tabs 下是否足够安全。
- localStorage quota fallback 对大 board、多 preview snapshot 的保真度。

## Track 6: Architecture / Maintainability

### Covered

- System context entrypoint 位于 `CONTEXT-MAP.md`。
- Backend context boundaries 和 dependency direction 记录在 `apps/api/CONTEXT.md`。
- Cross-module constraints 和 generated-code ownership 记录在 `docs/architecture-constraints.md`。
- Frontend generated API boundary 有 `apps/web/orval.config.ts`、`apps/web/scripts/check-api-generated-drift.ts`、`apps/web/package.json` 的 `api:generate` / `api:check-generated` 支撑。
- Backend package layout 符合 documented contexts：`runtime`、`platform`、`tasking`、`identity`、`accountpool`、`boards`、`media`、`workflow`、`prompts`。
- Targeted import scan 未发现 `internal/platform`、`internal/db`、`internal/events` import business contexts。
- Frontend `shared` 未在 targeted scan 中出现 feature imports。
- Route pages 未在 targeted scan 中直接 import `shared/api/generated-query`。
- Generated code boundaries 明确：Orval output 位于 `apps/web/src/shared/api/generated-query`，sqlc output 位于 `apps/api/internal/db/sqlc`。
- Orval 和 sqlc generated drift checks 均通过。
- Canvas runtime 拆成 persistence/autosave/history/transactions 与 workspace/runtime submodules，并有 focused specs。
- Image editor runtime 有 `imageProcessingBackends.ts`、`ImageProcessingRuntime.ts`、`imageProcessingScheduler.ts`、worker/main-thread backends 等 seams 和对应 specs。

### Gap

- Architecture constraints 主要依赖 docs 和人工 scan，没有 dedicated import-boundary lint。
- `docs/adr/` 尚未建立，虽然 `CONTEXT-MAP.md` 建议 system-wide ADRs 放在那里。
- Frontend context docs 目前只有 Canvas；Admin、User/Auth、Canvas Board 没有等价 context map。
- 维护热点集中在较大的 orchestration modules，例如 `usePendingImageResultNodes.ts`、`useCanvasViewportRuntime.ts`、`useAdminUserCommands.ts`、`useUsageStatisticsCharts.ts`，大型功能追加前需要 deep audit。
- `apps/web/src/app/stores/user.ts` 直接 import generated-query APIs；需要明确为 bootstrap exception，或移到 `shared/api` / `shared/auth` adapter。
- 当前没有自动报告 fan-in、large modules、dependency cycles、generated-code boundary drift。

### Unknown

- TypeScript import cycles 是否存在。
- 大型 Go handlers 或 Vue composables 是否存在需要优先拆分的 complexity hotspots。
- codebase-memory graph coverage 是否足够作为唯一 architecture audit source；本轮必须 fallback 到 targeted file scans。
- mock smoke、benchmark、generated drift checks 是否在 CI 中强制执行，而不只是 local scripts。

## Candidate Deep Audit Queue

| Priority | Track | Question | Status | Evidence |
| --- | --- | --- | --- | --- |
| P1 | Auth / Permission / Ownership | 所有 admin routes 是否被 guard，且 `AdminMiddleware` 是否有直接测试？ | Gap | `apps/api/internal/identity/auth/middleware.go`, `routes_admin.go` |
| P1 | Auth / Permission / Ownership | Auth Service 是否 deterministic reject unsafe production secrets/origins？ | Gap | `apps/auth/src/env.ts`, auth Baseline warnings |
| P1 | Task / Async Runtime | 默认 tests 是否能证明 submit -> River insert -> worker convergence？ | Gap | `tasksubmission/store.go`, `taskqueue/river_queue.go`, `runtime/worker/river_worker.go` |
| P1 | Task / Async Runtime | final failure convergence 失败时系统和 operator 应如何处理？ | Unknown | `taskruntime.Service.ConvergeTaskAfterRiverFinalFailure`, worker error handling |
| P1 | Frontend State Integrity | local write 成功但 server sync 失败时 board save 是否能清晰恢复？ | Gap | `boardSessionRepository.save`, `boardSync.syncCanvasBoardToServer` |
| P1 | Deployment / Ops | backup restore plus credential rotation 是否做过 drill？ | Unknown | `scripts/deploy-stack.sh`, `docs/data-protection-runbook.md` |
| P2 | Frontend State Integrity | SSE happy path、malformed payload、failed-event message 是否有测试 pin 住？ | Gap | `apps/web/src/shared/api/taskEvents.ts` |
| P2 | Frontend State Integrity | Multi-tab board editing 是否需要 conflict detection？ | Unknown | `canvasBoardState.ts`, `boardSync.ts` |
| P2 | Media / Storage | real S3/R2 upload/delete/restore 是否有 env-gated smoke？ | Gap | `apps/api/internal/media/storage`, upload handlers |
| P2 | Architecture / Maintainability | 是否应添加 import-boundary lint 来 enforce `docs/architecture-constraints.md`？ | Gap | Manual scans only |
| P2 | Architecture / Maintainability | 大型 frontend orchestration modules 是否需要 targeted deep audit？ | Gap | `usePendingImageResultNodes.ts`, `useCanvasViewportRuntime.ts`, `useUsageStatisticsCharts.ts` |
| P2 | Architecture / Maintainability | 是否应初始化 `docs/adr/` 存放 system-wide ADRs？ | Gap | `CONTEXT-MAP.md` recommends `docs/adr/` |
| P2 | Architecture / Maintainability | `app/stores/user.ts` 是否应停止直接 import generated-query？ | Unknown | `apps/web/src/app/stores/user.ts` |

## 下一步执行计划

1. 把 P1 gaps 提升成 GitHub Issues，并使用本文的 Finding Format。
2. 优先做 Auth route coverage 和 Task/River convergence 的 Deep Audit。
3. 增加一个 production-like ops drill issue，覆盖 backup restore、storage credentials、smoke verification。
4. 保持 fix small：先补 missing tests/contracts，再考虑 module shape refactor。
