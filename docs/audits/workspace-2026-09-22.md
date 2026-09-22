# 工作区分批审计记录

## 范围与基线

- 原始基线：`f0e923ca23a75905f1aec998e3ce71589e62f0ce`。
- 审计 staged、unstaged 与 untracked 的最终工作区内容。初扫 Git tracked diff 为 548 个文件；MCP 返回 673 个变更路径，其路径口径包含新增文件和 rename 两端，不能直接等同 Git 文件数。
- 每批执行 Standards / Spec 双轴复核、必要修复及相关验证，再提交本批明确的文件列表。保留其他批次原有 staged 内容。
- codebase-memory MCP 用于符号与调用影响分析，Git diff 和文件内容用于核对实际实现。已确认 MCP 泛型 `DecodeCursor` 的调用追踪漏报，使用文本检索补齐。
- 当前没有与全部工作区变更对应的独立 Issue Spec。已知契约来自 `DESIGN.md`、相关 `CONTEXT.md`、ADR、导出接口注释与原始基线行为；这不代表全部产品意图已获验证。

## 批次计划

| 批次 | 范围 | 核心验证 | 进度 |
| --- | --- | --- | --- |
| 1 | Platform 日志脱敏、HTTP 错误、分页和 Accepted response | 边界、泄露、请求关联、独立 package 测试 | 已审计和修复，验证结果见下 |
| 2 | PostgreSQL / pgx / sqlc、Schema、migrations、runtime privileges | 类型映射、事务、权限、migration replay、生成一致性 | 2A 已提交；2B 配置契约子批已提交，runtime/migration 仍在审计 |
| 3 | AI Gateway、Workflow、accountpool 退役 | 出口限制、请求方言、模型映射、配置 | 待审计 |
| 4 | Task submission、River、execution、retention、statistics | 幂等、超时/重试/取消、终态与扣费原子性 | 待审计 |
| 5 | Billing、Credits、Payments、Rewards、Identity | 金额、账务状态机、身份生命周期、错误契约 | 待审计 |
| 6 | Media、Library、Search、Boards、Prompts | 所有权、资源回收、分页、分享、输入验证 | 待审计 |
| 7 | Frontend、OpenAPI、generated API | DTO 契约、管理页、用户流程、build/typecheck | 待审计 |
| 8 | Composition、监控、CI、文档与遗漏路径复核 | 集成装配、发布门禁、完整回归、文件覆盖清单 | 待审计 |

后续按真实依赖调整提交边界。尤其 pgx/sqlc 类型迁移与 Task/Gateway 变更交叉：先审清共享文件与调用方，必要时把依赖闭包并入同一批；禁止为凑批次提交缺少配套代码的状态。共享生成文件随其输入一起核验，通过工具生成。

## 第一批：公共日志与 HTTP 基础契约

代码范围：`apps/api/internal/platform/{applog,errorrender,pagination,response}` 中的 9 个新增/变更文件。

### Standards

发现并修复 5 项：

1. **P1：日志脱敏死循环。** `password=abc` / `token=abc` 被替换后保留前缀，从头重新搜索会永远命中同一个位置。用例在原实现中触发 3 秒 test timeout，堆栈定位到 `scrubSensitivePairs`。改为对原输入单向扫描，并覆盖多值、空值、已脱敏值、quoted DSN、Unicode 和无效 UTF-8。
2. **P1：error 属性泄露凭据。** 原实现只处理 string；`logger.Error("dial failed", "err", errors.New("postgres://app:password@db/app"))` 在 JSON/Text 两种输出中均泄露密码，回归用例已复现。现在 error 格式化结果经过同一脱敏路径，LogValuer 只解析一次；日志 message 也处理已知凭据模式。
3. **P2：敏感 Group 绕过脱敏。** 敏感父 key 的 Group 与 `WithGroup` 未传递敏感上下文。现在逐层遮盖其成员，保持父 logger 和此前已绑定的非敏感属性不变。尚未发现实际业务使用敏感 Group，此项保障公共 Handler 契约。
4. **P2：空密码 URL 中断后续扫描。** `postgres://u:@db postgres://u:secret@db` 原实现遇到第一个空密码就停止，泄露后一个 URL。现在继续检查后续 URL。
5. **P2：密码中的 @ 导致部分泄露。** `postgres://u:p@ssword@db` 原实现只遮住 `p`。现在按当前 URL authority 内最后一个 `@` 分隔 userinfo，与 Go `net/url` 的解析约定一致，避免吞入其他 URL 或路径。

未发现需要额外重构的阻塞性 code smell。

### Spec

发现并修复 2 项：

1. **P2：5xx 异常详情失去请求关联。** `RenderDefault` 未延续旧 `response.InternalError` 的 `request_id` / method / path 记录；Boards 等调用方迁移后无法通过响应 ID 定位异常详情。恢复这些字段，并验证响应和日志使用同一个 ID，内部错误只留在日志。
2. **P3：cursor 接受 JSON null。** `DecodeCursor` 的注释承诺 single JSON object，但 `bnVsbA` 可解码为零值 struct。增加顶层 object 检查。现有 Library、Payments 的三个解码调用均进一步检查必填字段，本轮未确认存在实际 API 绕过。

AcceptedDTO 的 202、numeric code、accepted、request_id 与 data 契约保持一致；另验证错误匹配表在多个 sentinel 同时命中时遵守表顺序。

### 验证与边界

- 工作区：四个 package 的 `go test -race -count=1 -timeout=60s` 和 `go vet` 通过。
- 独立快照：以原始 HEAD 加上仅本批 9 个代码/测试文件构造，用同样的 Race 测试、`go vet` 和 package build 验证独立提交。
- 本批不执行 migration、外部服务写入或部署，也不代表其余工作区已通过审计。
- 脱敏是已知模式过滤，覆盖 sensitive attribute keys / slog Groups、string/error/message 的既定凭据模式；不能把任意对象内部字段或任意文本当成已经全面脱敏。调用方仍须避免记录原始凭据对象。
- 本批无数据库变更。后续需要撤销时，在干净工作区使用该批 commit 的 `git revert` 生成反向提交；不通过 reset 丢弃其他工作。

查找本批提交：`git log --oneline -- docs/audits/workspace-2026-09-22.md`。

## 第二批：PostgreSQL / pgx / Schema Release（2A）

本次提交只包括 Schema Release 工具。事务、runtime pool、AccountPool retirement migration、testdb nested module 和全部 generated sqlc 差异继续留在 2B，待连接池 idle 语义和跨层调用方闭包完成后再提交。

### Standards

发现并修复 1 项：

1. **P2：Schema Release 错误包含 DSN。** command runner 原错误拼接全部 argv，Atlas URL 可能包含数据库密码。错误只保留 executable 和 exit error；stdout/stderr 由上层按需返回，未把 URL 放入错误文本。

### Spec

发现 0 项；2B 有一项待提交修复：

1. **2B 待提交，P1：AccountPool retirement 漏清理 retired 密文。** `api_account_secrets` 的历史轮换会保留 `secret_payload`、`fingerprint` 和 `key_version`，但原 migration 只匹配 `status <> 'retired'`，正常 retired 行因此永久保留凭据材料。工作区已改为清理全部仍有材料的行，保留历史行/FK，并在隔离 Fresh database 中插入 active 与 retired 两类 fixture 验证结果为零。

另验证：Board share `token_secret` 通过 repository 的 Encrypt/Decrypt seam；新增 execution-attempt constraints、Schema fragments、snapshot 和 migrations 的静态对象/FK 对齐；`atlas.sum` 由 Atlas 根据实际 migration 内容更新，其中旧 migration 的既有 checksum drift 被一并校正。

### 验证与边界

- `go test ./internal/db/schemarelease ./internal/db/schemacontract -count=1` 通过；Schema Release integration cleanup test 以隔离 PostgreSQL 17 通过。
- 独立 PostgreSQL 17 临时实例：Atlas 9 migrations Fresh replay、Schema Release 默认检查（snapshot/catalog 对比、sqlc generation drift check）通过；active/retired secret purge fixture 已验证但属于待提交 2B。
- 2B 会单独验证 `cd apps/api/internal/testdb && go test ./...`、pgx transaction/pool 资源语义、完整 generated sqlc 一致性及 migration purge。
- rollback/fallback：本批代码提交可用 `git revert <batch-2a-commit>` 回滚。2B migration 中的 DROP 和 secret purge 仍是 ADR 已批准的不可逆操作，不能通过 runtime image rollback 恢复，发布前需保留备份并走 Fresh Database/cutover 流程。
- 未执行线上数据库写入；临时数据库位于 `/tmp/ideaboard-audit-b2.*`，完成后可停止/删除该临时实例。

## 第二批补充：数据库配置契约（2B-0）

本子批仅提交 `platform/config` 的连接池/Task retention 配置、环境样例与对应测试。它不提交 `platform/database/postgres.go` 的 pgx runtime 接入，也不提交 sqlc、testdb 或 destructive migration；这些文件仍有跨层依赖，必须在后续闭包中一起验证。

### Standards / Spec

- 修复 invalid `DB_POOL_MAX_IDLE` fallback 未按有效 open 上限 clamp 的边界错误。
- 修复 `TASK_RETENTION_DAYS` 未加入测试环境清理列表导致的顺序污染。
- 增加 runtime + River 连接池总预算上限（80），为默认 PostgreSQL `max_connections=100` 保留 migrations/CLI/health tooling headroom；生产 `InvalidEnv` 仍 fail-fast，开发环境保留 warning/default 行为。
- `docs/runbooks/deployment.md` 已定义变量范围与 idle clamp 契约；配置实现与该文档一致。

### 验证与边界

- `cd apps/api && go test ./internal/platform/config -count=1` 通过。
- 本子批未证明 pgx pool 实际 runtime idle 回收；当前 `postgres.go` 仍属于未提交迁移闭包，不能把 `DB_POOL_MAX_IDLE` 视为已在运行时生效。

## 第三批补充：Outbound HTTP 可靠性与 SSRF 边界

本子批提交 `platform/outboundhttp` 标准客户端及 `safehttp` policy error seam。它覆盖所有已迁移调用方共用的 retry、timeout、Retry-After、request-body replay、metrics 和 allowlist 约束；不包含各业务 context 的 provider 行为审计。

### Standards / Spec

- 双轴复核发现并修复带 `http.Request.GetBody` 的 POST 在 503/429 retry 后发送空 body 的问题。
- 修复 nil context 触发 `context.WithTimeout` panic，以及读取不可重放 body 出错时未关闭 reader 的 Resource Cleanup 问题。
- `outbound_retries_total` 仅在确实安排下一次 attempt 时递增；最后一次失败和 `MaxAttempts=1` 不再误计 retry。
- `safehttp.ErrPolicyViolation` 将 URL/SSRF 的确定性拒绝与 transient transport error 区分；policy rejection 立即 fail closed，不重试。
- 提交后复核又修复 request-body reader 的关闭、SSRF prevalidation 失败时 fresh body 的关闭、不可重放 body 受 call timeout 约束，以及 Retry-After/backoff duration overflow。

### 验证与边界

- `cd apps/api && go test ./internal/platform/outboundhttp ./internal/platform/safehttp -count=1 -race` 通过。
- `cd apps/api && go vet ./internal/platform/outboundhttp ./internal/platform/safehttp` 通过。
- 本批只验证共享客户端与安全边界；provider-specific status mapping、Cloudflare gateway billing 及业务 retry budget 仍在后续批次审计。
- 以上补丁作为同一 outbound 子批的 follow-up commit 提交；若 reader 无法响应 Close，后台读取 goroutine 仍可能延迟结束，这是 Go `io.Reader` 无强制取消能力的边界。

## 第三批补充：Workflow Gateway route registry（3A）

本子批只提交 `workflow/gateway_path.go`、`workflow/provider_registry.go` 及纯函数测试。它把 provider namespace、route source、chat/image 判定和 Gateway reachability classification 收敛到无 I/O seam，供 preview 与 worker 后续共同使用。

`internal/aigateway` 的 config/detect/model/handler 仍未提交：其 catalog/mapping audit 写入依赖尚未闭合的 pgx `settings/sqlc` migration；当前已修复但必须随数据库调用方闭包一起验证。

### 验证

- 以当前 HEAD 加仅 3A 文件构造的独立快照中，`go test ./internal/workflow -count=1` 通过。

## 第四批前置：Idempotency primitive（4A）

本次先提交共享 `platform/idempotency` primitive，不提交尚未闭合的 boards/task/rewards consumers。MCP `search_graph`/`codegraph_explore` 追踪了 `HashRequest`、`Store`、`replayBoardWrite` 及 task/reward HTTP 调用链；Standards / Spec 双轴复核确认 consumer 仍有独立的 contract 缺陷，留待 4B。

### Findings and scope boundary

- P1（4B 待处理）：Rewards `Redeem` 只解析 header、随后丢弃 key，不能把 key 绑定到 Code/request，也不能在同 key 不同 Code 时返回 conflict。
- P1（4B 待处理）：Boards 的 `Lookup` → service side effect → `StoreResult` 非原子；相同 key 并发请求可能同时执行副作用。
- P2（4B 待处理）：Task cancel 只校验 header 存在，没有按 `task_id` 做 request binding 或 replay。
- P2（4B 待处理）：Rewards HTTP 删除既有 redemption metrics 文件，属于与本 primitive 无关的 observability 回归，未纳入本提交。
- 4A 修复 `HashRequest` 在包含 NUL 的 caller-controlled parts 上使用 length-prefix framing，同时保持普通输入的历史 digest 稳定，避免影响已有 durable request hashes。
- 4A 让导出的零值 `Store` lazy-init，并新增 `Store.Do` 的 same-key in-flight coordination：同 hash waiter 获得 leader 的结果/错误，不同 hash 在副作用前 conflict。现有 consumers 尚未迁移到 `Do`，因此 4A 不宣称端到端并发幂等已完成。

### Verification

- `cd apps/api && go test ./internal/platform/idempotency -count=1 -race` 通过。
- `cd apps/api && go vet ./internal/platform/idempotency` 通过。
- 提交：`a7f3e32c fix(idempotency): harden fingerprints and concurrent replay`。

## 第四批补充：Reward Redeem consumer（4B-1）

`#771` 的原始 issue/评论明确要求 Reward Redeem 强制幂等键，并允许通过 durable `user+code` 事实 replay；本轮复核发现仅依赖该 natural fact 不足以绑定 caller key。MCP `search_graph` 追踪了 `UserHandler.Redeem`、rewards composition/service、前端 resource 和相关 HTTP tests。

### 修复

- `UserHandler.Redeem` 现在把 `(user, key, normalized code)` 绑定到 process-local `Store.Do`。同 key 同 code 直接 replay 首次结果并标记 `replayed=true`；同 key 不同 code 在副作用前返回 `409 reward_code_idempotency_conflict`。
- `Idempotency-Key` 解析和 replay 判定先于 redemption rate limiter，合法 replay 不会再次消耗 quota 或被错误渲染成 `429`；只有真正的新副作用尝试进入 limiter。
- Rewards HTTP 失败统一通过 machine-error mapping；补充错误码、key conflict、replay-before-limit 和独立 response fixture 测试。

### Verification and boundary

- 当前工作区：`go test -race` 覆盖 `platform/idempotency`、`boards`、`tasking/interfaces/http/task`、`rewards/interfaces/http`、`rewards/tests`，并对相关 package 执行 `go vet`，全部通过。
- Clean archive（只含已提交代码）中 Rewards HTTP package/tests 的 `go test -count=1` 和 `go vet` 通过。
- 提交：`c2daf1c8`、`295d57de`、`9413b36e`。
- Replay 仍是单进程 cache；跨实例/重启的 Reward key→request durable fact 尚未建立，后续必须与 Rewards schema/service transaction 一起审计，不能把本批视作分布式幂等完成。
- 工作区原有 `rewards/interfaces/http/metrics.go` 删除仍未纳入本批；这是独立的 observability 回归，需单独决定恢复或替代实现。

## 第四批补充：Media Storage presign provider boundary（4C）

本子批修复 `StorageService.Presign` 的 provider boundary：Local profile 不支持 provider hand-off，应在 integrity re-read 前立即拒绝。此前实现会先执行一次无意义的 object I/O；当 Local object 不存在或 integrity facts 不匹配时，调用方可能收到误导性的 storage/integrity error，而不是稳定的 unsupported-provider contract。

同时补充 storage profile ID 的 resolve error context，便于定位多 profile 配置问题。现有 `TestPresignRejectsLocalProfileForProviderHandOff` 已覆盖该顺序契约，无需扩大测试 fixture。

### Verification and boundary

- `cd apps/api && go test ./internal/media/storage/tests -run TestPresign -count=1` 通过。
- `cd apps/api && go vet ./internal/media/storage/application/runtime` 通过。
- Clean archive（仅替换 `presign.go`）中 `go test ./internal/media/storage/application/runtime -count=1` 与 `go vet ./internal/media/storage/application/runtime` 通过。
- 提交：`6669d6ee fix(media): reject local presign before integrity lookup`。
- 本批未触碰当前 dirty 的 media pgx migration/test fixtures；跨 profile persistence、driver capability 和 presign URL security 仍需后续批次单独审计。

## 第四批补充：Rewards redemption observability（4D）

工作区原有 staged deletion 删除了 `rewards/interfaces/http/metrics.go` 及其测试。该删除与幂等 consumer 改造无关，并违反 Rewards `CONTEXT.md` 中“固定 outcome、credited Ideas、latency，且禁止用户/Code/digest labels”的 observability contract。本子批恢复 bounded Prometheus recorder，并接回当前 `UserHandler.Redeem`：invalid input、rate limit、idempotency conflict、service failure、replay 和 success 都会记录固定 outcome；replay 不重复 credited credits，数据库冲突通过 PostgreSQL SQLSTATE 分类。

### Verification and boundary

- `cd apps/api && go test ./internal/rewards/interfaces/http ./internal/rewards/tests -count=1 -race` 通过。
- `cd apps/api && go vet ./internal/rewards/interfaces/http ./internal/rewards/tests` 通过。
- Clean archive（当前 HEAD 的 API）中 Rewards HTTP package/tests `go test -count=1` 与 `go vet` 通过。
- 提交：`d2d4c0ad fix(rewards): restore redemption observability`。
- 本批只恢复 HTTP redemption metrics；跨实例 metrics aggregation、durable idempotency fact 及 Rewards database transaction 仍不在此批范围内。

## 第四批补充：Identity internal HTTP error seam（4E）

本子批把 Identity internal login-metadata endpoint 及可复用的 Admin command error mapping 接到 shared `errorrender`：wire response 使用稳定 machine code，内部错误保留 server-side logging，避免 provider/Auth/database detail 进入响应。`constantTimeHeaderEquals` 的鉴权行为保持不变；新增测试覆盖 missing/wrong key、valid update 和 invalid body。

### Verification and boundary

- `cd apps/api && go test ./internal/identity/users/interfaces/http -count=1 -race` 通过。
- `cd apps/api && go vet ./internal/identity/users/interfaces/http` 通过。
- Clean archive（仅替换本子批 3 个文件）中同 package test/vet 通过。
- 提交：`cfebb45a fix(identity): stabilize internal HTTP error envelopes`。
- 当前 dirty 的 Identity Admin handler 尚未纳入本提交；它引用了同一 error seam，但与其他 user/admin lifecycle 改动耦合，后续需在完整闭包中验证。

## 第四批补充：Identity Admin HTTP mapping（4F）

在 4E error seam 之上，本子批把 Admin settings/user/impersonation 分支迁移到稳定 machine codes，并将 upstream/Auth/operation details 留在 server logs；原有鉴权、target checks 和 status classification 保持不变。这样 validation、not-found、forbidden、upstream rejected/unavailable 不再依赖可变的 copy 或泄露 wrapped error。

### Verification and boundary

- `cd apps/api && go test ./internal/identity/users/interfaces/http -count=1 -race` 通过。
- `cd apps/api && go vet ./internal/identity/users/interfaces/http` 通过。
- Clean archive（替换 `admin.go`）中同 package tests 通过。
- 提交：`de31ff78 fix(identity): map admin errors to stable codes`。
- 本批未覆盖 Identity repository、auth service client 和 user lifecycle 的其他 dirty changes。

## 第四批补充：Runtime RED metrics primitive（4G）

本子批提交 Runtime RED metrics primitive 与 Gin middleware：HTTP request 按 method/route template/status 记录 counter + latency，Gateway 按 bounded feature/outcome 记录 request/retry，River queue 通过 timeout-bounded refresher 发布 depth/head lag。nil-safe observer、collector reuse、negative input normalization 和 refresh failure 保留 last gauge，避免 metrics path 反向阻塞请求或 scrape；raw URL、user/task ID、error text 不进入 labels。

### Verification and boundary

- clean archive（仅加入 metrics primitive/middleware 文件）中：
  `go test ./internal/runtime/infrastructure/metrics ./internal/runtime/interfaces/http -run 'Test(Observe|Nil|Gateway|Refresh|Refresher|Multi|NewSet|REDMetrics)' -count=1 -race` 通过。
- 同范围 `go vet` 通过。
- 提交：`ba9c8ceb feat(runtime): add bounded RED metrics primitives`。
- 本批未提交 `routes.go`/composition wiring；这些文件同时包含 Search/AIGateway/Billing module migration，必须在后续完整闭包中审计，当前 middleware 仍未接入 production route tree。

## 第四批补充：S3 outbound delivery and data-plane metrics（4H）

本子批将 S3-compatible delivery 的 public/signed GET 迁移到标准 `outboundhttp.Client`，统一 timeout、allowlist、retry/status handling；Put/Delete data-plane 调用通过共享 `s3-storage` target 记录 success/canceled/timeout/http 4xx/http 5xx/transport outcomes。404 miss 仍作为 delivery fallback/control flow，不误报为 transport failure；response body cleanup 和原有 signed fallback 语义保持不变。

### Verification and boundary

- clean archive（替换 S3 `service.go` 与 RED helper/test）中 `go test ./internal/media/storage/infrastructure/s3 -count=1` 通过。
- 同 package `go vet` 通过。
- 提交：`63569322 fix(media): standardize S3 outbound delivery`。
- 本批未覆盖 media Postgres repository、profile HTTP handler 或 remote downloader；这些改动仍需与 storage migration 闭包一起审计。

## 第四批补充：Remote image outbound boundary（4I）

本子批将 remote image fetch 迁移到标准 outbound client，统一 30s timeout、allow-host SSRF policy、bounded retries 和 Retry-After。`HTTPStatusError` 转换为兼容旧逻辑的 response/status，只有 transport/timeout 作为 Go error 返回；body 仍由既有 size-limited image decoder 负责读取与 cleanup。

### Verification and boundary

- clean archive（替换 remote downloader）中 `go test ./internal/media/storage/infrastructure/remote -count=1` 通过。
- 同 package `go vet` 通过。
- 提交：`2ea061d8 fix(media): standardize remote image egress`。
- 本批未修改 remote allowlist provider 或 media HTTP handler；其 policy/config ownership 仍需后续审计。

## 第四批补充：Media Storage HTTP error contract（4J）

本子批将 Storage profile HTTP handler 的 nil-service、JSON validation、list/command/provider errors 统一到 machine-readable `storage_*` codes。Curated `UserMessage()` 仍只用于明确安全的 profile validation；其他 wrapped provider/database errors 只进入 server logs，响应保持固定 safe message。并补充了 handler error envelope tests。

### Verification and boundary

- clean archive（替换 Storage HTTP handler/error seam/test）中 `go test ./internal/media/storage/interfaces/http -count=1 -race` 通过。
- 同 package `go vet` 通过。
- 提交：`cf5282b3 fix(media): stabilize storage HTTP errors`。
- 本批未覆盖 storage repository、profile migration 和 upload/download handlers 的其他 dirty changes。

## 第四批补充：Payments wrapped-error response boundary（4K）

本子批修复 Payments HTTP `paymentErrorMapping` 的 response leak：缺少显式 safe message 的 mapping 不再回显 `err.Error()`，统一使用固定 fallback，并通过 shared `errorrender` 记录 server-side detail。测试覆盖 errors.Join/wrapped provider payload、Stripe/checkout/webhook mappings，以及 empty-message fallback，确保 secrets 不进入 wire envelope。

### Verification and boundary

- clean archive（仅替换 Payments `errors.go`/`errors_test.go`）中 `go test ./internal/payments/interfaces/http -count=1 -race` 通过。
- 同 package `go vet` 通过。
- 提交：`82b3d0e8 fix(payments): prevent wrapped error detail leaks`。
- Admin order pagination mapping、recharge history pagination 和 handler migration 仍未纳入；它们依赖当前 dirty application/readmodel closure，后续单独审计。

## 第四批补充：Stripe outbound observability（4L）

本子批为 Stripe SDK account/product/webhook/checkout/refund/dispute 调用接入固定 `stripe` target 的 outbound RED recorder，统一记录 success、HTTP 4xx/5xx、timeout/canceled/transport outcome 和 latency。Recorder 可注入 isolated metrics registry，production fallback 到 shared default metrics；labels 不含 account、customer、order、URL 或 provider payload。

### Verification and boundary

- clean archive（替换 Stripe adapter/checkout/dispute/refund 并加入 recorder/test）中 `go test ./internal/payments/infrastructure/stripe -count=1 -race` 通过。
- 同 package `go vet` 通过。
- 提交：`186736a0 feat(payments): record Stripe outbound outcomes`。
- 本批未修改 Payments transaction/reconciliation/webhook application semantics；仅增加 outbound observability。

## 第四批补充：Payments readmodel pagination contract（4M）

本子批将 Admin payment orders 与 user recharge history 的 readmodel cursor/limit 规则收敛到 shared `platform/pagination`：default 50、max 200、opaque cursor max 512 bytes，领域层仍负责 timestamp/UUID payload validation。该值与 `apps/api/openapi.yaml` 及 Payments `CONTEXT.md` 的 pagination contract 一致，避免 HTTP/application 两套边界漂移。

### Verification and boundary

- clean archive（替换 readmodel filter/cursor 文件并加入纯测试）中 `go test ./internal/payments/application/readmodel -count=1 -race` 通过。
- 同 package `go vet` 通过。
- 提交：`2a242ba6 fix(payments): align readmodel pagination contract`。
- `readmodel/service.go` 的 pgx/sqlc time/UUID conversion、HTTP handler machine mapping 和 integration pagination tests 仍未纳入，待下一批闭包验证。

## 第四批补充：Payments pagination HTTP closure（4N）

本子批完成 pagination 的 application/HTTP 闭包：readmodel service 适配 pgx/sqlc 的 nullable time 与 UUID cursor 字段；Admin payment order 的 filter/cursor、user recharge history 的 limit/cursor 都通过 stable machine codes 返回，避免 raw error text；HTTP 输入统一使用 shared pagination parser。新增 Admin handler safe-message tests。

### Verification and boundary

- clean archive（替换 readmodel service/facts 与 Payments HTTP handler 文件）中 `go test ./internal/payments/interfaces/http -count=1 -race` 通过。
- 同 package `go vet` 通过。
- 提交：`b77e7a73 fix(payments): harden pagination HTTP boundaries`。
- DB integration pagination fixture `payments/tests/recharge_pagination_test.go` 仍未提交，需等 testdb/sqlc migration dirty closure 一起验证。

## 第四批补充：Workflow vision namespace guard（4O）

本子批固定 `vision` 为 search-only reserved slug，阻止它进入 image workflow registry。这样不会再次把 `ai-image:vision` 与 `ai-search:vision` 映射到同一个 bare key，避免 Gateway route/catalog 配置被覆盖。补充 regression test 验证 parser、validator 与 image classification 全部拒绝该 slug，并保留原 provider API type tests。

### Verification and boundary

- clean archive（替换 workflow feature files）中 `go test ./internal/workflow -run 'TestVisionSlugIsReservedForSearch|TestValidateProviderAPIType' -count=1 -race` 通过。
- 提交：`28656b46 fix(workflow): reserve vision search slug`。
- `feature_rules.go` 的删除及 provider registry migration 仍未纳入，本批只处理 slug namespace invariant。

## 第四批补充：River retry/timeout policy matrix（4P）

本子批新增 Task queue 的 code-owned retry/timeout matrix，覆盖 drawing、final-failure、credit/payment reconciliation、checkout shutdown、retention、vision billing 和 media reclamation workers。Matrix 以 worker/queue roster、正数预算、跨 worker 一致性和关键安全上限测试约束，防止新 worker 静默继承全局 `JobTimeout` 或重复散落 retry literals。

### Verification and boundary

- clean archive（仅加入 queue matrix 文件）中 `go test ./internal/tasking/infrastructure/queue -run 'TestRiver(Timeout|Retry)Matrix' -count=1` 通过。
- 提交：`4fb2ffce fix(tasking): centralize River retry and timeout policy`。
- 本批只提交 policy/matrix contract；各 worker `Timeout()`/`MaxAttempts` wiring 与 queue routing SQL migration 仍需后续闭包审计。

## 第四批补充：Provider cache/image reference primitives（4Q）

本子批提交两个 network-free provider domain primitives：versioned SHA-256 text cache key 按 feature/model/canonical request bytes 隔离；provider image reference 仅接受 public HTTPS URL，relative `/uploads/` 必须通过 configured API origin 解析。测试覆盖 deterministic key、scope collision、empty input、HTTP/scheme-relative rejection 与 upload resolution。

### Verification and boundary

- clean archive（仅加入 provider domain primitive/test）中 `go test ./internal/runtime/interfaces/worker/providers/domain -count=1 -race` 通过。
- 同 package `go vet` 通过。
- 提交：`f45f27ab feat(providers): add cache and image reference primitives`。
- Executor、dialects、gateway client 与 probe wiring 属于尚未闭合的 provider migration，本批未纳入。

## 第四批补充：Search/Vision domain contract（4R）

本子批建立 Searchcatalog domain contract：web search provider vocabulary 只允许 Unsplash/Pexels，Vision search 使用独立 failure taxonomy 与 bounded image/byte/timeout limits，不把 Vision provider 混入 web-search credentials。`VisionProviderError` 保留 machine-readable failure kind，同时通过 `Unwrap` 保留 cause 供内部分类。

### Verification and boundary

- clean archive（仅加入 Searchcatalog domain files/tests）中 `go test ./internal/searchcatalog/domain -count=1 -race` 通过。
- 同 package `go vet` 通过。
- 提交：`b130054d feat(search): establish closed vision domain contract`。
- Vision input materializer、HTTP mapping、provider adapters 与 persistence migration 仍属于后续闭包。

## 第四批补充：Searchcatalog ports（4S）

本子批定义 Searchcatalog 的 dependency inversion seams：web provider search/test adapters 只依赖 closed domain vocabulary；Vision config 只携带 model，AI Gateway endpoint/token 留在 central gateway；gateway port 只交换 bounded request/response facts，不导入 Runtime transport implementation。

### Verification and boundary

- clean archive（仅加入 Searchcatalog ports）中 `go test ./internal/searchcatalog/ports -count=1` 与 `go vet` 通过。
- 提交：`ec7a544b feat(search): define gateway and provider ports`。
- Application/infrastructure/composition wiring 尚未提交，需在后续完整闭包验证。

## 第四批补充：Search frontend image-failure reporting removal（4T）

本子批审计并提交 Search frontend 对旧 image-failure telemetry 的移除：`createSearchFeed` 不再暴露或调用 `reportImageFailure`，`resource.ts` 不再依赖旧 generated mutation；image failure/retry 状态继续由 feed 本地维护。该删除与 Phase 3 Vision Search spec 一致：该阶段只要求有效 Vision 提交的 `vision_search_requests_total`，image-failure reporting 属于 deferred observability。MCP `codegraph_explore` 与全仓符号扫描确认旧 endpoint、handler、metric 和 generated caller 无残余运行时引用。

### Verification and boundary

- 当前 workspace Search tests：`npm run test:run -- src/features/search`，`12 files / 56 tests` 通过。
- clean `HEAD + 本批 3 个代码文件` snapshot：`feed.unit.spec.ts`，`9 tests` 通过。
- `git diff --check` 通过；Standards reviewer 未发现 documented-standard 违反或 Fowler smell，Spec reviewer 未发现缺失要求、scope creep 或实现错误。
- 提交：`75bbc00b fix(search): remove obsolete image failure reporting`。
- `apps/web/src/features/search/CONTEXT.md` 原有大范围重写中仍残留 “failure reporting” 描述；已修正工作区文案，但未将整份用户 dirty 文档并入本代码 commit，待 Search docs 批次单独审计。
- `apps/api/openapi.yaml`、backend metrics handler 删除和 generated client 变更混在大型 Searchcatalog/API migration 中，本批未强行拆入；后续需验证其完整 schema/codegen 闭包。

## 第四批补充：Input focus-state primitive（4U）

本子批审计并提交 shared `Input` primitive 的 focus visual contract：移除默认 shadow，keyboard focus 从额外 ring 收敛为 semantic border color；invalid 状态使用 destructive focus border。MCP blast-radius 扫描确认该 variant 只经 `Input.vue` 暴露，消费者无需迁移。测试新增 invalid focus class 断言，并确认 `tailwind-merge` 会用 destructive focus 覆盖 base ring color。

### Verification and boundary

- 当前 workspace：`npm run test:run -- src/components/ui/primitives.unit.spec.ts`，`6 tests` 通过。
- clean `HEAD + 本批 2 个文件` snapshot：同一 focused test，`6 tests` 通过。
- `npm run build` 通过（`vue-tsc`、Vite、bundle size check）。
- `git diff --check` 通过。
- 提交：`8ccf0310 fix(ui): use border focus state for inputs`。
- 本批只处理 shared Input primitive；未修改各 feature 表单布局或 Admin model catalog 的其他 dirty UI。

## 第四批补充：Admin model catalog + generated API sync（4V）

本子批审计并提交 Admin model catalog 前端闭包：catalog/reference/observed UI、brand logo registry 与 generated API client 同步 `apps/api/openapi.yaml`。修复 catalog reference 与 route preview 的 stale async response 覆盖，修正 SKU 区域标题以匹配实际展示的 on-sale/off-sale 状态，并保留 provider-qualified model identity 语义，不引入额外过滤或跨 provider 归一化设计。MCP Codegraph 复核了 Admin handler、generated API 与 model UI 的调用边界。

### Verification and boundary

- 当前 workspace：`npm run api:check-generated` 通过；Admin models/logos focused tests 为 `11 files / 89 tests`；`npm run build` 通过。
- clean `HEAD + 本批 frontend/API 闭包` snapshot：generated drift 通过，`11 files / 89 tests` 通过；`vue-tsc` 与 Vite 编译通过。
- clean snapshot 的 bundle-size gate 仍因既有 `heic2any` chunk `1320.5 KB` 超过 `634.8 KB` budget 而失败；未为此引入与本批无关的拆包设计。
- `git diff --check` 通过。
- 提交：`966cb75e feat(admin): sync gateway model catalog API`。
- backend `internal/aigateway` 尚未在本批提交：其事务保存调用依赖尚未闭合的 `settings`/sqlc/DB migration closure，后续单独审计后再提交，避免产生 HEAD 不可编译的中间 commit。

## 第四批补充：AI Gateway/backend transaction audit（4X，暂不提交代码）

本子批继续审计 backend `internal/aigateway`、`platform/settings` 与 PGX/sqlc 迁移边界。MCP impact/Codegraph 确认 AI Gateway package 依赖 `settings.SetAndDeleteManyWithAdminAudit`，而 settings 又依赖整套 PGX `sqlc` generated interface 与大量 caller migration，不能做只包含单 package 的中间提交。

审计期间修复但暂留工作区的两项 handler/config 问题：

- `TestConnection` 在 Admin 请求需要从 mapping 推导 route 时，若 handler 未注入 store 会 nil dereference；现统一返回 `503 Service Unavailable` 并有 regression test。
- `ConfigRepository.Update` 在空 `base_url` 时吞掉 `GetSummary` 的非空 I/O error；现以 `load existing AI gateway config` 上下文返回。

仍需在完整 transaction closure 中修复的高风险项：`SaveCatalog` 在取得 advisory lock 前读取旧 catalog/mappings，存在并发 mapping 写入被随后 cascade 删除的数据丢失窗口。必须将 stale-key 计算与 lock/write 放入同一 pgx transaction seam，不能通过额外 retry 或 defensive copy 掩盖。

### Verification and boundary

- 当前 workspace：`cd apps/api && go test ./internal/aigateway ./internal/platform/settings -count=1 -race` 与 `go vet ./internal/aigateway ./internal/platform/settings` 通过。
- 当前 API 全量测试除工作区已有 workflow dirty 缺失符号（`ProviderOpenAICompatible`、`ProviderGoogleAI`、`ValidateProviderAPIType`）外均通过；失败属于未纳入本批的 workflow closure。
- PGX targeted checks 通过：`internal/db/dbutil`、`internal/platform/database/...`、`internal/runtime/infrastructure/database`；nested `internal/testdb` tests 通过。
- clean HEAD snapshot 证明：PGX/dbutil 不能脱离 generated sqlc 与 settings callers 编译；替换全套 sqlc + settings + PGX database/testdb 文件后 targeted packages 通过。
- transaction reviewer 未发现 rollback 修复 P1，但指出 `Commit(ctx)` cancellation 的结果-unknown 仍缺真实 PostgreSQL 回归测试。
- schema reviewer 要求 migrations → schema snapshot → queries → generated sqlc 原子闭包，并提示包含 DROP/purge 的 migration 需要 Fresh DB、backup 与 rollback/cutover 计划。
- 本子批不提交 backend 代码，避免产生 HEAD 不可编译、SQL/generated 不一致或保留已知并发数据丢失窗口的 commit；待完整 PGX/sqlc/schema/consumer closure 后一次性提交。

## 第四批补充：Workflow provider API type contract（4Y）

全量 Go test 暴露 `internal/workflow/features_test.go` 引用的 provider API type contract 在 dirty workspace 中被删除：`feature_rules.go` 缺失 `ProviderOpenAICompatible`、`ProviderGoogleAI`、`ProviderGoogleCloud` 与 `ValidateProviderAPIType` 等定义。该文件本来已存在于 HEAD，恢复后与 HEAD 字节一致，因此本次是恢复误删而非新增代码，不产生代码 commit；保留工作区恢复状态以维持测试契约。

### Verification and boundary

- `cd apps/api && go test ./internal/workflow -count=1 -race` 通过。
- `go vet ./internal/workflow` 通过。
- clean `HEAD + feature_rules.go` snapshot 同样通过 workflow test/race/vet。
- 未扩展到 provider runtime 或 database migration；本项仅恢复被删除的 workflow contract。

## 第四批补充：Searchcatalog backend closure（4Z，暂不提交代码）

本子批审计 Searchcatalog 的 domain/ports/application/infrastructure/interfaces，并用 MCP Codegraph 追踪 Search、Vision billing、provider adapter 与 Admin settings 边界。发现并修复三处 HTTP nil dependency panic：Search handler、Admin provider Update/Test 在未组合依赖时现在 fail-closed 返回 `503`，并新增 regression test。

另外修复 `VisionBillingService.ReconcilePending` 的连接池占用窗口：先完整读取并检查 `rows.Err()`，立即关闭查询 rows，再逐项开启 settlement transaction。这样满批（100 条）时不会让扫描连接与 reconcile transactions 同时占用 pool。

### Verification and boundary

- `cd apps/api && go test ./internal/searchcatalog/... -count=1 -race` 通过。
- `go vet ./internal/searchcatalog/...` 通过。
- clean snapshot 表明 Searchcatalog application/interface 依赖同一轮 Billing DDD reorg；Billing 又依赖 PGX/sqlc caller migration，无法从 HEAD 单独构建。因此本子批代码不单独 commit，避免产生不可编译中间态。
- reviewer 未发现新的 P1/P2；outbound allowlist/redirect、Vision input size/MIME、billing reserve/capture/release transaction 与 goroutine cleanup 均通过静态审计。
- residual：尚未执行真实 PostgreSQL/HTTP integration；完整提交需与 Billing + PGX/sqlc closure 一起验证。

## 第四批补充：AI Gateway admin response/probe security hardening（4W，待 backend closure）

MCP Codegraph 与 security review 识别并修复两项明确问题：Admin brand-logo response 的 cache policy 从 `public` 改为 `private`，避免共享 proxy/CDN 缓存绕过后续权限检查；`ProbeGatewayConnection` 拒绝 URL userinfo、query 与 fragment，避免敏感信息进入 URL/日志并保持 probe target 由受控 path 构造。补充 handler header 与 URL validation regression tests，并清理同一位置的重复注释。Logo cache 无界增长与 observed SQL 高基数属于低风险残余项，按“不过度 defense / design”暂不扩展。

### Verification and boundary

- `cd apps/api && go test ./internal/aigateway -count=1 -race` 通过。
- `cd apps/api && go vet ./internal/aigateway` 通过。
- clean `HEAD + internal/aigateway` snapshot 暴露其依赖 `settings.SetAndDeleteManyWithAdminAudit` 尚未进入 HEAD；因此安全修复将随完整 backend transaction/sqlc closure 一起提交，当前不单独制造不可编译 commit。
