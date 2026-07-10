# IdeaBoard Developer Handbook

`DEVELOPMENT.md` 服务日常开发：怎么启动、改哪里、跑什么验证。系统设计 contract 见 [DESIGN.md](./DESIGN.md)，线上操作见 [docs/runbooks/](./docs/runbooks/)。

## 本地开发

环境要求：

| 组件 | 建议版本 |
| ---- | -------- |
| Node.js | `22.18+` |
| Go | `1.26.4+` |
| PostgreSQL | `17+` |

启动本地依赖：

```bash
cd docker
cp .env.example .env
# 至少修改 POSTGRES_PASSWORD
docker compose -f compose.local.yml up -d
```

启动 Auth：

```bash
cd apps/auth
cp .env.example .env
npm install
npm run migrate
npm run dev
```

启动 API：

```bash
cd apps/api
cp .env.example .env
go run ./cmd migrate
go run ./cmd
```

启动 Web：

```bash
cd apps/web
npm install
npm run dev
```

初始化管理员：

```bash
cd apps/api
INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin
```

开发代理：

| Path | Target |
| ---- | ------ |
| `/api/auth/**` | `http://localhost:6001` |
| `/api/**` | `http://localhost:6000` |
| `/uploads/**` | `http://localhost:6000` |

## 验证矩阵

日常小改优先跑最小相关检查；跨层 contract 改动同时验证前后端。

| 场景 | 命令 |
| ---- | ---- |
| Web unit/spec | `cd apps/web && npm run test:run` |
| Web test typecheck | `cd apps/web && npm run test:typecheck` |
| Web browser-level spec | `cd apps/web && npm run test:browser` |
| Web build/type | `cd apps/web && npm run build` |
| Web lint | `cd apps/web && npm run lint` |
| Web style lint | `cd apps/web && npm run lint:style` |
| API tests | `cd apps/api && go test ./...` |
| API build | `cd apps/api && go build ./...` |
| sqlc regeneration | `cd apps/api && go tool sqlc generate` |
| Auth build | `cd apps/auth && npm run build` |

修改 `apps/api/internal/db/queries` 或 schema 后：

```bash
cd apps/api
go tool sqlc generate
git diff --exit-code -- internal/db/sqlc
```

## 常用入口

### Canvas workflow

| 入口 | 用途 |
| ---- | ---- |
| [CanvasPage.vue](./apps/web/src/features/canvas/pages/CanvasPage.vue) | Canvas route page |
| [executionContract.ts](./apps/web/src/features/canvas/execution/contracts/executionContract.ts) | 前端 workflow feature contract |
| [useFlowPromptGenerationOrchestrator.ts](./apps/web/src/features/canvas/composables/useFlowPromptGenerationOrchestrator.ts) | Prompt/node execution orchestration |
| [useFlowImageDirectActions.ts](./apps/web/src/features/canvas/execution/runtime/useFlowImageDirectActions.ts) | Image node direct actions |
| [taskEvents.ts](./apps/web/src/shared/api/taskEvents.ts) | Task SSE events |
| [features.go](./apps/api/internal/workflow/features.go) | 后端 canonical feature contract |

### Task runtime

| 入口 | 用途 |
| ---- | ---- |
| [service.go](./apps/api/internal/tasking/taskservice/service.go) | Task service |
| [store.go](./apps/api/internal/tasking/tasksubmission/store.go) | Transactional task submission |
| [river_queue.go](./apps/api/internal/tasking/taskqueue/river_queue.go) | River enqueue adapter |
| [river_worker.go](./apps/api/internal/runtime/worker/river_worker.go) | River worker adapter |
| [task_processor.go](./apps/api/internal/runtime/worker/task_processor.go) | Worker task processing |
| [provider_request.go](./apps/api/internal/runtime/worker/provider_request.go) | Provider request shaping |

### Prompt config

| 入口 | 用途 |
| ---- | ---- |
| [AdminPromptConfigPage.vue](./apps/web/src/features/admin/pages/prompt-config/AdminPromptConfigPage.vue) | Admin prompt UI |
| [engine.go](./apps/api/internal/prompts/engine.go) | Prompt rendering |
| [docs/runbooks/system-prompts.md](./docs/runbooks/system-prompts.md) | 运营模板和变量规则 |

### Auth

| 入口 | 用途 |
| ---- | ---- |
| [middleware.go](./apps/api/internal/identity/auth/middleware.go) | API auth middleware |
| [service_client.go](./apps/api/internal/identity/auth/service_client.go) | Go -> Auth internal client |
| [index.ts](./apps/auth/src/index.ts) | Auth service routes |
| [client.ts](./apps/web/src/shared/auth/client.ts) | Web auth client |
| [user.ts](./apps/web/src/app/stores/user.ts) | User session store |
| [docs/runbooks/auth-operations.md](./docs/runbooks/auth-operations.md) | Auth operation rules |

## Workflow Feature Contract

| Feature | 语义 |
| ------- | ---- |
| `sketch-to-image` | 草图成图 |
| `style-variation` | 风格延展 |
| `partial-edit` | 局部重绘 |
| `remove-bg` | 抠图 |
| `style-fusion` | 多图融合 |
| `pattern-gen` | 图案生成 |
| `hd-upscale` | HD |
| `image-extend` | 扩图 |

Rules:

- `feature` 是顶层任务字段，由后端统一校验。
- `image_urls` 是顶层媒体字段。
- `params` 只承载 feature-specific structured parameters。
- 工作流参数只接受 canonical key；前端 execution contract、OpenAPI 与 Go validation 必须保持同名同层级。
- `use_mask`、`mask_image_url`、`mask_image_urls` 是 `partial-edit` 运行时边界参数，不是 prompt 模板变量。
- 新增或修改 feature 时同步前端 contract、后端 workflow helper、Admin prompt config 和 runbook。

## Generated Code

Generated API:

- `apps/web/src/shared/api/generated-query` 由 Orval 生成，不手动编辑。
- Route pages 和 components 不直接依赖 generated API。
- 业务访问收口到 feature resource、feature adapter 或 shared composable。

sqlc:

- `apps/api/internal/db/sqlc/*.go` 不手动编辑。
- SQL schema/query 变更后运行 `go tool sqlc generate`。
- migrations 位于 `apps/api/internal/db/migrations/`。

## 环境变量要点

本地 API/Auth 至少确认：

| 变量 | 说明 |
| ---- | ---- |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | PostgreSQL 连接 |
| `AUTH_SERVICE_BASE_URL` | Go 调用 Auth service 的地址 |
| `AUTH_INTERNAL_KEY` | API/Auth internal endpoint 共享密钥 |
| `APP_SECRETS_ENCRYPTION_KEY` | API key 和 storage secret 静态加密主密钥 |
| `BETTER_AUTH_SECRET` | Better Auth 签名密钥，生产必须高熵 |
| `BETTER_AUTH_URL` | Auth 对外基准 URL |

Docker compose / deploy 层的 `APP_BASE_URL` 与 `API_BASE_URL` 是部署输入变量，不是 API runtime 直接读取的公开链接配置。生产部署细节见 [docs/runbooks/deployment.md](./docs/runbooks/deployment.md)。

## 改动 Checklist

### 改 Canvas 或 workflow

1. 更新前端 execution contract。
2. 更新 Go workflow validation/helper。
3. 检查 Admin prompt config 展示和变量。
4. 确认 prompt 变量没有承载媒体 URL、mask 或 provider 参数。
5. 跑 Web tests/build 和 API tests。

### 改 task submission/runtime

1. 保持 task insert 和 River enqueue 在同一个 PostgreSQL transaction。
2. 保持 River 是任务队列，NATS 只做事件层。
3. 检查 cancel、terminal state、retry 和 no-capacity 分支。
4. 跑 API tests；跨层状态变化同时跑 Web task event 相关测试。

### 改 Admin 页面

1. Route page 不直接依赖 generated API。
2. 在 vertical context/resource 内处理 command sequencing 和 cache invalidation。
3. 后端 payload ownership 不泄漏到无关 `shared` module。
4. 跑 Web lint/test/build。

### 改 Auth 或安全逻辑

1. 高风险身份动作继续经由 Go Admin API 编排。
2. 不允许前端直连 Better Auth admin client 执行跨库删除/角色变更。
3. 检查 production secret validation。
4. 跑 API/Auth 相关测试，并复核 [docs/runbooks/auth-operations.md](./docs/runbooks/auth-operations.md)。

## Repo Map

```text
.github/workflows/           # CI / release workflow
deploy/                      # reverse proxy templates
docker/                      # local PostgreSQL/NATS and production Compose
docs/agents/                 # agent workflow docs
docs/runbooks/               # deployment, operations, security runbooks
scripts/                     # packaging, deployment and release scripts

apps/web/src/
├── app/                      # app startup, router, store wiring
├── features/                 # business modules
│   ├── canvas/               # Canvas Workspace
│   ├── boards/               # board list/preview/sync
│   ├── admin/                # Admin vertical contexts
│   ├── auth/                 # login/register views
│   ├── user/                 # user layout shell
│   └── profile/              # user center
└── shared/                   # cross-feature API/auth/types/utils/assets

apps/api/
├── cmd/                      # service entrypoints
├── internal/platform/        # config, database, response, safehttp
├── internal/runtime/         # bootstrap, HTTP routes, worker, webdist
├── internal/accountpool/     # accounts/channels/load
├── internal/identity/        # auth/users/admin user management
├── internal/media/           # storage/assets/media refs/image format
├── internal/boards/          # board ownership and management
├── internal/tasking/         # task model/submission/queue/runtime/stats
├── internal/workflow/        # feature contract
└── internal/prompts/         # prompt templates and assembly

apps/auth/
├── src/auth.ts               # Better Auth config
├── src/env.ts                # env validation
└── src/index.ts              # auth and internal routes
```

## 维护规则

1. 以 working tree 为准，不回滚用户未提交改动。
2. 优先小范围、可回滚改动。
3. 跨层 contract 改动默认同时验证前端和后端。
4. 改 domain language 时更新相关 `CONTEXT.md`。
5. 改系统设计 contract 时更新 [DESIGN.md](./DESIGN.md)。
6. 改执行流程或日常入口时更新本文档。
