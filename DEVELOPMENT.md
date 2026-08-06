# IdeaBoard Developer Handbook

`DEVELOPMENT.md` 服务日常开发：怎么启动、改哪里、跑什么验证。系统设计 contract 见 [DESIGN.md](./DESIGN.md)，线上操作见 [docs/runbooks/](./docs/runbooks/)。

## 本地开发

环境要求：

| 组件 | 建议版本 |
| ---- | -------- |
| Node.js | `22.23.2` |
| Go | `1.26.5+` |
| PostgreSQL | `17+` |

启动本地依赖：

```bash
cd docker
cp .env.example .env
cp migration.env.example migration.env
# 至少修改 POSTGRES_PASSWORD、API_DB_PASSWORD 与 AUTH_DB_PASSWORD；
# migration.env 的 DB_MIGRATION_PASSWORD 必须与 POSTGRES_PASSWORD 相同。
docker compose -f compose.local.yml up -d
```

初始化数据库时必须先执行 API migration，再执行 Auth migration。Auth 的 limited runtime role 通过后者获得 Better Auth tables 和 typed runtime settings function 权限：

```bash
cd apps/api
cp .env.example .env
set -a
source ../../docker/migration.env
set +a
go run ./cmd migrate
```

```bash
cd apps/auth
cp .env.example .env
npm install
set -a
source ../../docker/migration.env
set +a
npm run migrate
```

然后分别启动 Auth 与 API：

```bash
cd apps/auth
npm run dev
```

```bash
cd apps/api
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
INIT_ADMIN_EMAIL='admin@example.com' \
INIT_ADMIN_DISPLAY_NAME='Admin' \
INIT_ADMIN_PASSWORD='your-strong-password' \
  go run ./cmd init-admin
```

`INIT_ADMIN_PASSWORD` 必须为 10–128 个字符；执行命令前 Auth Service 必须已启动。

初始化已有开发身份的 Credit Wallet 时，传入 Auth 导出的 trusted canonical registration identity JSON；该命令幂等，普通 API startup 不会扫描用户。`CREDITS_WELCOME_IDENTITY_KEY` 首次使用后会在数据库绑定 fingerprint，误更换会 fail closed，不能当作普通可轮换 secret：

```bash
cd apps/api
CREDITS_WELCOME_IDENTITY_KEY='<base64-32-byte-key>' \
  go run ./cmd init-credit-wallets verified-identities.json
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
| Credits million dataset | `cd apps/api && CREDITS_LOAD_TEST=1 go test ./internal/credits -run '^TestCreditMillionTransactionDataset$' -count=1 -v` |
| Usage million dataset | `cd apps/api && CREDITS_LOAD_TEST=1 go test ./internal/usage -run '^TestUsageMillionTransactionDataset$' -count=1 -v` |

### CI parity gate

Push 前需要复现 GitHub 的真实 PostgreSQL integration tests 与 Linux browser geometry 时，在仓库根目录运行：

```bash
export API_TEST_DATABASE_DSN='postgres://ideaboard:your-password@127.0.0.1:5432/ideaboard?sslmode=disable'
./scripts/ci-parity.sh
```

该入口与 GitHub Workflows 共用 `scripts/ci-parity.sh`：API tests 固定禁用 Go test cache（`-count=1`），缺少 `API_TEST_DATABASE_DSN` 会立即失败，不会静默 `SKIP` PostgreSQL integration tests。Linux 直接运行 Web smoke；macOS 等其他系统通过 Docker 中固定版本的 Playwright Linux image 运行，因此需要 Docker 已启动。container 使用只读源码副本，不会改写宿主的 `node_modules`。

只复现单项时可运行：

```bash
TEST_MODE=standard TEST_SHARD=0 TEST_SHARD_COUNT=1 ./scripts/ci-parity.sh api-test
./scripts/ci-parity.sh web-smoke
```

普通 `go test ./...` 仍允许在未配置测试数据库时跳过 integration tests；严格 fail-closed 仅由 parity/CI mode 启用。完整 lint、build、migration、sqlc drift 和 vulnerability checks 仍由上表与 GitHub 的独立 jobs 执行。

修改 `apps/api/internal/db/queries` 或 schema 后：

```bash
cd apps/api
go tool sqlc generate
git diff --exit-code -- internal/db/sqlc
```

Rewards 是 pre-launch clean break：RewardCode 由单一 canonical migration 创建。若本地数据库包含已删除的 Rewards migration version 或历史 Rewards Ledger fact，`go run ./cmd migrate` 会 fail closed；请显式重建 Fresh Database，不要补空 migration、手删 Ledger 或尝试兼容性回放。

## 常用入口

### Canvas workflow

| 入口 | 用途 |
| ---- | ---- |
| [CanvasPage.vue](./apps/web/src/features/canvas/pages/CanvasPage.vue) | Canvas route page |
| [executionContract.ts](./apps/web/src/features/canvas/execution/contracts/executionContract.ts) | 前端 workflow feature contract |
| [useFlowPromptGenerationOrchestrator.ts](./apps/web/src/features/canvas/composables/useFlowPromptGenerationOrchestrator.ts) | Prompt/node execution orchestration |
| [useFlowImageActionOrchestrator.ts](./apps/web/src/features/canvas/composables/useFlowImageActionOrchestrator.ts) | Image node direct actions |
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
| [authSession.ts](./apps/web/src/features/auth/session/authSession.ts) | User session store |
| [docs/runbooks/auth-operations.md](./docs/runbooks/auth-operations.md) | Auth operation rules |

### Credits

| 入口 | 用途 |
| ---- | ---- |
| [service.go](./apps/api/internal/credits/service.go) | Grant/Hold/Capture/Release/Refund Command seam 与 caller-owned transaction adapter |
| [reconciliation.go](./apps/api/internal/credits/reconciliation.go) | 自动账本检查、Run history 与全局 Finding queue；Stale Hold 固定 24 小时且不接受 Admin 配置 |
| [CONTEXT.md](./apps/api/internal/credits/CONTEXT.md) | Credits domain language 与 invariants |
| [credits-million.md](./docs/benchmarks/credits-million.md) | 显式百万流水 harness 与参考结果 |

### Rewards

| 入口 | 用途 |
| ---- | ---- |
| [service.go](./apps/api/internal/rewards/service.go) | RewardCode、Redemption create/list/redeem/history 与 Credits Grant 编排 |
| [CONTEXT.md](./apps/api/internal/rewards/CONTEXT.md) | Rewards Domain Language 与 invariants |

### Billing

| 入口 | 用途 |
| ---- | ---- |
| [service.go](./apps/api/internal/billing/service.go) | 全局 SKU Catalog、versioned pricing 与 Quote seam |
| [CONTEXT.md](./apps/api/internal/billing/CONTEXT.md) | Billing domain language、SKU namespace 与 snapshot invariants |
| [service.go](./apps/api/internal/tasking/taskbilling/service.go) | AI Image Workflow SKU resolver、Readiness、Submit Hold 与 terminal Capture/Release orchestration |

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
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | 当前 API/Auth 进程的 runtime PostgreSQL 连接；runtime role 不拥有 schema |
| `DB_MIGRATION_USER` / `DB_MIGRATION_PASSWORD` | 仅 API/Auth migration command 使用的 schema owner；必须与 `DB_USER` 不同 |
| `AUTH_SERVICE_BASE_URL` | Go 调用 Auth service 的地址 |
| `AUTH_INTERNAL_KEY` | API/Auth internal endpoint 共享密钥 |
| `APP_SECRETS_ENCRYPTION_KEY` | API key 和 storage secret 静态加密主密钥 |
| `REWARDS_CODE_HMAC_KEYS` | Rewards Code current/previous keyed-HMAC JSON registry；key 至少 32 bytes，不能放入 Admin settings |
| `BETTER_AUTH_SECRET` | Better Auth 签名密钥，生产必须高熵 |
| `BETTER_AUTH_URL` | Auth 对外基准 URL |

Auth 使用相同的 owner/runtime separation：长期运行的 Auth 读取 `DB_USER=ideaboard_auth`，`npm run migrate` 通过 `DB_MIGRATION_USER` 临时使用 owner，并在完成后只授予 Better Auth tables 权限。本地 migration credential 只放在已忽略的 `docker/migration.env`，不要复制进 API/Auth runtime `.env`。

Docker compose / deploy 层的 `APP_BASE_URL` 与 `API_BASE_URL` 是部署输入变量，不是 API runtime 直接读取的公开链接配置。生产部署细节见 [docs/runbooks/deployment.md](./docs/runbooks/deployment.md)。

API 在独立 `METRICS_ADDR` listener 暴露 Prometheus `/metrics`，默认 `127.0.0.1:9091`。该 listener 只用于本机或私有监控网络，不得接入用户 API 反代或 Cloudflare。

轮换 `REWARDS_CODE_HMAC_KEYS` 时先部署 new current + old previous；新 Code 只使用 current，lookup 同时验证两者。任何 persisted RewardCode 的 primary `key_version` 未配置时，API startup 都会 fail closed；disabled、expired 或未兑换都不例外。只有数据库中不再存在依赖 old version 的 RewardCode 时，才能移除 previous。

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
├── app/                      # app startup, router, theme wiring
├── features/                 # business modules
│   ├── account-management/   # user account settings
│   ├── admin/                # Admin vertical contexts
│   ├── auth/                 # login/register views
│   ├── board-catalog/        # board list and previews
│   ├── canvas/               # Canvas Workspace
│   ├── canvas-board/         # board domain and persistence
│   ├── legal/                # legal pages
│   ├── user/                 # user routes and layout
│   └── user-shell/           # user navigation and shell controls
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
