# ADR-0001: PostgreSQL owns the Credits ledger

- Status: Accepted
- Date: 2026-07-11

## Context

「Idea」是 IdeaBoard 后续产品共用的 Closed-loop Virtual Credit。首个消费方会是 AI image workflows，但 Wallet 和 Ledger 必须独立于具体产品。

Task submission 已要求 business Task、Credit Hold 和 River enqueue 处于同一 PostgreSQL transaction；Task 成功与 Capture 也需要原子提交。外部 ledger service 会把这一边界改造成 Saga/TCC，并引入当前不需要的补偿与 dual-write 风险。

## Decision

- Credits Ledger 与 Tasking 共用项目的 PostgreSQL 17，`credits` context 单独拥有 schema 与 Command interface。
- 使用 Double-entry Ledger；每个 Transaction 支持 N 条 immutable postings，并在同一 asset 内严格平衡为 0。
- `credit_accounts.balance` 是由 posting trigger 维护的 O(1) projection，Ledger 才是事实源。
- Grant、Hold、Capture、Release、Refund 与 Purchase 都通过集中 Command interface；业务 transaction 通过 `Service.InTx(pgx.Tx)` 原子组合。
- 每个 Command 使用全局 idempotency key 与服务端 canonical request hash；PostgreSQL advisory transaction lock 和 unique constraint 共同防止并发重复执行。
- User available/reserved 不得为负。账户锁按稳定 ID 顺序获取。
- System Issuance、Revenue 和 Sink 按 Wallet shard，避免全局 system account 热行，同时保留可聚合的 account role 与 metadata。
- Ledger rows 不得修改或删除；修正必须追加 reversal/correction Transaction。
- 禁止把外部 billing/payment/ledger 作为 Idea 余额的第二事实源。支付确认后由 Payments 调用 typed Purchase Command 原子入账；Purchase Credit 与自主奖励的 Grant 分离。
- PostgreSQL schema owner/migration role 与 API runtime role 分离。Runtime 对 Credit tables 只有 SELECT，只能在自己的 transaction 内执行固定-search-path 的 typed `SECURITY DEFINER` Command function。

## Consequences

- Tasking 可以保持当前 PostgreSQL + River transactional contract，不需要外部分布式事务。
- 百万级事实表仍可使用 keyset pagination 和 account projection；达到 PostgreSQL 的实际吞吐瓶颈后，再通过新 ADR 评估独立 Ledger service。
- Owner credential 只进入 API/Auth migration processes，不进入长期运行的 API/Auth；owner/superuser 的运维级能力不能作为应用写路径。
- `billing` 负责 SKU Catalog、versioned pricing 和 Quote，不得复制 Wallet 或 Ledger。

## Rejected Alternatives

- Formance/TigerBeetle：会破坏当前单 transaction 原子性并增加独立服务运维。
- Stripe/Lago/OpenMeter：适合 Payment、Invoice 或 Usage Billing，不适合作为内部生态货币事实源。
- 单行余额或 append-only 单边流水：无法结构化证明守恒，也难以可靠审计冻结、结算和退款。
