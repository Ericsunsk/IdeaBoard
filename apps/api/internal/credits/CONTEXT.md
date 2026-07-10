# Credits Context

Credits 是 IdeaBoard 的平台级封闭生态货币，用户文案统一显示为「点子」。本 context 只拥有货币账户、不可变账本、余额 projection 和 Credit Commands；商品身份与价格属于后续 `billing` context。

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

## Public Seams

- `Commands` 是唯一应用写入入口，统一提供 Grant、Hold、Capture、Release 与 Refund；底层只调用 typed `SECURITY DEFINER execute_credit_command`。
- `Service.InTx(pgx.Tx)` 允许 Tasking 将 Credit Command 放入已有的 Task/River PostgreSQL transaction；caller 拥有 commit/rollback。
- `WalletReader` 提供 Wallet balance 与 keyset-paginated Ledger 查询。
- Admin HTTP 只提供 #244 所需的单用户 Grant、Wallet read 和 Ledger read；完整调账与 UI 属于后续 tickets。

其他 context 不得直接更新 `credit_accounts.balance`、插入 Ledger 或自行计算幂等 hash。

## Invariants

- `CreditAmount` 为正整数，单次命令上限为 `9_000_000_000_000_000`；available/reserved 永不为负。
- Ledger Transaction 支持 N postings。PostgreSQL deferred constraint 在 commit 时校验 posting count、单一 asset 和总和为 0。
- Ledger Transaction 与 Posting 禁止 UPDATE/DELETE；错误只能追加 reversal/correction transaction。
- Posting statement 按 `account_id` 稳定顺序更新 projection，Command 在写入前也按相同顺序锁账户。
- API runtime role 对 Credit tables 只有 SELECT，不能直接 DML Ledger/projection，也不能执行内部 trigger functions；migration owner 才拥有 schema。
- System Issuance、Revenue 与 Sink 使用 `wallet:<user_id>` shard，并保留 role/metadata，避免单个全局 system row 串行化所有 Wallet。
- 每个 Command 先取得 idempotency-key advisory transaction lock，再比较服务端 canonical SHA-256 request hash。相同 key/相同 hash 返回原结果；不同 hash 返回 conflict。
- actor、source、reason 和 business reference 必填并写入不可变 Transaction。
- Auth public registration 只接受 username/password，由服务端 canonicalize username、生成 synthetic claim identity，并在受控命令内标记为 `trusted_canonical_registration`；直接 Better Auth email sign-up 被拒绝。
- Identity 通过 typed Wallet lifecycle seam 原子 provision Wallet、冻结/恢复 Wallet，并以 trusted canonical registration identity 的 keyed HMAC claim 幂等发放 100 点子 Welcome Grant；claim identity 明文不进入 Credits schema。
- Welcome identity HMAC key 首次使用时通过 typed `SECURITY DEFINER` command 永久绑定数据库 fingerprint；配置误变更时 startup/provision fail closed，避免同一身份生成新 claim。
- 删除业务用户先在本地事务冻结 Wallet 并记录 durable deletion phase，再依次幂等删除 Auth、清理 Storage、写 user tombstone；失败请求可从持久 phase 重试收敛。最终保留 user tombstone 主键、Task、Wallet、Ledger 与 Welcome claim；相同领取身份重新注册不会再次获得 Welcome Grant。

## Verification

默认 suite 使用 `internal/testdb` 的隔离 PostgreSQL schema 验证 Commands、HTTP contract、并发锁、limited runtime role 和数据库 constraints。百万数据集 harness 由 migration owner/test DSN 通过 `CREDITS_LOAD_TEST=1` 显式开启，命令和基准结果见 `docs/benchmarks/credits-million.md`。
