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
