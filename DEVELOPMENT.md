# IdeaBoard Developer Handbook

`DEVELOPMENT.md` 服务日常开发：怎么启动、改哪里、跑什么验证。系统设计 contract 见 [DESIGN.md](./DESIGN.md)，线上操作见 [docs/runbooks/](./docs/runbooks/)。

## 本地开发

环境要求：

| 组件 | 建议版本 |
| ---- | -------- |
| Node.js | `22.23.2` |
| Go | `1.26.6+` |
| PostgreSQL | `17+` |
| Atlas CLI | `1.3.0` |

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
source ../../docker/.env
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

### Atlas migration workflow

Atlas Community Edition 的 migration contract 使用 `atlas migrate validate`，它会校验 `atlas.sum`、执行 Fresh replay 并检查 PostgreSQL semantic validity。`atlas migrate lint` 属于 Atlas Pro，不作为本项目的 CI 依赖。

生成新的 versioned migration 时，在隔离的 development database 上运行：

```bash
cd apps/api
atlas migrate diff add_feature \
  --dir file://internal/db/migrations \
  --dev-url "${ATLAS_DEV_URL}" \
  --to file://internal/db/schema/schema.sql
```

提交 migration SQL 与更新后的 `internal/db/migrations/atlas.sum`。提交前使用同一份隔离 database 运行 contract check；它还会阻止新增 migration 中未经明确批准的 `DROP TABLE`、`DROP COLUMN`、`TRUNCATE` 等 destructive DDL：

```bash
ATLAS_DEV_URL='postgres://user:password@127.0.0.1:5432/ideaboard_atlas_validate?sslmode=disable' \
bash scripts/atlas-migration-check.sh
```

如确需 destructive migration，SQL 文件必须包含 `-- destructive: approved`，并且只在经过 review 的环境中显式设置 `ATLAS_ALLOW_DESTRUCTIVE=1`；CI 默认始终拒绝。

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
| Test infrastructure modules | `cd apps/api/internal/testdb && go test ./...` and `cd apps/api/internal/testsupport && go test ./...` |
| API build | `cd apps/api && go build ./...` |
| Production dead-code scan | `cd apps/api && deadcode ./...` |
| Test reachability dead-code scan | `cd apps/api && deadcode -test ./...` |
| Schema snapshot generation | `cd apps/api && API_TEST_DATABASE_DSN='<test-dsn>' BASE_SHA=HEAD go run ./cmd/schema-release --write-artifact` |
| Schema Release check | `cd apps/api && BASE_SHA="$(git merge-base HEAD origin/main)" API_TEST_DATABASE_DSN='<test-dsn>' go run ./cmd/schema-release` |
| sqlc regeneration | `cd apps/api && go tool sqlc generate` |
| Auth build | `cd apps/auth && npm run build` |
| Credits million dataset | `cd apps/api && CREDITS_LOAD_TEST=1 go test ./internal/credits -run '^TestCreditMillionTransactionDataset$' -count=1 -v` |
| Usage million dataset | `cd apps/api && CREDITS_LOAD_TEST=1 go test ./internal/usage -run '^TestUsageMillionTransactionDataset$' -count=1 -v` |

### CI parity gate

GitHub 只暴露一个稳定的 required check：`CI Gate`。它在 PR、`main` push 和手动运行上始终创建，先把改动分类为 Web、API、Auth、Schema、Migration Integration、Docker 与 Ops，再只调用受影响的 reusable workflows；docs-only 改动会明确成功。Dependency audit 由 Web/Auth quality 与 API quality 各自负责，周一 schedule 的 `security.yml` 做全量 audit。未知路径采用 fail-safe 分类，执行全部 context，避免新增目录静默绕过 CI。

`CI Gate` 的 Step Summary 会列出本次实际选中的 verification lanes 及结果；耗时以 GitHub Actions 原生 job timing 为事实源。优化 CI 前先对最近多次相同改动类型的运行取基线，重点比较 API race shards、Web Quality 与 Browser Smoke，不以单次 cache hit 或总时长推断收益。

修改 CI classifier、workflow 或 repo-local setup actions 后，先运行本地 contract tests：

```bash
bash scripts/ci-change-classifier-test.sh
bash scripts/ci-gate-test.sh
bash scripts/ci-workflow-contract-test.sh
bash scripts/ci-parity-test.sh
bash scripts/ci-migration-integration-test.sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/*.yml
```

Node 版本只维护在根目录 `.node-version`，Go 版本只维护在 `apps/api/go.mod`。Workflows 通过 `.github/actions/setup-node` 与 `.github/actions/setup-go` 读取版本和 dependency cache contract，不要在 workflow 内复制 runtime version。

Migration Integration 的 Auth build、runtime role 初始化与 API/Auth 两轮幂等迁移统一由 `scripts/ci-migration-integration.sh` 编排；workflow 只负责准备 PostgreSQL、Node、Go 与 locked dependencies，不复制迁移顺序。

`scripts/ci-gate.sh` 统一生成 lane summary 并验证 required results；`scripts/ci-workflow-contract-test.sh` 使用结构化 YAML checker 验证 workflow semantics，避免把 YAML 格式当作 contract。

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

Rewards 是 pre-launch clean break：RewardCode 由单一 canonical migration 创建。API schema 由 Atlas versioned migrations 管理；若本地数据库不符合当前 Atlas history，请显式重建 Fresh Database，不要补空 migration、手删 Ledger 或尝试兼容性回放。

## 常用入口

### Canvas workflow

| 入口 | 用途 |
| ---- | ---- |
| [CanvasPage.vue](./apps/web/src/features/canvas/pages/CanvasPage.vue) | Canvas route page |
| [executionContract.ts](./apps/web/src/features/canvas/execution/contracts/executionContract.ts) | 前端 workflow feature contract |
| [generationIntent.ts](./apps/web/src/features/canvas/image-workflow/generationIntent.ts) | Prompt/node execution orchestration |
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
| [web-performance.md](./docs/benchmarks/web-performance.md) | Web bundle size guard 与 Canvas interaction benchmark |

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
| `render` | 草图成图 |
| `variations` | 风格延展 |
| `inpaint` | 局部重绘 |
| `cutout` | 抠图 |
| `fusion` | 多图融合 |
| `upscale` | 放大 |
| `outpaint` | 扩图 |

Rules:

- `feature` 是顶层任务字段，由后端统一校验。
- `image_urls` 是顶层媒体字段。
- `params` 只承载 feature-specific structured parameters。
- 工作流参数只接受 canonical key；前端 execution contract、OpenAPI 与 Go validation 必须保持同名同层级。
- `use_mask`、`mask_image_url`、`mask_image_urls` 是 `inpaint` 运行时边界参数，不是 prompt 模板变量。
- 新增或修改 feature 时同步前端 contract、后端 workflow helper、Admin prompt config 和 runbook。

## Generated Code

Generated API:

- `apps/web/src/shared/api/generated-query` 由 Orval 生成，不手动编辑。
- Route pages 和 components 不直接依赖 generated API。
- 业务访问收口到 feature resource、feature adapter 或 shared composable。

sqlc:

- `apps/api/internal/db/sqlc/*.go` 不手动编辑。
- SQL schema/query 变更后运行 `go tool sqlc generate`。
- Atlas migrations 位于 `apps/api/internal/db/migrations/`，由 `apps/api/atlas.hcl` 与 `atlas.sum` 管理。
- Atlas migration 生成使用 `atlas migrate diff`；完整性与 Fresh replay 使用 `scripts/atlas-migration-check.sh`。CI 先在隔离 database 执行 `atlas migrate validate`，再在集成 database 执行 Fresh apply 与 incremental idempotency apply；不要把 Atlas Pro-only 的 `atlas migrate lint` 加入流水线。
- API schema 由独立 `api-migrate` image 执行 Atlas versioned migrations，再执行 `finalize-migrate` 完成 River 与 runtime privilege setup。Pre-launch rebaseline 只能原子删除旧 SQL chain 并新增一个更高版本 baseline，随后重建 Fresh Database；不得手工编辑 Atlas revisions 表。
- Schema source 的唯一 lifecycle 是 `schema-release`：修改 domain fragment 或 manifest 后先用 `--write-artifact` 生成 `schema.sql`，再运行默认 check；CI 和 release gate 只运行默认 check。
- clean-break release 只支持 Fresh Database。旧数据库必须先导出需要保留的数据，再重建并按当前 baseline 初始化；不能使用旧 migration binary 做跨 baseline 升级或回滚。

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

Docker compose / deploy 层的 `PUBLIC_ORIGIN` 是公开 Origin 的 canonical source；`API_BASE_URL` 仅在 API 使用独立 host 时作为 optional override。API runtime 仍分别生成 Web 跳转/分享链接与 Stripe Webhook/服务端素材地址；`PUBLIC_ORIGIN` 缺失或无效时拒绝启动。生产部署细节见 [docs/runbooks/deployment.md](./docs/runbooks/deployment.md)。

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
├── src/admission/            # registration, Origin/CORS and Rate Limit policy
├── src/application/          # Better Auth and production composition
├── src/infrastructure/       # PostgreSQL, migration, Runtime and env adapters
├── src/social-identity/      # OAuth Provider settings and runtime registry
├── src/transactional-mail/   # typed templates, Resend and Mail settings
├── src/trusted-identity/     # canonical identity and internal commands
├── src/index.ts              # service entrypoint
└── src/migrate.ts            # migration entrypoint
```

## 维护规则

1. 以 working tree 为准，不回滚用户未提交改动。
2. 优先小范围、可回滚改动。
3. 跨层 contract 改动默认同时验证前端和后端。
4. 改 domain language 时更新相关 `CONTEXT.md`。
5. 改系统设计 contract 时更新 [DESIGN.md](./DESIGN.md)。
6. 改执行流程或日常入口时更新本文档。
