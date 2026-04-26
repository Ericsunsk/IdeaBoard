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
- `apps/web/src/shared` 负责复用 API helper、UI primitive、types、utils 和 assets。
- `shared` 不得依赖任何具体 feature。

## 后端边界

- `apps/api/cmd` 保持轻量，只负责启动应用。
- `apps/api/internal/platform` 负责 config 和基础设施适配层。
- `apps/api/internal/runtime` 负责 HTTP 注册、bootstrap 和 worker。
- 各领域包在同一包内收口自己的 handler、service、repository 和 model。
- 对外 contract 以当前 HTTP 路由、JSON 字段名、环境变量语义和数据库结构为准；不提供已下线能力或历史字段别名的适配层。

## 命名规范

- 目录统一使用英文语义名称和 `kebab-case`。
- Vue SFC 使用 `PascalCase`。
- TypeScript helper 和 composable 使用 `camelCase.ts`。
- Go 文件使用 `snake_case.go`。

## 验证要求

- 前端：`cd apps/web && npm run build`
- 后端：`cd apps/api && go test ./... && go build ./...`
- 跨层改动默认同时验证前后端。
