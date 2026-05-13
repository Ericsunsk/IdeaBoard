# 架构约束

## 仓库布局

- 源码应用统一放在 `apps/` 下。
- 前端位于 `apps/web`。
- 后端位于 `apps/api`。
- 部署脚本、docker 目录和仓库级文档保留在仓库根目录。


## 前端边界

- `apps/web/src/app` 负责启动装配、router 和 store wiring。
- `apps/web/src/pages` 只负责路由级页面组装。
- `apps/web/src/features` 负责业务逻辑和 feature 内部 UI。
- `apps/web/src/features/user` 负责用户侧 layout、路由壳层与跨用户页的状态提示。
- `apps/web/src/features/workspace/components/nodes` 只保留节点 Vue 组件；节点配置、工具、样式和 composable 分别放入该目录下的 `config/`、`utils/`、`styles/`、`composables/`。
- `apps/web/src/shared` 负责复用 API helper、auth、types、utils 和 assets。
- `apps/web/src/shared/api/generated-query` 是 Orval 生成代码，不手动编辑。
- 手写 API 补充层用于聚合、补齐未生成接口或适配业务返回类型；新增文件优先沿用现有 `shared/api/*.ts` 扁平 wrapper，只有明确需要分组时再放入子目录。

> [!IMPORTANT]
> `shared` 不得依赖任何具体 feature。


## 后端边界

- `apps/api/cmd` 保持轻量，只负责启动应用。
- `apps/api/internal/platform` 负责 config 和基础设施适配层。
- `apps/api/internal/runtime` 负责 HTTP 注册、bootstrap 和 worker。
- 各领域包在同一包内收口自己的 handler、service、repository 和 model。
- 对外 contract 以当前 HTTP 路由、JSON 字段名、环境变量语义和数据库结构为准；不提供已下线能力或历史字段别名的适配层。


## 命名规范

| 层 | 规范 | 示例 |
|----|------|------|
| 目录 | `kebab-case` | `image-processing/` |
| Vue SFC | `PascalCase` | `WorkspaceScreen.vue` |
| TS helper / composable | `camelCase.ts` | `useChannelAccounts.ts` |
| Go 文件 | `snake_case.go` | `task_runtime_service.go` |

Go package 目录例外：保持 Go 生态习惯，使用小写无分隔符命名，例如 `autherrors/`、`imageproc/`；不要为了匹配前端目录风格改成 `kebab-case`。


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
