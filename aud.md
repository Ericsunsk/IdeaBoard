# IdeaBoard 项目审计报告

**日期**: 2026-06-17  
**方式**: `/subagents` 并行审计 + 主线程复核 + 后续 targeted 修复  
**范围**: `apps/web`、`apps/api`、`apps/auth`、`docker/`、`.github/workflows/`、根文档  
**模式**: Daily mode；先审计，再小步修复高/中优先级问题  

## 结论概览

| 严重度 | 数量 | 说明 |
| --- | ---: | --- |
| High | 1 | 用户密码更新后的 session 状态与 UI 安全预期不一致 |
| Medium | 6 | 任务删除一致性、前端错误反馈、admin 监控、部署/验证文档漂移 |
| Low | 5 | sqlc 边界、测试缺口、设计 token、generated API 边界、工具文档小漂移 |

最值得优先处理的是 `H-1` 和 `M-1`：前者影响账号安全语义，后者影响任务/文件删除的一致性。其余 Medium 多数是可小步修复的可靠性和工程化问题。

## 当前修复状态

| 项 | 状态 | 落点 |
| --- | --- | --- |
| H-1 | 已修复 | `ProfileCenterScreen.vue` 密码更新后调用 `userStore.logout()`，成功后 `router.replace('/user/login')` |
| M-1 / L-2 | 已修复 | `DeleteUserTask` 先取消 + 删除 task row，output 对象清理改为 best-effort；新增 cleanup failure test |
| M-2 | 已修复 | `hydrateCanvasBoardsFromServer()` 返回 `{ ok, error }`，`BoardsScreen` 初次加载失败时提示 toast |
| M-3 | 已处理 | 移除不可达 degraded anomaly 规则；如需要 header 级 SSE degraded 告警，后续接入 `useTaskMonitorStream().status` |
| M-4 / M-5 / M-6 / L-5 | 已修复 | README / docker README / AGENTS 验证与 smoke 脚本文档已对齐 |
| L-1 / L-3 / L-4 | 未处理 | 属于结构性改善，建议单独排期 |

## High

### H-1. 密码更新后没有真正结束 Better Auth session（已修复）

- 位置: `apps/web/src/features/profile/screens/ProfileCenterScreen.vue:577`、`apps/web/src/app/router/index.ts:90`、`apps/web/src/app/stores/user.ts:197`
- 问题: 密码更新成功后前端只清空本地 store 并跳转 `/user/login`；但 `/user/login` 是 guest route，route guard 会重新 `ensureSession()`。如果 Better Auth 当前 cookie/session 仍有效，用户可能被立即重定向回 `/user/home`。
- 风险: UI 传达“请重新登录”，但真实 session 仍可能可用，安全预期和系统行为不一致。
- 建议:
  - 如果产品语义是强制重登：`updatePassword` 成功后调用 `authClient.signOut()`，或让 Auth/API 端 revoke 当前 session，再跳登录页。
  - 如果允许当前 session 保留：调整 UI copy，不要提示必须重新登录。
  - 补 password update + route guard regression test。
- 本轮处理: `ProfileCenterScreen.vue` 在密码更新成功后调用 `userStore.logout()`，只有 sign out 成功才提示并跳转登录页；sign out 失败会展示“请手动退出后重新登录”。

## Medium

### M-1. 用户删除任务链路存在半删除状态（已修复）

- 位置: `apps/api/internal/taskservice/service.go:303-319`
- 问题: `DeleteUserTask` 依次执行 `MarkCancelled`、`DeleteTaskOutputsByUserTask`、`DeleteByTaskIDAndUserID`，没有 shared DB transaction 或补偿逻辑。
- 风险: 如果 output 清理成功但最终 task row 删除失败，可能出现任务仍在、结果文件或输出记录已消失的半删除状态。
- 建议:
  - 把数据库状态变更放进同一个事务。
  - 对象存储清理改成可重试 async job，或至少在最终删除成功后再做不可回滚清理。
  - 给 partial failure 增加 failure-injection test。
- 本轮处理: `DeleteUserTask` 先 `MarkCancelled` 再删除 task row，随后按对象前缀 best-effort 清理 output 文件；清理失败只记录 warning，避免文件清理成功但 DB 删除失败的半删除状态。

### M-2. 画板同步失败只写 console，用户无感（已修复）

- 位置: `apps/web/src/features/boards/lib/boardSync.ts:174`、`apps/web/src/features/boards/screens/BoardsScreen.vue:701`
- 问题: `hydrateCanvasBoardsFromServer()` 捕获加载失败后只 `console.warn`；调用方在 `finally` 结束 loading，没有 toast、retry 或错误状态。
- 风险: API/网络失败时，用户看到空列表或旧 localStorage 数据，却不知道远端同步失败。
- 建议: 让 hydrate 返回 `{ ok, error }` 或 rethrow，由 `BoardsScreen` 显示非阻塞错误与“重试”；补 initial hydrate failure 测试。
- 本轮处理: `hydrateCanvasBoardsFromServer()` 失败时返回 `{ ok: false, error }`；`BoardsScreen` 初次加载会展示“画布同步失败，请稍后重试”，并保留本地画布。

### M-3. Admin anomaly radar 的 degraded 规则不可达（已处理）

- 位置: `apps/web/src/features/admin/shared/composables/useAdminAnomalyRadar.ts:74`、`apps/web/src/features/admin/shared/lib/adminAnomalyRules.ts:59`
- 问题: 规则支持 `isMonitorDegraded`，但调用处永远传 `false`。
- 风险: 实时监控流 degraded/fallback 时，admin header 仍可能显示正常。
- 建议: 接入 `useTaskMonitorStream().status` 或移除不可达规则；补 degraded case unit test。
- 本轮处理: 移除了 `isMonitorDegraded` 输入和不可达的 `system.monitor-stream-degraded` 规则，避免 dead branch 造成误导。若产品仍需要全局 header 展示 SSE degraded，需要再把 `useTaskMonitorStream().status` 提升到 anomaly radar 可消费的状态源。

### M-4. README 的 VPS bootstrap 地址硬编码仓库 owner/name（已修复）

- 位置: `README.md:343-347`、`DEPLOYMENT.md:208-223`
- 问题: bootstrap 命令硬编码 `Ericsunsk/IdeaBoard`。
- 风险: 仓库迁移、改名或 fork 后，文档部署入口会拉错脚本或直接失败。
- 建议: 改成可配置的 `GITHUB_REPOSITORY` 示例，或明确标注这是当前仓库专用命令。
- 本轮处理: README 已提示 fork 或自建仓库需要替换 `Ericsunsk/IdeaBoard`。

### M-5. README 验证建议缺少自动化验证矩阵（已修复）

- 位置: `README.md:425-434`
- 问题: 上线前验证只列 health curl 和人工点检，未串起仓库已有自动化检查。
- 风险: 协作者容易把“服务能打开”误当成“已验证完成”，漏跑 build/test/lint/smoke。
- 建议: 增加分层验证矩阵，例如：
  - Web: `npm run lint`、`npm run lint:style`、`npm run test:run`、`npm run build`
  - API: `go test ./...`、`go test -race ./...`、`go tool sqlc generate && git diff --exit-code -- internal/db/sqlc`
  - Auth: `npm run build`、`npm test`
  - Production: `ideaboard smoke`
- 本轮处理: README 已增加 Web/API/Auth/Production/手工分层验证矩阵，并使用当前存在的 smoke script 名称。

### M-6. docker 本地依赖手册把本地启动和生产提示混在一起（已修复）

- 位置: `docker/README.md:11-18`
- 问题: `compose.local.yml` 的快速启动注释写“生产 stack 至少修改...”，但命令实际只启动本地 PostgreSQL + NATS。
- 风险: 新接手的人会被引导去改生产域名/密钥，入口认知变复杂。
- 建议: 拆成“本地依赖启动”和“生产 Stack”两个小节；本地段只强调 `POSTGRES_PASSWORD` 等本地必需项。
- 本轮处理: `docker/README.md` 已明确 `compose.local.yml` 只启动本地 PostgreSQL + NATS，不包含应用容器。

## Low

### L-1. SubmitTask 手写 raw SQL 绕开 sqlc 边界

- 位置: `apps/api/internal/tasksubmission/store.go:34-118`
- 问题: `insertTask` 手写 `INSERT INTO tasks`，没有复用 sqlc query。
- 风险: `tasks` schema 变化时，编译期不一定能暴露 drift，提交路径更容易 runtime break。
- 建议: 将创建任务 SQL 收敛到 `db/queries/tasks.sql` 并生成 `sqlc` 方法；若短期不改，至少给 schema drift 增加 targeted test。

### L-2. DeleteUserTask 缺少 partial failure 测试（已修复）

- 位置: `apps/api/internal/taskservice/service_test.go:523-606`
- 问题: 当前测试主要覆盖删除成功路径，未覆盖 output 清理成功但最终 task delete 失败。
- 风险: `M-1` 的半删除状态未来回归时，测试不容易拦住。
- 建议: 增加 failure-injection test，断言最终删除失败时不会造成不可恢复状态，或进入明确可重试状态。
- 本轮处理: 新增 `TestDeleteUserTaskSucceedsWhenOutputCleanupFails`，覆盖 output cleanup 失败时 task row 仍完成删除的策略。

### L-3. 用户侧页面仍有大量硬编码 color / arbitrary Tailwind value

- 位置: `apps/web/src/features/boards/screens/BoardsScreen.vue:247`、`apps/web/src/features/profile/screens/ProfileCenterScreen.vue:2`
- 问题: 用户侧页面和已 token 化的 admin / board card 体系不一致。
- 风险: 后续 theme/brand 调整容易漂移，移动端 hover/focus/contrast 也更难统一验证。
- 建议: 抽 `user/canvas` shared UI tokens/classes，逐步替换 literal colors；同时确认英文 UI copy 是否为产品刻意选择。

### L-4. 多个页面直接依赖 generated-query

- 位置: `apps/web/src/features/admin/pages/users/AdminUsersPage.vue:373`、`apps/web/src/features/admin/pages/prompt-config/AdminPromptConfigPage.vue:199`、`apps/web/src/features/admin/pages/task-monitor/useTaskMonitor.ts:2`
- 问题: feature page/composable 直接依赖 `generated-query` hook/model，绕过 feature/service adapter 边界。
- 风险: Orval regen 或 API shape 小变动会扩散到页面层，错误处理和 request shape 也更难统一。
- 建议: 收敛到 `shared/api` 或 `features/admin/shared/api` adapter，页面只消费 domain-friendly types。

### L-5. AGENTS 工具说明有脚本名漂移（已修复）

- 位置: `AGENTS.md:43`、`apps/web/package.json:16-18`
- 问题: AGENTS 写 `npm run smoke:user-mock`，实际脚本名是 `smoke:user`。
- 风险: 协作者按文档执行会遇到 missing script。
- 建议: 更新 AGENTS 为 `npm run smoke:user`，或在 `apps/web/package.json` 增加兼容 alias。
- 本轮处理: AGENTS 已对齐 `npm run smoke:admin-mock` 与 `npm run smoke:user`。

## 已验证/检查

- subagent 前端审计已运行并通过：
  - `npm run -s --prefix apps/web lint -- --no-fix`
  - `npm run -s --prefix apps/web lint:style`
  - `npm run -s --prefix apps/web test:run`，27 files / 100 tests passed
  - `npm run -s --prefix apps/web api:check-generated`
  - `git diff --check`
- subagent 后端审计已运行并通过：
  - `go test ./internal/tasksubmission ./internal/runtime/taskqueue ./internal/db/migrations`
  - `go test ./internal/taskservice -run 'TestNormalizeTaskListKeyword|Test.*DeleteUserTask|Test.*GetUserTasks'`
- 主线程只读复核：
  - `git status --short`
  - `rg --files`
  - targeted `sed` / `nl -ba` 查看 README、AGENTS、docker docs、Vite/router/store/API/Auth/任务删除链路。
- 本轮修复后验证已通过：
  - `cd apps/web && npm run lint`
  - `cd apps/web && npm run lint:style`
  - `cd apps/web && npm run test:run`，27 files / 101 tests passed
  - `cd apps/web && npm run build`
  - `cd apps/web && npm run api:check-generated`
  - `cd apps/api && go test ./internal/taskservice ./internal/tasksubmission`
  - `cd apps/api && go test ./...`
  - `cd apps/auth && npm run build`
  - `cd apps/auth && npm test`，12 tests passed
  - `git diff --check`

## 未执行项

- 未跑 `cd apps/api && go test -race ./...`；耗时较高，建议在合并前或 CI 中补跑。
- 未跑 `cd apps/api && go tool sqlc generate && git diff --exit-code -- internal/db/sqlc`；本轮未修改 SQL/schema。
- 未做浏览器 smoke；本轮目标是 repo audit 和 targeted 修复，不是交互验收。

## 建议修复顺序

1. 合并前补 `go test -race ./...` 和 sqlc 一致性检查。
2. 视产品需要决定是否把 SSE degraded 状态接入 admin header anomaly radar。
3. 单独排期处理 `L-1` / `L-3` / `L-4`：raw SQL 边界、用户侧 design token、generated-query adapter 收敛。
