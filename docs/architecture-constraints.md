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
- `apps/web/src/features/workspace/components/nodes` 只保留节点 Vue 组件及组件私有实现；跨节点配置、工具和服务放在 `features/workspace/config`、`utils` 或 `services`。
- `apps/web/src/shared` 负责复用 API helper、auth、types、utils 和 assets。
- `apps/web/src/shared/api/generated-query` 是 Orval 生成代码，不手动编辑。
- 手写 API 补充层用于聚合、补齐未生成接口或适配业务返回类型；新增文件优先沿用现有 `shared/api/*.ts` 扁平 wrapper，只有明确需要分组时再放入子目录。

> [!IMPORTANT]
> `shared` 不得依赖任何具体 feature。


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
| Vue SFC | `PascalCase` | `WorkspacePage.vue` |
| TS helper / composable | `camelCase.ts` | `useChannelAccounts.ts` |
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
