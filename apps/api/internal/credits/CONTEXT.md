# Credits Context

Credits 是 IdeaBoard 的平台级封闭生态货币。用户文案统一显示为 `Idea`，数量恰好为 1 时显示 `1 Idea`，其余数量显示 `N Ideas`；内部 domain unit、代码、API 与 schema 继续使用 `Credit/credits`。本 context 只拥有货币账户、不可变账本、余额 projection 和 Credit Commands；商品身份、versioned pricing 与 Quote 属于 `billing` context。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Credit | 不可拆分的整数货币单位，禁止 floating point |
| Credit Wallet | 单个用户在 Credits context 中的账户边界 |
| Wallet Restriction | 与 Wallet 生命周期状态独立的原因化消费限制；按 type、source 与 related object 追溯，可由各来源独立解除 |
| Credit Account | Double-entry posting 的落点；每个 Wallet 有 available/reserved，System account 按 Wallet shard |
| Ledger Transaction | 一次不可变的账务事实，包含同一 asset 下至少两条 Posting |
| Posting | 对一个 Credit Account 的有符号增减；同一 Transaction 的合计必须为 0 |
| Balance Projection | `credit_accounts.balance` 的 O(1) 缓存，只能由 Posting trigger 更新，不是独立事实源 |
| Grant | System Issuance → User Available |
| Hold | User Available → User Reserved |
| Capture | User Reserved → System Revenue |
| Release | User Reserved → User Available |
| Refund | System Revenue → User Available，累计不得超过原 Charge |
| Purchase Credit | 已验证外部支付的 `System Issuance → User Available`；由 Payments 以固定 actor/source/reference 调用，不是 discretionary Grant |
| Purchase Refund | 已验证 Stripe Refund 对应的 `User Reserved → System Issuance`；必须关联 Payments 创建的 Refund Hold，幂等终态结算 |
| Reconciliation Run | 由 `manual` 或 `scheduled` Trigger 启动、River 异步执行的全量一致性扫描；manual 持久化真实 Admin/reason，scheduled 持久化 schedule slot 且不伪造 actor |
| Finding | 由 finding type + Wallet + subject 生成稳定 fingerprint 的可重跑异常事实 |
| Finding Type | Credits 拥有的稳定 typed anomaly contract；Admin 对已知类型提供统一名称与摘要，未知类型必须安全降级 |

## Public Seams

- `Commands` 是唯一应用写入入口，统一提供 Grant、Deduct、Hold、Capture、Release、Refund、Purchase、PurchaseRefund、PaymentDisputeDebit 与 PaymentDisputeRestore；外部支付命令使用独立 typed `SECURITY DEFINER` routines，其余命令通过 `execute_credit_command_v2`。
- `Service.InTx(pgx.Tx)` 与 transaction-scoped `CommandsInSQLTx(*sql.Tx)` 是同一 `Commands` contract 的 caller-owned transaction adapters：Task/River submission 使用 pgx，`taskbilling.SettleTerminal` 使用 database/sql；caller 都拥有 commit/rollback。Task Repository 不持有 application-lifetime Credits/Billing dependency。
- `WalletReader` 提供 Wallet balance 与 keyset-paginated Ledger 查询。
- `WalletRestrictionCommands` 通过 transaction-capable typed seam 幂等创建和精确解除 Restriction；其他 context 不得直接写 Restriction table。
- `Service.GetCaptureUsage` 是已结算 Capture 的 typed-location 按用户只读聚合实现，供 caller-owned Usage interface 组合；不返回 Refund，也不授予 Ledger 写入能力。
- 用户侧 Billing Records 是独立 caller-owned Read Model：可读取 Ledger 最终收支并组合 immutable Task Billing Snapshot，但不复用 Admin Ledger DTO、不返回 Hold/Release/Refund，也不获得 Ledger 写入能力。
- `Reconciliation` 通过独立 River queue 运行，Admin 可读取 run history、summary，以及不依赖 Run ID 的 open/resolved 全局 keyset Finding queue。
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
- Purchase 仅接受 `service/payments` actor、`stripe_purchase` source 与 `payment_order` business reference；它从不创建 Wallet，也不能由 Admin 或客户端伪造。已验证外部付款是平台既有义务，因此 Purchase 不受 frozen lifecycle 或 active Restriction gate 阻断，必须与 Payments 的 Payment Order terminal fact 原子收敛。
- Task Hold 不使用 Credits-owned TTL 自动解冻；只有 Task terminal transition 能在同一 transaction 中 Capture 或 Release。
- actor、source、reason 和 business reference 必填并写入不可变 Transaction。
- Deduction 是仅限 Admin actor 的新 immutable `user_available → system_sink` transaction，永不消费 reserved。Correction 仅限 authenticated Admin，并以原 Posting 的反向 Posting 追加记录；允许关联原 Admin Grant/Deduction/Refund，或固定 `source=reward_redemption` 的 user-origin Grant，其他非 Admin-origin Grant 仍拒绝。Capture 更正使用 linked Refund。
- frozen 或匿名化 tombstone Wallet 的历史仍可读。discretionary Grant/Deduction/Hold 被拒绝；既有 Hold 的 Capture/Release 必须继续收敛，authenticated Admin linked Refund/Correction 仍可用于修复，Wallet 不可开始新的消费。
- Wallet lifecycle status 与 active Restriction 是正交状态。Grant、Deduction、Hold 仅在 lifecycle active 且不存在 active Restriction 时允许；Capture、Release 必须继续收敛既有 Hold，authenticated Admin linked Refund/Correction 继续承担修复职责。Command 的 idempotency replay 在当前 gate 前返回原结果。
- Restriction create/release 先锁 Wallet row，与 Credit Command gate 线性化；同一 `(Wallet, type, source, related object)` 仅允许一个 active row，解除保留历史且不会清除其他来源的 Restriction。
- Auth public registration 使用 canonical Email、username 和 Password；Email verification 成功后由 `canonical_email` verified identity 触发 Wallet provisioning，不再生成 synthetic Email。
- Reconciliation scan 使用 MVCC reads，不锁 Ledger/Account 热行；DB advisory lock 与 active-run unique index 双重保证同一时刻仅有一个 active run。
- Credit Health 状态固定为 `unchecked | healthy | attention | critical | stale | failed`。判定优先级为：open high-risk Finding 或其精确 active `ledger_integrity/reconciliation` Restriction → critical；最新 failed Run 晚于最近成功 Run → failed；从未成功且无 failed → unchecked；最近成功严格超过 48 小时 → stale；其余 open Finding → attention；否则 healthy。active Run 单独返回，不覆盖状态。
- Reconciliation 的 Stale Hold 策略固定为 24 小时并由 DB per-Run snapshot/constraint 作为事实源；Admin API/UI 不接受阈值，public Run DTO 也不暴露这一内部固定值。
- manual Reconciliation 必须提供真实 Admin 与 trim 后 1–1000 字符的 reason；scheduled Run 的 actor/reason 必须为空。每天 03:00 Asia/Shanghai 调度，River leader 启动时若 24 小时无成功 Run 则触发 catch-up。
- manual、daily 与 catch-up 共用同一 transaction-bound start seam。DB advisory lock、active-run unique、scheduled slot unique 与 `enqueue_required` 返回值保证多实例/重复调度只 enqueue 一次；Run insert/reset 与 River enqueue 必须同 transaction。ScheduleJob 遇到其他 active Run 时必须 snooze，不能吞掉目标 slot；仍 overdue 的 failed target slot 复用原 Run、清理终态字段并重新入队。
- `warning/low`（长期 Hold，或 Billing Snapshot、Settlement、Hold、terminal Ledger 全部完整但 Task trace 缺失）只报告，不限制 Wallet。Task trace 只有在 Hold/terminal command amount、Posting debit/credit shape 与 Snapshot total 全部一致时才能降为 low；其余缺失或不一致均为 `error|critical/high` 资金事实风险。高风险 Finding 会在 run report 同一 transaction 内幂等创建绑定该 Finding 的 `ledger_integrity/reconciliation` Restriction。Finding resolved、降为 low、失去或改变 Wallet 归属时只精确解除其自身 Restriction；Reconciliation alerts 已删除。
- Finding fingerprint 不包含 run ID；重复异常复用 open Finding 并增加 occurrence/run link，异常消失后标记 resolved，reopen 会创建新的 Restriction history row。Finding 的 `isolated` 从精确 active Restriction 推导；扫描不修改 Wallet lifecycle status、Ledger、projection，也不自动创建 Correction。
- Run-Finding link 仅保留内部 historical snapshot 和审计关联；Admin Finding read contract 只有全局 open/resolved queue，不提供 Run-scoped 双轨查询。
- Identity 通过 typed Wallet lifecycle seam 原子 provision Wallet、冻结/恢复 lifecycle status，并以 trusted canonical registration identity 的 keyed HMAC claim 幂等发放 `100 Ideas` Welcome Grant；Ban/Unban/删除不得创建或清除独立 Restriction，claim identity 明文不进入 Credits schema。
- Welcome identity HMAC key 首次使用时通过 typed `SECURITY DEFINER` command 永久绑定数据库 fingerprint；配置误变更时 startup/provision fail closed，避免同一身份生成新 claim。
- 删除业务用户先在本地事务冻结 Wallet 并记录 durable deletion phase，再依次幂等删除 Auth、清理 Storage、写 user tombstone；失败请求可从持久 phase 重试收敛。最终保留 user tombstone 主键、Task、Wallet、Ledger 与 Welcome claim；相同领取身份重新注册不会再次获得 Welcome Grant。

## Verification

默认 suite 使用 `internal/testdb` 的隔离 PostgreSQL schema 验证 Commands、HTTP contract、并发锁、limited runtime role 和数据库 constraints。百万数据集 harness 由 migration owner/test DSN 通过 `CREDITS_LOAD_TEST=1` 显式开启，命令和基准结果见 `docs/benchmarks/credits-million.md`。
