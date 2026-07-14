# Credits Context

Credits 是 IdeaBoard 的平台级封闭生态货币。用户文案统一显示为 `Idea`，数量恰好为 1 时显示 `1 Idea`，其余数量显示 `N Ideas`；内部 domain unit、代码、API 与 schema 继续使用 `Credit/credits`。本 context 只拥有货币账户、不可变账本、余额 projection 和 Credit Commands；商品身份、versioned pricing 与 Quote 属于 `billing` context。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Credit | 不可拆分的整数货币单位，禁止 floating point |
| Credit Wallet | 单个用户在 Credits context 中的账户边界 |
| Credit Account | Double-entry posting 的落点；每个 Wallet 有 available/reserved，System account 按 Wallet shard |
| Ledger Transaction | 一次不可变的账务事实，包含同一 asset 下至少两条 Posting |
| Posting | 对一个 Credit Account 的有符号增减；同一 Transaction 的合计必须为 0 |
| Balance Projection | `credit_accounts.balance` 的 O(1) 缓存，只能由 Posting trigger 更新，不是独立事实源 |
| Grant | System Issuance → User Available |
| Hold | User Available → User Reserved |
| Capture | User Reserved → System Revenue |
| Release | User Reserved → User Available |
| Refund | System Revenue → User Available，累计不得超过原 Charge |
| Reconciliation Run | Admin 以 non-empty reason 触发、River 异步执行的全量一致性扫描；run 持久化原始 actor/reason，只检测、报告和隔离 |
| Finding | 由 finding type + Wallet + subject 生成稳定 fingerprint 的可重跑异常事实 |

## Public Seams

- `Commands` 是唯一应用写入入口，统一提供 Grant、Deduct、Hold、Capture、Release 与 Refund；底层只调用 typed `SECURITY DEFINER execute_credit_command_v2`。
- `Service.InTx(pgx.Tx)` 与 `Service.InSQLTx(*sql.Tx)` 是同一 `Commands` contract 的 caller-owned transaction adapters：Task/River submission 使用 pgx，现有 Task Store terminal transition 使用 database/sql；caller 都拥有 commit/rollback。
- `WalletReader` 提供 Wallet balance 与 keyset-paginated Ledger 查询。
- `Service.GetCaptureUsage` 是已结算 Capture 的 typed-location 按用户只读聚合实现，供 caller-owned Usage interface 组合；不返回 Refund，也不授予 Ledger 写入能力。
- 用户侧 Billing Records 是独立 caller-owned Read Model：可读取 Ledger 最终收支并组合 immutable Task Billing Snapshot，但不复用 Admin Ledger DTO、不返回 Hold/Release/Refund，也不获得 Ledger 写入能力。
- `Reconciliation` 通过独立 River queue 运行，Admin 可读取 run history、summary 和 keyset-paginated findings。
- Admin HTTP 提供单用户 Wallet/Ledger read、Grant、Deduction、linked Refund 与 Correction；不提供 set balance、更新或删除账本历史。Ledger read 显式返回 Hold/Charge reference，避免 Admin 以裸关联 ID 推断结算状态。

其他 context 不得直接更新 `credit_accounts.balance`、插入 Ledger 或自行计算幂等 hash。

## Invariants

- `CreditAmount` 为正整数，单次命令上限为 `9_000_000_000_000_000`；available/reserved 永不为负。
- Ledger Transaction 支持 N postings。PostgreSQL deferred constraint 在 commit 时校验 posting count、单一 asset 和总和为 0。
- Ledger Transaction 与 Posting 禁止 UPDATE/DELETE；错误只能追加 reversal/correction transaction。
- Posting statement 按 `account_id` 稳定顺序更新 projection，Command 在写入前也按相同顺序锁账户。
- API runtime role 对 Credit tables 只有 SELECT，不能直接 DML Ledger/projection，也不能执行内部 trigger functions；migration owner 才拥有 schema。
- System Issuance、Revenue 与 Sink 使用 `wallet:<user_id>` shard，并保留 role/metadata，避免单个全局 system row 串行化所有 Wallet。
- 每个 Command 先取得 idempotency-key advisory transaction lock，再比较服务端 canonical SHA-256 request hash。相同 key/相同 hash 返回原结果；不同 hash 返回 conflict。
- Task Hold 不使用 Credits-owned TTL 自动解冻；只有 Task terminal transition 能在同一 transaction 中 Capture 或 Release。
- actor、source、reason 和 business reference 必填并写入不可变 Transaction。
- Deduction 是仅限 Admin actor 的新 immutable `user_available → system_sink` transaction，永不消费 reserved。Correction 仅限 authenticated Admin、关联原 Admin Grant/Deduction/Refund，并以原 Posting 的反向 Posting 追加记录；Capture 更正使用 linked Refund。
- frozen 或匿名化 tombstone Wallet 的历史仍可读。discretionary Grant/Deduction 被拒绝；仅 authenticated Admin 的 linked Refund/Correction 可写入 frozen Wallet，余额仍被冻结而不可消费。
- Auth public registration 只接受 username/password，由服务端 canonicalize username、生成 synthetic claim identity，并在受控命令内标记为 `trusted_canonical_registration`；直接 Better Auth email sign-up 被拒绝。
- Reconciliation scan 使用 MVCC reads，不锁 Ledger/Account 热行；DB advisory lock 与 active-run unique index 双重保证同一时刻仅有一个 active run。
- 启动 Reconciliation 必须提供 trim 后 1–1000 字符的 reason；active run 的重复 start 返回原 run，禁止覆盖首次请求的 actor/reason。
- `warning/low`（当前为长期 Hold）只报告；`error|critical/high` 会在 finding、alert 与 run report 同一 transaction 内冻结相关 Wallet。扫描不得修改 Ledger、projection 或自动创建 Correction。
- Finding fingerprint 不包含 run ID；重复异常复用 open finding 并增加 occurrence/run link，异常消失后标记 resolved。frozen Wallet 的历史 read contract 不变。
- Identity 通过 typed Wallet lifecycle seam 原子 provision Wallet、冻结/恢复 Wallet，并以 trusted canonical registration identity 的 keyed HMAC claim 幂等发放 `100 Ideas` Welcome Grant；claim identity 明文不进入 Credits schema。
- Welcome identity HMAC key 首次使用时通过 typed `SECURITY DEFINER` command 永久绑定数据库 fingerprint；配置误变更时 startup/provision fail closed，避免同一身份生成新 claim。
- 删除业务用户先在本地事务冻结 Wallet 并记录 durable deletion phase，再依次幂等删除 Auth、清理 Storage、写 user tombstone；失败请求可从持久 phase 重试收敛。最终保留 user tombstone 主键、Task、Wallet、Ledger 与 Welcome claim；相同领取身份重新注册不会再次获得 Welcome Grant。

## Verification

默认 suite 使用 `internal/testdb` 的隔离 PostgreSQL schema 验证 Commands、HTTP contract、并发锁、limited runtime role 和数据库 constraints。百万数据集 harness 由 migration owner/test DSN 通过 `CREDITS_LOAD_TEST=1` 显式开启，命令和基准结果见 `docs/benchmarks/credits-million.md`。
