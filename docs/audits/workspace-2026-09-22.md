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

### 验证与边界

- `cd apps/api && go test ./internal/platform/outboundhttp ./internal/platform/safehttp -count=1 -race` 通过。
- `cd apps/api && go vet ./internal/platform/outboundhttp ./internal/platform/safehttp` 通过。
- 本批只验证共享客户端与安全边界；provider-specific status mapping、Cloudflare gateway billing 及业务 retry budget 仍在后续批次审计。
