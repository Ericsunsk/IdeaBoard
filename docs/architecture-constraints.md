# 架构约束

本文记录跨模块边界和 ownership 规则。更具体的执行约定见 `AGENTS.md`；后端 context map 见 `apps/api/CONTEXT.md`；多 context 入口见 `CONTEXT-MAP.md`。

## 仓库布局

- 应用源码统一放在 `apps/` 下：`apps/web`、`apps/api`、`apps/auth`。
- 部署脚本、Docker 配置和仓库级文档保留在仓库根目录或 `docs/`。
- `docs/` 下普通文档使用 `kebab-case.md`；根目录保留传统大写文档名。

## 前端边界

- `apps/web/src/app` 负责启动装配、router 和 store wiring。
- 路由级页面放在对应 `apps/web/src/features/*/pages`，或对应 vertical context 的 page component。
- `apps/web/src/features` 承载业务逻辑、route pages、feature 内部 UI 和 feature-local adapters。
- `apps/web/src/shared` 只放跨 feature 复用的 API helpers、auth、types、utils 和 assets。
- `shared` 不得依赖任何具体 feature。

## Canvas 与 Admin 约束

- Canvas 保持轻量 context map，不按默认 DDD 分成 `domain/application/infrastructure`。
- Canvas 子模块优先通过各自 `index.ts` public seam 依赖；同一子模块内部可使用相对 deep import。
- `features/canvas-board/domain` 继续作为共享 snapshot schema/core module。
- Admin vertical contexts 拥有自己的 resource、view model、command sequencing 和 side effect handling。
- Admin route pages 必须通过对应 vertical context/resource 访问 generated API，不直接依赖 Orval 生成入口。

## Generated API 边界

- `apps/web/src/shared/api/generated-query` 是 Orval generated code，不手动编辑。
- Vue route pages 和 components 不直接依赖 `shared/api/generated-query`。
- Generated-query 依赖应收口在 feature context、shared composable，或明确的 API adapter 内。
- 手写 API 补充层用于聚合、补齐未生成接口或适配业务返回类型；业务特定 adapter 优先放在对应 feature context。

## 前后端 Ownership 映射

| Frontend context | Generated endpoint surface | Backend context | Ownership / contract 边界 |
|----|----|----|----|
| `features/admin/users` | `admin-users` | `internal/identity/users` | Identity 拥有 admin user list、role/status mutation、session revoke 和 impersonation 语义；前端负责 view model、command sequencing 和 cache invalidation。 |
| `features/admin/provider-config` | `admin-accounts`, `admin-channels` | `internal/accountpool/accounts`, `internal/accountpool/channels` | AccountPool 拥有 channel/account/provider payload、model detection 和 scheduling visibility；前端负责 editor state、payload defaults、model options 和 cards。 |
| `features/admin/prompt-config` | `admin-feature-prompts` | `internal/prompts`, `internal/workflow` | Workflow/Prompts 拥有 feature slug validation、prompt template storage、variables 和 revisions；前端负责 resource access、save command 和 workflow registry presentation。 |
| `features/admin/storage` | `admin-storage` | `internal/media/storage` | Media 拥有 object storage provider/profile、secret masking、bucket test/apply 和 upload object semantics；前端负责 profiles resource、editor workflow 和 provider-specific form helpers。 |
| `features/admin/task-monitor` | `admin-tasks`, `admin-monitor` | `internal/tasking/taskservice`, `internal/tasking/taskstats` | Tasking 拥有 task list/detail、runtime monitor、stream/poll fallback contract 和 result detail；前端负责 list params、snapshot merge、charts 和 result image presentation。 |
| `features/admin/statistics` | `admin-statistics` | `internal/tasking/taskstats` | Tasking 拥有 statistics aggregation、backfill status 和 range contract；前端负责 statistics resource、time-range selection 和 chart view models。 |

Frontend context 名称不要求与 backend context 同名；硬约束是 generated endpoint 和 backend payload ownership 不泄漏到 route component 或无关 `shared` module。

## 后端边界

- `apps/api/cmd` 保持轻量，只负责启动应用。
- `internal/runtime` 负责 HTTP 注册、bootstrap、worker 和 embedded frontend dist。
- `internal/platform` 负责 config 和基础设施适配层，不 import 业务 context。
- 业务 context、依赖方向和无 legacy 迁移策略以 `apps/api/CONTEXT.md` 为准。
- 对外 contract 以当前 HTTP 路由、JSON 字段名、环境变量语义和数据库结构为准。

## 命名规范

| 层 | 规范 | 示例 |
|----|------|------|
| 目录 | `kebab-case` | `image-processing/` |
| Vue SFC | `PascalCase` | `CanvasPage.vue` |
| TS helper / composable | `camelCase.ts` | `useCanvasImageTools.ts` |
| Go 文件 | `snake_case.go` | `task_runtime_service.go` |

Go package 目录保持小写无分隔符，例如 `autherrors/`、`webdist/`。

## 统计数据模型

- `TaskStatEvent` 按 `task_id` 去重，保证同一任务只计数一次。
- `TaskDailyStat` 按 `(stat_date, model_name, user_key)` upsert 聚合。
- `TaskDailyStatBackfill` 标记某日期已完成历史扫描，避免重复回填。
- 前端统计 API 返回 `backfill_status`，由前端决定是否触发补录。
