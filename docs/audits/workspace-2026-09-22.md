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
| 2 | PostgreSQL / pgx / sqlc、Schema、migrations、runtime privileges | 类型映射、事务、权限、migration replay、生成一致性 | 待审计 |
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
