# 后端目录与命名规范化方案

范围：`apps/api/internal` 全量约 600 个 `.go` 文件。评判以成熟 SaaS / Go 惯例为准；`DESIGN.md §6` 只写了前端规则，凡与 Go 惯例冲突处以本方案为准、后续回写规则。

## 0. 先定规则（DESIGN.md §6 修订方向）

经四片审计确认，`§6` 逐字套到 Go 会误判，必须先修规则再治代码：

1. `§6` 标注 `frontend-only`：`kebab-case`、`PascalCase.vue`、`index.ts` 唯一出口、`dependency-cruiser` 均为前端专属。`kebab-case` 在 Go 非法（包名禁连字符），不得套用。
2. 新增 Go 命名节，核心五条：
   - `package` 名等于目录短名；跨域同名短包（`domain`、`events`、`infrastructure`、`tests`）必须改名，`import` 别名打架是 P0。
   - `foo/foo.go` 是正统 Go 写法（`config.Config` 式调用），不算 stutter，不改。真正的 stutter 只限泛型名：`ports/ports.go`、`testhelpers/testhelpers.go`、`message.go × N`。
   - 测试默认 `foo.go ↔ foo_test.go` 同名配对；`architecture`、`carrier`、`smoke`、`fixtures`、`boundary`、`tests/` 集成目录测试列白名单，不强制配对但禁新后缀（`_db`、`_pg`、`_v2`、`test_` 前缀一律禁）。
   - 存储统一 `postgres`（目录 + 文件），禁 `pg`；禁版本号进文件名（`v1_`、`_v2`、`modelsdev` 类拼写）。
   - 生成文件（`sqlc/*`、`schema.sql`）与 Atlas migrations 只许按既有 lifecycle 动，不在本次范围。

## 1. P0：包名与路径特例（先做，阻塞脚手架与审计脚本）

| # | 现状 | 改法 |
|---|---|---|
| 1 | `interfaces/http` 包名分裂：`billing`、`searchcatalog` 下用 `package httpapi`，`credits`、`rewards`、`payments`、`identity/users` 下用 `package http` | 统一为 `package http`（短名即上下文；`httpapi` 是 http 拆分前的历史名）。`billing` 3 文件 + `searchcatalog` 7 文件改包名，调用方 `import` 别名同步 |
| 2 | `tenancy/postgres/` 直挂 | 迁入 `tenancy/infrastructure/postgres/`，与其他五域同构；`memory.go` 去向一并定（留顶层或进 `infrastructure/memory`，二选一） |
| 3 | `rewards/infrastructure/credits` 包 `creditsadapter` vs `identity/users/infrastructure/credits` 包 `credits` | 同路径模式统一包名（二选一，以 `credits` 为准） |
| 4 | `payments/infrastructure/events` 包 `paymentevents` | 改 `package events`（若 import 冲突则目录改 `eventhub`，毕竟生造连词更差） |
| 5 | `runtime/infrastructure/{config,http,nats}` 包 `configinfra/httpinfra/natsinfra` vs 目录名 | 包名改回 `config/http/nats`（短名；如与标准库 `net/http` 撞 `import` 再加别名，不在包名里加后缀） |
| 6 | `media/errors` 包 `mediaerrors`、`tasking/interfaces/http/statistics` 包 `statisticshttp` | 前者改 `package errors`（调用方加别名），后者改 `package http` 与其他域一致 |
| 7 | `runtime/interfaces/worker/providers/domain` 包 `domain` 撞 `runtime/domain`；`providers/infrastructure` 包 `infrastructure` 过于通用；`runtime/interfaces/events` 包 `events` 撞 `internal/events` | 依次改 `dialectdomain`→建议 `dialects` 合并或 `providerdomain`；`providers/infrastructure` 改名或并入上层；`interfaces/events` 改 `eventbus`/`eventiface` 二选一 |

## 2. P1：四个平铺超限目录拆分（每目录独立 PR）

| 目录 | 规模 | 拆法 |
|---|---|---|
| `runtime/composition` 32 文件 | `control_*` 6 + `*_wiring` 5 + `*_migration` 2 + 门禁测试混放 | 拆 `control/`、`wiring/` 子包，`composition` 只留装配根（`app/container/cli`）；`event_bus_test.go` 归位到有 `event_bus.go` 的包 |
| `internal/events` 33 文件 | `webhook_*` 14 + outbox + nats + store + status/visibility | 拆 `webhook/`、`outbox/`、`nats/`，`status/visibility` 随 `v1_envelope` 留根或单列 `status/`；`postgres_store`（345 行）、`webhook_store`（579 行）不动逻辑只搬家 |
| `payments/infrastructure/postgres` 25 文件 | `admin_reads_*`、`refund_*`、`dispute_*`、`readmodel*`、`order` 混装 | 按聚合拆 `adminreads/`、`refunds/`、`disputes/`，根留 `repository/order` |
| `tasking/interfaces/worker` 28 文件 | `workspace_task_` 簇 30 文件（含 `application/runtime` 5 件同名前缀） | 先去前缀（`workspace_task_` 在包内零信息，`worker` 包内直接叫 `claim.go/execution.go/routing.go`），再按 `claim/execution/settle` 聚合；测试 20+ 切面随实现走，`_integration/_pg` 后缀按白名单规则处置 |
| 结构性平铺（未超限但无分层） | `aigateway` 18、`usage` 9、`billingrecords` 6 全平铺 | 补 `application/domain/ports` 最小分层，只挪文件不改逻辑 |

## 3. P2：机械重命名（批量脚本 + 全测，低风险）

1. 泛型自重复：`ports/ports.go ×2 → contracts.go`、`testhelpers/testhelpers.go → helpers.go`（或包改名）、`principal/principal.go → types.go`、`reconciliation/reconciliation.go → service.go`、`settings/runtime_settings.go → runtime.go`、`database/database.go → store.go`（以实现为准）、`metrics/metrics.go`、`events/events.go`、`retention/retention.go` 保留（`foo/foo.go` 正统，不动——与 stutter 真违规区分开）。
2. `message.go × 16`、`doc.go × 7`：具名化（`errors.go`、`docs.go` 或并入同包主文件），`handler/message` 例外保留。
3. 单复数/拼写：`orgs` 已改 `organizations`；`modelpool → model_pool` 已做；`modelsdev.go → models_dev.go`；`paramkeys/keys.go` 全小写压缩改 `keys.go` 保留或 `param_keys.go`；`fragments/organization.sql ↔ queries/organizations.sql`、`workspace.sql ↔ workspaces.sql` 跨层对齐复数；`admin_` vs `administration` 统一全称。
4. 后缀：`_db_test ×2`（`privileges_db`、`service_db`）去后缀并入同名测试；`board_access_outbox_pg` 等 `pg` 改 `postgres` 或去掉（同目录已是 `postgres/` 则直接去掉）。
5. 版本文件名：`searchcatalog/v1_workspace.go → workspace.go`、`v1_vision_upload_test.go` 并入、`principal_v2_test.go` 去 `_v2`。
6. `postgres` vs `pool`：`river/pool_test.go` 等 orphan 先补实现或并入（见 P3）。

## 4. P3：测试命名收敛（规则先行，逐域合并）

1. 白名单化（写进 Go 命名节，不再判违规）：`architecture_test.go × 15`、`carrier_test.go × 7`、`smoke/fixtures/boundary/seam`、`tests/` 三个集成目录（`payments 14`、`rewards 14`、`users/tests 19`，强制 `fixtures` 单一入口）。
2. 真 orphan 合并：`boards` 14（`access_matrix/access_signals/board_media_refs/membership_*/policy_boundary/preview_catalog/write_authorization` 按 `policy/access/preview` 聚合）、`workspacetasks` 9、`worker` 16（随 P1 前缀去除自然归位）、`domain/routing_observability`、`infrastructure/postgres/claim_attempt/envelope_board`、`statistics/handler_sse/support`、`renderer_body/driver_root/checksum`、`access_pg/document_acceptance_pg/status`、`fixtures/status/source_contract`。
3. 内外测试包统一：`credits/payments` 内部包 vs `rewards/users` 外部 `_test` 包，二选一（建议外部 `_test`，与 `lifecycle/orchestration/domain/ports` 已有外部包对齐）。
4. `tests/` 19/14/14 堆积：不强制配对，但 `fixtures_test.go × 3`、`helpers_test.go × N` 合单入口，`admin_http_helpers/reconciliation_helpers/code_service_helpers` 按资源改名。

## 5. 平台层专项

1. `db/schema/schema.sql`（5922 行）补生成头注（`fragments → assembly → schemarelease` 产物，手改禁区），与 sqlc 硬约束对齐。
2. `platform/database/privileges/` + `database/runtime_privileges.go` 双层：子包并入父级或父文件下沉，二选一。
3. `platform/outbound` vs `outboundhttp` vs `safehttp` 三概念、`apperror` vs `errorrender` vs `validation/error` 三概念：各合并为一（`safehttp` 留出口，`errorrender` 留渲染），废弃另两处（先改规则再搬）。
4. `settings/` 12 文件主题发散：按 `branding/outbound/policy` 拆小包或单列文件。
5. `assembly.go` 内 `requiredFragmentNames` 15 vs manifest 21 子集关系补断言或注释。

## 6. 不做事项

`foo/foo.go` 正统命名、`sqlc/*`、`migrations/*`、`atlas.sum`、`schema assembly` 链条、`docs/adr/*` 历史引用、`cmd/*/main.go` 的 `package main` 惯例，均不动。

## 7. 执行顺序与验证

P0（包名+路径+规则回写）→ P1 四目录逐个 PR（composition → events → payments-postgres → worker）→ P2 批量 rename → P3 测试收敛 → 平台专项。每步 `go build ./... + go vet + go test` 对应包，回滚一律 `git mv` 逆操作。`internal/runtime` 零导入（生产+测试）在四片已确认干净，每步复查。
