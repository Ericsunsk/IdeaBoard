# IdeaBoard Developer Handbook

`DEVELOPMENT.md` 服务日常开发：怎么启动、改哪里、跑什么验证。系统设计 contract 见 [DESIGN.md](./DESIGN.md)，线上操作见 [docs/runbooks/](./docs/runbooks/)。

## 本地开发

本地一键起全栈（根目录 `process-compose.yml`；dashboard 与本地 imgproxy
错开端口——imgproxy 占 `8080`，dashboard 用 `PC_PORT_NUM=8081`）：

```bash
PC_PORT_NUM=8081 process-compose up
```

`up` 自动按 `docker-deps → migrate → api/auth/realtime/web` 顺序起：
infra 容器先行，随后 one-shot `migrate` 跑 `scripts/migrate.sh --env dev`
（API→Auth 强制顺序，幂等，重跑无操作），成功才放行四个应用进程；
migrate 失败整栈停住并报错，不带病起服务。日常开发无需手跑 migrate。

环境要求：

| 组件       | 建议版本                                    |
| ---------- | ------------------------------------------- |
| Node.js    | 以根目录 `.node-version` 为准               |
| Go         | 以 `apps/api/go.mod` 的 `go` directive 为准 |
| PostgreSQL | `17+`                                       |
| Atlas CLI  | `1.3.2`                                     |

启动本地依赖（`docker/.env` 已是加密进 git 的文件，全新 clone 自带，
不要再从 example 覆盖它；`migration.env` 仍是本机文件，缺失才创建）：

> [!IMPORTANT]
> 本地先填齐 7 个必填再起服务：`API_PG_PASSWORD`、`AUTH_INTERNAL_KEY`、
> `RT_TICKET_SECRET`、`APP_SECRETS_ENCRYPTION_KEY`、`CREDITS_WELCOME_IDENTITY_KEY`、
> `REWARDS_CODE_HMAC_KEYS`、`PUBLIC_ORIGIN`——缺一即启动 fail-closed（行为不变）。

```bash
cd docker
test -f migration.env || cp migration.env.example migration.env
# migration.env 的 PG_MIGRATION_PASSWORD 必须与 .env 解密后的 POSTGRES_PASSWORD 相同。
dotenvx run -f .env -- docker compose -f compose.local.yml up -d
```

NATS 本地端口为 `127.0.0.1:4222`，与 production 一样使用 `ideaboard` +
`NATS_PASSWORD` 认证；依赖 Compose 与宿主应用必须消费同一 dev secret 集合。
本地栈与生产 NATS 均为必填（`compose.local.yml` / `compose.yml` 缺 `NATS_PASSWORD`
即 fail fast；文档与 Compose 已按必填对齐，不再标可选）。仅在不带 NATS 直跑
API 二进制时（如单测）才回落到进程内 SSE fanout。builtin-local 始终从进程 cwd 解析一次 root：宿主在 `apps/api` 启动，容器
`WORKDIR /app`，两者的 `uploads/` 与 imgproxy mount 一致。
本地 API 的 trusted proxies 仅 loopback；production Compose 才信任私网 bridge。
API 的 trusted proxies 是代码常量、无 env 开关（`AppConfig.TrustedProxies`）：
本地只信 loopback，production 追加私网段（`10.0.0.0/8`、`172.16.0.0/12`、
`192.168.0.0/16`）。裸机/拆分部署须把反代放在上述范围内（宿主 loopback 或
RFC1918 内网），否则客户端 IP 与限流归因错误；确需其他网段时改代码，不加开关。

### dotenvx（compose 边界的 secret 通道）

`docker/.env` 是 dotenvx 加密后进 git 的文件（只 review key 名，不看值）；
`docker/.env.example` 保持明文模板。本地全栈（`go run` / `npm run dev`）
经 Infisical Cloud 注入，不依赖本机明文 `.env`（仓库内无跟踪的明文 `.env`，
均为 gitignored）；entrypoint 的 dotenv 只在文件存在时才读且永不覆盖已注入值，
单跑排障仍可用各应用的 `.env.example` 模板：

```bash
npm i -g @dotenvx/dotenvx
# 本地起 API（示例，在 apps/api 下）：Cloud 取 dev 全集注入（默认 `/`），断网时改走 dotenvx 解密见下
../../scripts/infisical-run.sh --env dev -- go run ./cmd --mode all
# 私钥从密码管理器取，用完即清，不落地（本机不再保留 .env.keys 文件）；
export DOTENV_PRIVATE_KEY='...'
dotenvx run -f docker/.env -- docker compose -f docker/compose.local.yml up -d
```

新机器 onboarding：`git clone` + 放好私钥即可 `up`，不再需要逐项手填 secret。
改 secret 走 PR（`dotenvx set KEY value -f docker/.env` 就地重加密，只产生
key 名与密文 diff；`dotenvx get KEY -f docker/.env` 单值核对）。加密文件
key 名受 `scripts/env-schema-check.sh` 的 F 门禁校验。

本地可选 `direnv`（不强制）：在 `docker/.envrc` 写
`export DOTENV_PRIVATE_KEY='...'`（用完即清，不落地），
进目录自动加载，`dotenvx run` 不再需要手动 export。示例仅文档，不提交 `.envrc`。

手动/排障才直跑 `scripts/migrate.sh`（与 compose 内同一入口、同一幂等语义，
重跑无操作）。Auth 的 limited runtime role 通过 Auth migration 获得 Better Auth tables 和 typed runtime settings function 权限：

```bash
scripts/migrate.sh --env dev            # api→auth 一次跑完
scripts/migrate.sh --env dev --only api # 只跑 API migration
scripts/migrate.sh --env dev --only auth # 只跑 Auth migration（首次跑前先 cd apps/auth && npm install）
```

`docker/migration.env` 缺失时 migrate 直接报错并指引
`cd docker && cp migration.env.example migration.env`，不用空值硬跑。

脚本用 `infisical export --format=dotenv` 落 0600 临时文件再 `set -a; source`，合并 Cloud 全集与 `docker/migration.env`
（owner 口令仍只走本地文件，不进 Cloud；不走 dotenvx，也不走会净空父 env 的 `infisical run`）。
开跑前先等 postgres accept 连接（`pg_isready` 探活，60s 超时），再用运行时 role 直连 `SELECT 1` 预检：Cloud 口令与 volume 指纹不一致直接报错，不进 60 次重试；
Welcome key 指纹对 guard 表比对，不一致提前报（fresh DB 无 guard 表时跳过并注明）。

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

如确需 destructive migration，SQL 文件必须包含 `-- destructive: approved`，并且只在经过 review 的环境中显式设置 `ATLAS_ALLOW_DESTRUCTIVE=1`；CI 默认始终拒绝。被拦截的 DDL 包括 `DROP TABLE` / `DROP COLUMN` / `DROP CONSTRAINT` / `DROP SCHEMA` / `DROP INDEX` / `DROP VIEW` / `DROP FUNCTION` / `DROP TRIGGER` / `TRUNCATE`，bare `DROP <object>` 与 `ALTER TABLE ... DROP <object>` 两种写法都会命中。

不用 process-compose、改为手动单起时，才分别启动 Auth 与 API：

```bash
cd apps/auth
npm run dev
```

```bash
cd apps/api
go run ./cmd
```

本地默认 `API_MODE=all`（单进程同时跑 HTTP 与 River workers）。拆分冒烟时用同一二进制起双进程（同库，需先 migrate）：

```bash
cd apps/api
go run ./cmd --mode=http-only &
go run ./cmd --mode=worker-only &
```

`--mode` 覆盖 `API_MODE`；生产 compose 由 `docker/compose.yml` 按 service 固定字面值（`api` 为 http-only，`api-worker` 为 worker-only），见 ADR-0021。

启动 Web：

```bash
cd apps/web
npm install
npm run dev
```

初始化管理员（与 migrate 同形态，跑完自动 unset `INIT_ADMIN_*`；执行前 Auth Service 必须已启动）：

```bash
scripts/migrate.sh --env dev --only init-admin \
  --admin-email 'admin@example.com' \
  --admin-password 'your-strong-password' \
  --display-name 'Admin'
```

`--admin-password` 必须为 10–128 个字符；`--display-name` 可省（默认取邮箱 local part）。

初始化已有开发身份的 Credit Wallet 时，传入 Auth 导出的 trusted canonical registration identity JSON；该命令幂等，普通 API startup 不会扫描用户。`CREDITS_WELCOME_IDENTITY_KEY` 首次使用后会在数据库绑定 fingerprint，误更换会 fail closed，不能当作普通可轮换 secret：

```bash
cd apps/api
CREDITS_WELCOME_IDENTITY_KEY='<base64-32-byte-key>' \
  go run ./cmd init-credit-wallets verified-identities.json
```

开发代理：

| Path            | Target                  |
| --------------- | ----------------------- |
| `/api/auth/**`  | `http://localhost:6001` |
| `/api/**`       | `http://localhost:6000` |
| `/realtime` WS  | `http://localhost:6002` |

## 验证矩阵

日常小改优先跑最小相关检查；跨层 contract 改动同时验证前后端。

| 场景                             | 命令                                                                                                                                                                             |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Web unit/spec                    | `cd apps/web && npm run test:run`                                                                                                                                                |
| Web test typecheck               | `cd apps/web && npm run test:typecheck`                                                                                                                                          |
| Web browser-level spec           | `cd apps/web && npm run test:browser`                                                                                                                                            |
| Web build/type                   | `cd apps/web && npm run build`                                                                                                                                                   |
| Web lint                         | `cd apps/web && npm run lint`                                                                                                                                                    |
| Web style lint                   | `cd apps/web && npm run lint:style`                                                                                                                                              |
| API tests                        | `cd apps/api && go test ./...`                                                                                                                                                   |
| Test infrastructure modules      | `cd apps/api/internal/testdb && go test ./...` and `cd apps/api/internal/testsupport && go test ./...`                                                                           |
| API build                        | `cd apps/api && go build ./...`                                                                                                                                                  |
| Production dead-code scan        | `cd apps/api && GOTOOLCHAIN="go$(awk '/^go [0-9]/ {print $2; exit}' go.mod)" deadcode ./...`                                                                                     |
| Test reachability dead-code scan | `cd apps/api && GOTOOLCHAIN="go$(awk '/^go [0-9]/ {print $2; exit}' go.mod)" deadcode -test ./...`                                                                               |
| Schema snapshot generation（本地）     | `cd apps/api && API_TEST_DATABASE_DSN='<test-dsn>' BASE_SHA=HEAD go run ./cmd/schemarelease --write-artifact`                                                                    |
| Schema Release check（CI/release）     | `cd apps/api && BASE_SHA="$(git merge-base HEAD origin/main)" API_TEST_DATABASE_DSN='<test-dsn>' go run ./cmd/schemarelease`                                                     |
| sqlc regeneration（query-only）        | `cd apps/api && go tool sqlc generate`                                                                                                                                           |
| Auth build                       | `cd apps/auth && npm run build`                                                                                                                                                  |
| Credits million dataset          | `cd apps/api && API_TEST_DATABASE_DSN='<test-dsn>' CREDITS_LOAD_TEST=1 go test ./internal/credits/application/commands -run '^TestCreditMillionTransactionDataset$' -count=1 -v` |
| Usage million dataset            | `cd apps/api && API_TEST_DATABASE_DSN='<test-dsn>' CREDITS_LOAD_TEST=1 go test ./internal/usage -run '^TestUsageMillionTransactionDataset$' -count=1 -v`                         |

Schema Release 的 `BASE_SHA` 按场景取值，不要混用：本地生成用 `BASE_SHA=HEAD`，只比 working tree 与 `HEAD` 的差集，避免把未合入的历史误判为 release 变更；CI/release check 使用 `scripts/ci-baseline.sh` 给出的已成功 CI 祖先，累计覆盖尚未通过验证的提交；不能自行改成 `event.before` 或当前 `origin/main` 的 merge-base。`--base-sha` 缺省读 `BASE_SHA` env，`--head-sha` 缺省为 `HEAD`，`api.yml` 传可信的累计成功祖先 `--base-sha "${ATLAS_BASE_SHA}"`，并在执行前校验 SHA、checkout 与 ancestry；`API_TEST_DATABASE_DSN` 必填，缺失即 fail-closed。`--write-artifact` 仍先跑完全部只读校验再发布 `schema.sql`，不是跳检直写。

`internal/db/migrationhistory` 是 Git migration discovery、append-only 与原子 clean-break rebaseline 判定的唯一实现；Schema Release 直接使用，Atlas Shell gate 经 `cmd/migrationhistory` 调用。两者都覆盖 commit range、working tree 与 untracked SQL；风险 SQL lint、版本冲突、checksum 和 Fresh replay 门禁继续独立生效。Clean-break 要求以更高版本替换一个 baseline，允许删除旧 chain；合并后的 CI 区间可附带严格高于新 baseline 的新增尾迁移，仍只走 Fresh 重放，拒绝第二 baseline、低版本新增和已有文件的修改或重命名。

### Frontend 工具与治理规则

Formatter 使用 Prettier（`singleQuote`、无分号、`printWidth: 100`），通过 `npm run format:check` 检查。i18n 四 locale（`en`、`zh-CN`、`es`、`ko`）parity 由 `apps/web/scripts/checkLocaleParity.ts`（`npm run lint:locale-parity`）与 `localeParity.unit.spec.ts` 锁定，CI 通过 governance 与 unit jobs 执行同一校验；交互与样式行为通过 Playwright browser specs 验证。请求数硬约束见 [api CONTEXT](./apps/web/src/api/CONTEXT.md)（单 provider + 标量 key、最小化 invalidate、懒加载 + 聚合读优先）；新增 query key 工厂必须在 `queryKeyRequestBudget.contract.spec.ts` 登记，同屏 distinct GET 超 4 个必须走聚合读或懒加载。

ESLint 直接执行治理规则。Canvas 高频交互态禁用 Query 为 error；其余治理规则的 severity 以 `eslint.config.js` 为准。boolean-prefix 使用 TypeScript type-aware ESLint，现有例外保存在 `apps/web/scripts/booleanNaming.baseline.json` 的 declaration identities（file、lexical owner、kind/name、occurrence）；新增同文件声明仍 warning，lint 不自动刷新基线，`npm run lint` 以 `--max-warnings=0` 拒绝任何 warning。自有 boolean fields 与仓内消费者直接使用 predicate names，不保留旧名 alias。DOM、第三方 SDK 与 generated DTO 字段按真实类型契约检查；自有 wire/persisted DTO 用 `@externalContract` 标明外部契约，仅豁免该契约的字段。显式 dictionary 的 keys 是 identities，显式声明的自有 fields 仍受命名检查。`no-barrel-import` 提示 cascading/未声明 barrel，canonical public Seams 按 inventory 保留。

### Frontend 开发约定

新增 feature 的 ownership、zones 与 public seams 遵循 [DESIGN.md](./DESIGN.md)；UI 实现遵循 [apps/web/DESIGN.md](./apps/web/DESIGN.md)。在 feature 的 `CONTEXT.md` 记录领域术语与独有规则。额外 public subpath 必须登记在 [Seam inventory](./docs/frontend-seam-inventory.md)，Admin verticals 不需要统一根 barrel。

Specs 与被测 module/control colocate，使用四种 suffix：

- `unit`：确定性的纯逻辑。
- `contract`：无真实 Backend 的 API shape、含正反例的 architecture policy fixtures、通过 `toBe` 验证的公开 Seam/singleton identity。
- `component`：通过 `src/test/render` 验证 SFC 的可观察行为。
- `browser`：必须由真实 Browser 验证的 geometry。

`describe` 按 Seam 分组，`test` 描述可观察行为，不断言私有实现。新测试统一使用 `test`，现有 `it` 无需仅为命名修改。UI queries 优先 Role；TestId 和 class assertions 只用于已有约定允许的 visual/layout contract。新 factory 使用 `buildX(overrides?)`，现有 `makeX` 保持原契约。新增 cached server state 的 Query 使用范围遵循 [core CONTEXT](./apps/web/src/core/CONTEXT.md) 并在 feature `CONTEXT.md` 说明。

Composable 有两个或更多参数时使用 options object，单个必填参数保持 positional。`create*Feedback` factory 保持 `toast` 为第一个 positional 参数。

### 显式负载 harness

两条百万流水命令都是 `CREDITS_LOAD_TEST=1` 门控的显式 harness，不在默认 suite 内：

- 必须显式传 `API_TEST_DATABASE_DSN`。未设置时两个 harness 都只打印
  `set API_TEST_DATABASE_DSN to run PostgreSQL integration tests` 然后 `SKIP`，
  `go test` 仍然报 `PASS`——静默跳过，不是通过。
- Credits harness 在子包 `./internal/credits/application/commands`（不是
  `./internal/credits`）；Usage harness 就在 `./internal/usage` 根下。写错包路径
  会得到 `[no tests to run]` 而没有任何失败信号。

`deadcode` 是外部工具（`golang.org/x/tools/cmd/deadcode`），**不在**
`apps/api/go.mod` 的 `tool` block 里，因此没有 `go tool deadcode`。它也不在任何
CI lane 中跑——API Quality 用 `golangci-lint`（其中 `unused` 与它是不同算法）。
首次使用先安装，版本跟随项目依赖的 `golang.org/x/tools`：

```bash
cd apps/api
go install "golang.org/x/tools/cmd/deadcode@$(go list -m -f '{{.Version}}' golang.org/x/tools)"
```

上表命令里的 `GOTOOLCHAIN` 是必需的：`deadcode` 的 source-processing 包必须与运行
`go list` 的 Go toolchain 同版本。用本机更高版本的 toolchain 会解析 stdlib 失败或
在 RTA 阶段直接 panic。版本从 `apps/api/go.mod` 的 `go` directive 推导，不要在本文档
里硬编码版本字面量。

### CI parity gate

Frontend 函数长度检查使用 `governance/function-length`：每个函数最多 100 行自身代码，空行和纯注释不计，嵌套函数体由各自的检查负责。仅 `.spec.ts` 中从 Vitest 导入的 `describe` 分组回调豁免；`it`、hooks、测试 helper 和生产函数仍接受检查。`complexity ≤ 15`、`max-depth ≤ 4` 保持独立约束。完整组件/工厂体积仍通过 `npm run audit:complexity` 报告，避免把单函数检查误当作整个模块的复杂度指标。

GitHub 只暴露一个稳定的 required check：`CI Gate`。PR 与 `main` push 基于当前 checkout 的已成功 CI 祖先计算累计变更，覆盖失败提交遗留的改动；找不到可信祖先、历史不完整或 GitHub API 不可用时回退全量。手动 CI 与发布前验证始终使用全量模式，同时解析同一可信比较基线；全量决定测试范围，不跳过 OpenAPI/migration risk 的历史审查。若无法取得可信祖先，测试仍执行全量，所选历史审查门禁明确失败，不能以 `HEAD`、上一失败提交或空比较绕过。已成功验证的当前 source commit 可作为全量重验的比较基线。未知路径采用 fail-safe 分类，执行全部 context，避免新增目录静默绕过 CI。

普通 Go 改动分别触发 API 测试与 build，不无条件重跑 sqlc 和 migration；queries/generated SQL 与 schema/migration 则选中对应门禁。API tests 使用 Go import、test import 的传递反向依赖选择受影响包，并保留源码扫描的 architecture/ownership/cross-stack contracts；无法可靠识别输入时回退完整 API suite。新增源码扫描测试应使用 `architecture_test.go` / `*boundary_test.go` 命名，其他跨模块输入必须登记在 canonical `scripts/ci-api-test-plan.go` 的 source contract ownership，跨应用输入同时更新 classifier 的消费方，并补正反例。Web 局部改动使用现有 Vitest 的依赖选择能力，额外保留 source contract specs 与文件系统读取方的测试；unit 按选中集合分片，browser、lint、typecheck、build 与 smoke 保留。配置、tokens、公共契约等影响不明的变更回退完整验证。所选测试继续使用同一 runner、Standard/Race 与真实 PostgreSQL/NATS，未增加另一套执行实现。

Repo-local actions 按实际调用方分类；例如部署 action 只触发 Ops/部署契约检查，Node setup 不触发完整 API tests。日常源代码变更不额外构建 preview images；Dockerfile、依赖等 packaging 输入变化仍验证镜像构建。正式发布在镜像构建、签名与发布之前，复用同一 `ci.yml` 对 preflight 固定的 source commit 执行完整验证，不能以增量 green 替代。Dependency audit 由各应用门禁负责，周一 `security.yml` 继续执行全量 dependency audits。

`CI Gate` 的 Step Summary 会列出本次实际选中的 verification lanes 及结果；耗时以 GitHub Actions 原生 job timing 为事实源。优化 CI 前先对最近多次相同改动类型的运行取基线，重点比较 API race shards、Web Quality 与 Browser Smoke，不以单次 cache hit 或总时长推断收益。

修改 CI classifier、workflow 或 repo-local setup actions 后，先运行本地 contract tests：

```bash
bash scripts/ci-change-classifier-test.sh
bash scripts/ci-baseline-test.sh
bash scripts/ci-gate-test.sh
bash scripts/ci-workflow-contract-test.sh
bash scripts/handbook-version-contract-test.sh
bash scripts/ci-parity-test.sh
bash scripts/ci-migration-integration-test.sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/*.yml
```

Node 版本只维护在根目录 `.node-version`，Go 版本只维护在 `apps/api/go.mod`。Workflows 通过 `.github/actions/setup-node` 与 `.github/actions/setup-go` 读取版本和 dependency cache contract，不要在 workflow 内复制 runtime version。Dockerfile 中的 `NODE_VERSION` / `golang` pin 只是可独立构建的 fallback，必须与 canonical source 一致；handbook 入口链接与版本一致性由 `scripts/handbook-version-contract.sh` 在 CI 强制校验（见 `Handbook Contract` lane）。

Migration Integration 的 Auth build、runtime role 初始化与 API/Auth 两轮幂等迁移统一由 `scripts/ci-migration-integration.sh` 编排；workflow 只负责准备 PostgreSQL、Node、Go、Atlas 与 locked dependencies，不复制迁移顺序。Fresh validation 与两轮 apply 共用 workflow 准备的 `ATLAS_BINARY`，不重复下载安装；本地直跑该脚本时需预先安装 Atlas，或通过 `ATLAS_BINARY` 指定可执行文件。

`scripts/ci-gate.sh` 统一生成 lane summary 并验证 required results；`scripts/ci-workflow-contract-test.sh` 使用结构化 YAML checker 验证 workflow semantics，避免把 YAML 格式当作 contract。

Push 前需要复现 GitHub 的真实 PostgreSQL integration tests 与 Linux browser geometry 时，在仓库根目录运行：

```bash
export API_TEST_DATABASE_DSN='postgres://ideaboard:your-password@127.0.0.1:5432/ideaboard?sslmode=disable'
export API_TEST_NATS_URL='nats://ideaboard:your-password@127.0.0.1:4222'
./scripts/ci-parity.sh
```

该入口与 GitHub Workflows 共用 `scripts/ci-parity.sh`：API tests 固定禁用 Go test cache（`-count=1`），缺少 `API_TEST_DATABASE_DSN` 或 `API_TEST_NATS_URL` 会立即失败，不会静默 `SKIP` PostgreSQL/NATS integration tests。GitHub 的 API test matrix 按 `standard/race × shard` 执行，每片拥有独立 PostgreSQL 与 authenticated JetStream；Migration Gate 专注 migration/schema/River/runtime privileges，不重复运行 snapshot-backed test suite。Linux 直接运行 Web smoke；macOS 等其他系统通过 Docker 中固定版本的 Playwright Linux image 运行，因此需要 Docker 已启动。container 使用只读源码副本，不会改写宿主的 `node_modules`。

只复现单项时可运行：

```bash
TEST_MODE=standard TEST_SHARD=0 TEST_SHARD_COUNT=1 ./scripts/ci-parity.sh api-test
./scripts/ci-parity.sh web-smoke
```

普通 `go test ./...` 仍允许在未配置测试数据库或 NATS 时跳过对应 integration tests；严格 fail-closed 仅由 parity/CI mode 启用。完整 lint、build、migration、sqlc drift 和 vulnerability checks 仍由上表与 GitHub 的独立 jobs 执行。

修改 `apps/api/internal/db/queries`（query-only，不动 fragment/manifest）后：

```bash
cd apps/api
go tool sqlc generate
git diff --exit-code -- internal/db/sqlc
```

修改 domain fragment 或 `manifest` 后不要手跑 `sqlc generate` 直写 `schema.sql`：走 `schema-release` canonical lifecycle，先用 `--write-artifact` 生成 `schema.sql`（内部已校验 sqlc 生成一致），再运行默认 check；CI 和 release gate 只运行默认只读 check。

Rewards 是 pre-launch clean break：RewardCode 由单一 canonical migration 创建。API schema 由 Atlas versioned migrations 管理；若本地数据库不符合当前 Atlas history，请显式重建 Fresh Database，不要补空 migration、手删 Ledger 或尝试兼容性回放。

## 常用入口

### Canvas workflow

| 入口                                                                                                         | 用途                                |
| ------------------------------------------------------------------------------------------------------------ | ----------------------------------- |
| [CanvasPage.vue](./apps/web/src/features/canvas/ui/pages/CanvasPage.vue)                                     | Canvas route page                   |
| [executionContract.ts](./apps/web/src/features/canvas/core/execution/executionContract.ts)                   | 前端 workflow feature contract      |
| [generationIntent.ts](./apps/web/src/features/canvas/session/image-workflow/generationIntent.ts)             | Prompt/node execution orchestration |
| [useCanvasImageWorkflow.ts](./apps/web/src/features/canvas/session/image-workflow/useCanvasImageWorkflow.ts) | Image node direct actions           |
| [workspaceEventStream.ts](./apps/web/src/api/workspaceEventStream.ts)                                        | Task SSE events                     |
| [features.go](./apps/api/internal/workflow/features.go)                                                      | 后端 canonical feature contract     |

### Task runtime

| 入口                                                                                               | 用途                     |
| -------------------------------------------------------------------------------------------------- | ------------------------ |
| [canonical_intake.go](./apps/api/internal/tasking/application/task/canonical_intake.go)            | Task submission intake   |
| [river_queue.go](./apps/api/internal/tasking/infrastructure/queue/river_queue.go)                  | River enqueue adapter    |
| [worker.go](./apps/api/internal/runtime/interfaces/worker/river/worker.go)                         | River worker adapter     |
| [worker.go](./apps/api/internal/tasking/interfaces/worker/worker.go)                          | Worker task processing   |
| [request.go](./apps/api/internal/runtime/interfaces/worker/providers/request.go)                   | Provider request shaping |

### Prompt config

| 入口                                                                                                  | 用途               |
| ----------------------------------------------------------------------------------------------------- | ------------------ |
| [AdminPromptConfigPage.vue](./apps/web/src/features/admin/prompts/ui/pages/AdminPromptConfigPage.vue) | Admin prompt UI    |
| [engine.go](./apps/api/internal/prompts/engine.go)                                                    | Prompt rendering   |
| [docs/runbooks/system-prompts.md](./docs/runbooks/system-prompts.md)                                  | 运营模板和变量规则 |

### Auth

| 入口                                                                     | 用途                       |
| ------------------------------------------------------------------------ | -------------------------- |
| [middleware.go](./apps/api/internal/identity/auth/middleware.go)         | API auth middleware        |
| [service_client.go](./apps/api/internal/identity/auth/service_client.go) | Go -> Auth internal client |
| [index.ts](./apps/auth/src/index.ts)                                     | Auth service routes        |
| [client.ts](./apps/web/src/core/auth/client.ts)                          | Web auth client            |
| [userSession.ts](./apps/web/src/features/auth/session/userSession.ts)    | User session store         |
| [docs/runbooks/auth-operations.md](./docs/runbooks/auth-operations.md)   | Auth operation rules       |

### Credits

| 入口                                                                            | 用途                                                                                       |
| ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| [composition.go](./apps/api/internal/credits/composition.go)                    | Grant/Hold/Capture/Release/Refund Command seam 与 caller-owned transaction adapter         |
| [service.go](./apps/api/internal/credits/application/reconciliation/service.go) | 自动账本检查、Run history 与全局 Finding queue；Stale Hold 固定 24 小时且不接受 Admin 配置 |
| [CONTEXT.md](./apps/api/internal/credits/CONTEXT.md)                            | Credits domain language 与 invariants                                                      |
| [credits-million.md](./docs/benchmarks/credits-million.md)                      | 显式百万流水 harness 与参考结果                                                            |
| [web-performance.md](./docs/benchmarks/web-performance.md)                      | Web bundle size guard 与 Canvas interaction benchmark                                      |

### Rewards

| 入口                                                                         | 用途                                                                    |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| [service.go](./apps/api/internal/rewards/application/redemptions/service.go) | RewardCode、Redemption create/list/redeem/history 与 Credits Grant 编排 |
| [CONTEXT.md](./apps/api/internal/rewards/CONTEXT.md)                         | Rewards Domain Language 与 invariants                                   |

### Billing

| 入口                                                                     | 用途                                                                                             |
| ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| [store.go](./apps/api/internal/billing/infrastructure/postgres/store.go) | 全局 SKU Catalog、versioned pricing 与 Quote seam                                                |
| [CONTEXT.md](./apps/api/internal/billing/CONTEXT.md)                     | Billing domain language、SKU namespace 与 snapshot invariants                                    |
| [settle.go](./apps/api/internal/tasking/workspacetasks/settle.go) | Task Quote、Readiness、Submit Hold 与 terminal settlement 编排 |

Workflow feature、Task payload 和媒体输入的 canonical contract 见 [DESIGN.md](./DESIGN.md) §2；新增或修改能力时同步前端 execution contract、OpenAPI、Go workflow validation、Admin prompt config 和 runbook。

## Generated Code

Generated API:

- `apps/web/src/api/generated-query` 由 Orval 生成，不手动编辑。
- Route pages 和 components 不直接依赖 generated API。
- 业务访问收口到 feature resource、feature adapter 或 horizontal API adapter。

sqlc:

- `apps/api/internal/db/sqlc/*.go` 不手动编辑。
- query-only 变更运行 `go tool sqlc generate` 并以 `git diff --exit-code -- internal/db/sqlc` 确认 drift；fragment/manifest 变更走 `schema-release` canonical lifecycle，不要手跑 `sqlc generate` 直写 `schema.sql`。
- Atlas migrations 位于 `apps/api/internal/db/migrations/`，由 `apps/api/atlas.hcl` 与 `atlas.sum` 管理。
- Atlas migration 生成使用 `atlas migrate diff`；完整性与 Fresh replay 使用 `scripts/atlas-migration-check.sh`。CI 先在隔离 database 执行 `atlas migrate validate`，再在集成 database 执行 Fresh apply 与 incremental idempotency apply；不要把 Atlas Pro-only 的 `atlas migrate lint` 加入流水线。
- API schema 由独立 `api-migrate` image 执行 Atlas versioned migrations，再执行 `finalize-migrate` 完成 River 与 runtime privilege setup。Pre-launch rebaseline 只能原子删除旧 SQL chain 并新增一个更高版本 baseline，随后重建 Fresh Database；不得手工编辑 Atlas revisions 表。
- 新增数据库表及相关 sequence 时，必须同步 [runtime privilege policy](./apps/api/internal/platform/database/privileges/runtime.go) 与 [启动权限断言](./apps/api/internal/runtime/composition/wiring/database_wiring_privilege_test.go)，按 runtime 实际用途授予最小权限。
- Schema source 的唯一 lifecycle 是 `schema-release`：修改 domain fragment 或 manifest 后先用 `--write-artifact` 生成 `schema.sql`，再运行默认 check；CI 和 release gate 只运行默认 check。
- clean-break release 只支持 Fresh Database。旧数据库必须先导出需要保留的数据，再重建并按当前 baseline 初始化；不能使用旧 migration binary 做跨 baseline 升级或回滚。
- `api-migrate` 在 `atlas migrate apply` 之前会比对数据库记录的 Atlas revisions 与镜像内 `internal/db/migrations/` 的版本集合。出现缺失版本（错误的 image digest、回滚到旧 image、migration 文件被改名或删除、CI 变量错配）时 migrator **不会**自动清库，而是以 `clean break reset is not allowed` 失败，并列出缺失的 version。确认是预期的 pre-launch clean break、且已导出需要保留的数据后，才在 migration 容器上设置 `ATLAS_CLEAN_BREAK_ALLOW_RESET=true` 重跑：这是唯一的 opt-in 开关，值非法时按 `false` 处理并计入 `Config.InvalidEnv`。drop 前会把 `current_database()`、缺失 version 与 public schema 行数摘要写入结构化日志。

## 环境变量要点

本地 API/Auth 至少确认：

| 变量                                                          | 说明                                                                                               |
| ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `PG_HOST` / `PG_PORT` / `API_PG_USER` / `API_PG_PASSWORD` / `AUTH_PG_USER` / `AUTH_PG_PASSWORD` / `PG_DB` | 当前 API/Auth 进程的 runtime PostgreSQL 连接；runtime role 不拥有 schema；API 与 Auth 用各自的 `*_PG_USER`/`*_PG_PASSWORD`，不再共用同一个旧库密码变量名 |
| `PG_MIGRATION_USER` / `PG_MIGRATION_PASSWORD`                 | 仅 API/Auth migration command 使用的 schema owner；必须与 runtime user 不同                         |
| `AUTH_INTERNAL_URL`                                           | Go 调用 Auth service 的内网地址（`API_INTERNAL_URL` 是反方向：Auth/Realtime 调用 Go）               |
| `AUTH_INTERNAL_KEY`                                           | API/Auth internal endpoint 共享密钥                                                                |
| `APP_SECRETS_ENCRYPTION_KEY`                                  | API key 和 storage secret 静态加密主密钥                                                           |
| `REWARDS_CODE_HMAC_KEYS`                                      | Rewards Code current/previous keyed-HMAC JSON registry；key 至少 32 bytes，不能放入 Admin settings |
| `BETTER_AUTH_SECRET`                                          | Better Auth 签名密钥，生产必须高熵                                                                 |
| `PUBLIC_ORIGIN`                                               | Auth 对外基准 URL与 trusted origin                                                                 |

Auth 使用相同的 owner/runtime separation：长期运行的 Auth 读取 `AUTH_PG_USER=ideaboard_auth`，`npm run migrate` 通过 `PG_MIGRATION_USER` 临时使用 owner，并在完成后只授予 Better Auth tables 权限。本地 migration credential 只放在已忽略的 `docker/migration.env`，不要复制进 API/Auth runtime `.env`。

Docker compose / deploy 层的 `PUBLIC_ORIGIN` 是公开 Origin 的 canonical source；后端公开 API 地址恒等于 `PUBLIC_ORIGIN`（`API_BASE_URL` 已删除，无 override）。API runtime 仍分别生成 Web 跳转/分享链接与 Stripe Webhook/服务端素材地址；`PUBLIC_ORIGIN` 缺失或无效时拒绝启动。生产部署细节见 [docs/runbooks/deployment.md](./docs/runbooks/deployment.md)。

API 在独立 metrics listener 暴露 Prometheus `/metrics`；本地宿主只监听 `127.0.0.1:6000/9091`，production Compose 内监听 `0.0.0.0:6000/9091`（无独立 env 开关）；宿主机经 compose 以 `127.0.0.1:${API_METRICS_HOST_PORT:-9091}:9091` 映射。该 listener 只用于本机或私有监控网络，不得接入用户 API 反代或 Cloudflare。

轮换 `REWARDS_CODE_HMAC_KEYS` 时先部署 new current + old previous；新 Code 只使用 current，lookup 同时验证两者。任何 persisted RewardCode 的 primary `key_version` 未配置时，API startup 都会 fail closed；disabled、expired 或未兑换都不例外。只有数据库中不再存在依赖 old version 的 RewardCode 时，才能移除 previous。

## 改动 Checklist

### 改 Canvas 或 workflow

1. 更新前端 execution contract。
2. 更新 Go workflow validation/helper。
3. 检查 Admin prompt config 展示和变量。
4. 确认 prompt 变量没有承载媒体 URL、mask 或 provider 参数。
5. 跑 Web tests/build 和 API tests。

### 改 task submission/runtime

1. 不破坏 submit 的 transaction 边界。canonical 规则只写在 [DESIGN.md](./DESIGN.md) §3
   （Quote snapshot、Credit Hold、task insert、River enqueue 同一 transaction；终态与
   Capture/Release 原子提交；event 只能在 commit 后发布），本文不复述细则。
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
├── api/                      # HTTP exit, transport contracts, generated clients
├── core/                     # horizontal primitives and infrastructure runtime
├── theme/                    # tokens, atoms and UI presets
├── lib/                      # horizontal utilities
├── components/               # standard UI primitives
├── features/                 # business modules
│   ├── account/              # user account and billing settings
│   ├── admin/                # Admin vertical contexts
│   ├── auth/                 # login/register views
│   ├── board/                # Board bounded context: domain, sync, persistence, catalog
│   ├── canvas/               # Canvas Workspace
│   ├── legal/                # legal pages
│   ├── library/              # asset library
│   ├── search/               # search features
│   └── workspace/            # workspace routes, layout, shell (/user/* URLs)

apps/api/
├── cmd/                      # service entrypoints
├── internal/platform/        # config, database, response, safehttp
├── internal/runtime/         # bootstrap, HTTP routes, worker, webdist
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
