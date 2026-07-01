# 架构约束

## 仓库布局

- 源码应用统一放在 `apps/` 下。
- 前端位于 `apps/web`。
- 后端位于 `apps/api`。
- 部署脚本、docker 目录和仓库级文档保留在仓库根目录。


## 前端边界

- `apps/web/src/app` 负责启动装配、router 和 store wiring。
- 路由级页面放在对应 `apps/web/src/features/*/pages` 下。
- `apps/web/src/features` 负责业务逻辑、路由页和 feature 内部 UI。
- `apps/web/src/features/user` 负责用户侧 layout、路由壳层与跨用户页的状态提示。
- `apps/web/src/features/canvas/components/nodes` 只保留节点 Vue 组件及组件私有实现；跨节点配置、工具和服务放在 `features/canvas/config`、`utils`、`services` 或明确业务 context 下。
- `apps/web/src/features/canvas/image-tools` 收拢 canvas 图像工具入口；页面和节点组件只消费 image tool chrome/stage/action binding，不直接串联 image-editor 与 mask-editor 的内部状态。
- `apps/web/src/features/canvas/execution` 收拢任务执行 context；graph/source planning、backend request adapter、task runner、pending result node materialization 分别放在 `planning`、`commands`、`task-runner`、`result-nodes` 下。
- `apps/web/src/features/canvas/board-session` 收拢 canvas board load/save/exists/preview/upload lifecycle；`features/canvas-board/domain` 继续作为共享 snapshot schema/core module。
- Canvas 保持轻量 context map，不按默认 DDD 分成 `domain/application/infrastructure`。只有能缩小 module interface、提升 locality 的子域才独立成 deep module；`components/**`、route page、viewport/pointer/RAF utilities 保持 UI/runtime 辅助角色。
- Canvas 子模块之间优先通过各自 `index.ts` public seam 依赖；同一子模块内部可以使用相对 deep import。新增跨子模块调用前，先判断是否应收口到既有 interface。
- `apps/web/src/features/admin/users` 收拢 admin users list、view model、user commands 和 impersonation/session side effects。
- `apps/web/src/features/admin/provider-config` 收拢 channel/account/provider configuration；channels route page 不直接处理 provider payload mapping。
- `apps/web/src/features/admin/prompt-config` 收拢 feature prompt resource、query key、save command 和 generated prompt types；prompt-config route page 不直接依赖 Orval 生成入口。
- `apps/web/src/features/admin/statistics` 收拢 statistics resource 和 generated statistics contract；statistics route page、charts 和 data composable 不直接依赖 Orval 生成入口。
- `apps/web/src/features/admin/storage` 收拢 storage resource 与 editor workflow；generated-query 只在 resource module 内使用。
- `apps/web/src/features/admin/task-monitor` 收拢 task list、runtime snapshot、stream/polling fallback 和 result detail。
- Admin vertical context 可以自带 route component，例如 `features/admin/users/AdminUsersPage.vue`；也可以保留 `features/admin/pages/*` 下的 route component 作为薄壳或组合层。两种形状都必须通过对应 vertical context/resource 访问 generated API、payload mapping 和 side effect command。
- `apps/web/src/shared` 负责复用 API helper、auth、types、utils 和 assets。
- `apps/web/src/shared/api/generated-query` 是 Orval 生成代码，不手动编辑。
- 手写 API 补充层用于聚合、补齐未生成接口或适配业务返回类型；业务特定 API adapter 优先放在对应 feature context 内，只有跨 feature 共享时才放入 `shared/api`。
- 路由页和 Vue 组件不得直接依赖 `shared/api/generated-query`；generated-query 依赖应收口在 feature context、shared composable 或明确的 API adapter 内。
- 本轮前端 context 重排不保留旧兼容 alias 或旧路径 re-export；移动后直接更新 import 并删除旧入口。

> [!IMPORTANT]
> `shared` 不得依赖任何具体 feature。

## 前后端 Ownership 映射

| Frontend context | Generated endpoint surface | Backend context | Ownership / contract 边界 |
|----|----|----|----|
| `features/admin/users` | `admin-users` | `internal/identity/users` | Identity 拥有 admin user list、role/status mutation、session revoke 和 impersonation 语义；前端 context 负责 view model、command sequencing 和 cache invalidation。 |
| `features/admin/provider-config` | `admin-accounts`, `admin-channels` | `internal/accountpool/accounts`, `internal/accountpool/channels` | AccountPool 拥有 channel/account/provider payload、model detection 和 scheduling visibility；前端 context 负责 editor state、payload defaults、model options 和 cards。 |
| `features/admin/prompt-config` | `admin-feature-prompts` | `internal/prompts`, `internal/workflow` | Workflow/Prompts 拥有 feature slug validation、prompt template storage、variables 和 revisions；前端 context 负责 resource access、save command 和 workflow registry presentation。 |
| `features/admin/storage` | `admin-storage` | `internal/media/storage` | Media 拥有 object storage provider/profile、secret masking、bucket test/apply 和 upload object semantics；前端 context 负责 profiles resource、editor workflow 和 provider-specific form helpers。 |
| `features/admin/task-monitor` | `admin-tasks`, `admin-monitor` | `internal/tasking/taskservice`, `internal/tasking/taskstats` | Tasking 拥有 task list/detail、runtime monitor、stream/poll fallback contract 和 result detail；前端 context 负责 list params、snapshot merge、charts 和 result image presentation。 |
| `features/admin/statistics` | `admin-statistics` | `internal/tasking/taskstats` | Tasking 拥有 statistics aggregation、backfill status 和 range contract；前端 context 负责 statistics resource、time-range selection 和 chart view models。 |

Frontend context 名称不要求与 backend context 同名；唯一硬约束是 generated endpoint 和 backend payload ownership 不泄漏到 route component 或无关 `shared` module。


## 后端边界

- `apps/api/cmd` 保持轻量，只负责启动应用。
- `apps/api/internal/platform` 负责 config 和基础设施适配层。
- `apps/api/internal/runtime` 负责 HTTP 注册、bootstrap、worker 和 embedded frontend dist。
- `apps/api/internal/tasking` 收拢 Tasking bounded context；内部保留 `tasks` 模型、`taskservice` 应用服务、`taskstore` 持久化、`tasksubmission` 事务提交、`taskstats` 统计监控、`taskmedia` 媒体引用、`taskqueue` 队列接口和 `taskruntime` 执行状态服务等小包。
- `apps/api/internal/identity` 收拢 Auth middleware/client、auth errors、users 和 admin/user 管理。
- `apps/api/internal/media` 收拢 storage、assets、image format validation 和 media references。
- `apps/api/internal/accountpool` 收拢 accounts、channels、account load 和 task scheduling visibility。
- `apps/api/internal/runtime/worker` 仍是 runtime execution adapter，可依赖 `tasking` 下的任务队列和运行时服务；业务领域包不得依赖 `internal/runtime` 下的 worker 实现细节。
- 后端 context、依赖方向和无 legacy 迁移策略记录在 `docs/backend-context-map.md`。
- 其他领域包在同一包内收口自己的 handler、service、repository 和 model。
- 对外 contract 以当前 HTTP 路由、JSON 字段名、环境变量语义和数据库结构为准；字段命名与层级应在 OpenAPI、前端调用和后端 validation 中保持一致。


## 命名规范

| 层 | 规范 | 示例 |
|----|------|------|
| 目录 | `kebab-case` | `image-processing/` |
| Vue SFC | `PascalCase` | `CanvasPage.vue` |
| TS helper / composable | `camelCase.ts` | `useCanvasImageTools.ts` |
| Go 文件 | `snake_case.go` | `task_runtime_service.go` |

Go package 目录例外：保持 Go 生态习惯，使用小写无分隔符命名，例如 `autherrors/`、`webdist/`；不要为了匹配前端目录风格改成 `kebab-case`。

仓库根目录文档保留传统大写文件名，例如 `README.md`、`DEVELOPMENT.md`、`DEPLOYMENT.md`；`docs/` 下普通文档使用 `kebab-case.md`。


## 验证要求

- 前端：`cd apps/web && npm run build`
- 后端：`cd apps/api && go test ./... && go build ./...`
- Auth 服务：`cd apps/auth && npm run build`
- 生产、安全或发布相关改动：对 `apps/web` 和 `apps/auth` 运行 `npm audit --omit=dev`。
- 跨层改动默认同时验证前后端与 Auth 服务。


## 统计数据模型

- `TaskStatEvent` 按 `task_id` 去重，保证同一任务只计数一次。
- `TaskDailyStat` 按 `(stat_date, model_name, user_key)` upsert 聚合。
- `TaskDailyStatBackfill` 标记某日期已完成历史扫描，避免重复回填。
- 回填逻辑：首次查询缺失日期时扫描 tasks 表终态记录，逐条 upsert。
- 前端统计 API 返回 `backfill_status`，由前端决定是否触发补录。
