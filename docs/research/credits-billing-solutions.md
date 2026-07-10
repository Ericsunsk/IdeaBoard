# Credits / Billing 成熟方案调研

> 调研快照：2026-07-11。本文是技术选型研究，不是 ADR，也不是实现事实源。

## 结论

IdeaBoard 当前最优方案是：**在现有 PostgreSQL 中实现小而专用的 Credits Double-entry Ledger，并由项目自己的 Billing Module 管理 SKU Catalog 与 versioned pricing。第一版不引入 Lago、OpenMeter、Formance、TigerBeetle 或 Stripe Billing Credits 作为 Credits 事实源。**

原因不是“少改代码”，而是目前没有成熟产品同时满足以下组合：

- 与 Task insert、Credit Hold、River enqueue 共享同一个 PostgreSQL transaction；
- 与 Task 成功状态、Credit Capture 共享同一个 PostgreSQL transaction；
- 原生支持 Hold / Capture / Release / Refund；
- 使用整数、封闭生态 Credits，而不是法币 invoice balance；
- 提供符合 IdeaBoard 的全局 SKU、固定 Credit 定价和历史价格快照；
- 不引入额外分布式一致性与运维系统。

成熟的外部产品各自解决了其中一部分，但都会把当前可证明的本地原子性改造成 Saga / TCC 最终一致性。对尚未上线、第一版不接真实支付、交易规模远未超过 PostgreSQL 能力的 IdeaBoard，这不是长期收益更高的交换。

建议把 [pgledger](https://github.com/pgr0ss/pgledger) 当作 PostgreSQL 并发控制、账户锁顺序和 balanced transfer 的参考实现，不把它直接作为不可替换的依赖。它目前仍是 `v0.7.0`，缺少强制幂等 Command、显式 Hold 状态机和可升级 migrations；其 README 也把 versioned views/functions 与 migration series 列在 TODO 中。[来源：README](https://github.com/pgr0ss/pgledger#todo)、[v0.7.0 release](https://github.com/pgr0ss/pgledger/releases/tag/v0.7.0)

## IdeaBoard 的选择约束

本次评估按已经确认的产品与系统 contract：

- 「点子」在代码中称为 `credits`，是整个 IdeaBoard 生态的 Closed-loop Virtual Credit。
- 用户独立持有钱包，Credits 不提现、不交易、不转账、不产生利息。
- Double-entry Ledger 是事实源；余额是投影，不允许直接覆盖。
- 所有 Grant、Hold、Capture、Release、Refund、Admin adjustment 都必须强制幂等。
- AI Task 提交时锁定 SKU price snapshot 并冻结 Credits。
- Task 成功时 Capture，失败、取消或超时 Release；Hold 不独立按 TTL 自动释放。
- 未配置价格 fail closed，显式 `0` Credits 才免费。
- 当前 Task insert 与 River enqueue 必须在同一个 PostgreSQL transaction 中。
- 目标设计要求 Task 成功状态与 Capture 原子提交。
- 第一版不接真实支付，不需要 subscription、invoice、tax、dunning 或 revenue recognition。

## 能力矩阵

符号：✅ 原生支持；◐ 可组合或只支持相邻语义；❌ 不适配；`N/A` 不属于该产品职责。

| 方案 | Double-entry | Prepaid Credits | Hold / Capture / Release | 幂等 | SKU / versioned pricing | Self-host | Go 集成 | 与 IdeaBoard PostgreSQL 共事务 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Custom PostgreSQL | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ pgx/sqlc | ✅ |
| pgledger | ✅ | ◐ 需建账户模型 | ◐ 用 reserved account 组合 | ❌ 需自行补 | ❌ | ✅ | ✅ SQL/pgx | ✅ |
| Lago | ❌ 未公开为通用复式账本 | ✅ Wallet | ❌ invoice finalization 才消费 | ◐ usage event 有去重，Wallet Command 未见通用幂等 contract | ◐ Plans/pricing units，偏 invoice | ✅ | ✅ REST/Go SDK | ❌ 独立服务 |
| OpenMeter | ◐ 内部 Credit Ledger，公共 Credits API 仍 unstable | ✅ | ❌ 无公共 reservation lifecycle | ◐ Grant key / event dedup | ✅ versioned Plans/Rate Cards | ✅ | ✅ 官方 Go SDK | ❌ 独立服务及多项依赖 |
| Formance Ledger CE | ✅ | ◐ 需自行建钱包账户 | ◐ 用账户转移建模 | ✅ | ❌ | ✅ | ✅ 官方 Go SDK | ❌ 独立服务 |
| Formance Wallets EE | ✅ 基于 Ledger | ✅ | ✅ hold/confirm/void/partial confirm | ✅ 底层 Ledger | ❌ | ✅ 需商业许可 | ✅ REST/Go SDK | ❌ 独立服务 |
| TigerBeetle | ✅ debit/credit transfer | ◐ 需自行建钱包与 Grant | ✅ pending/post/void | ✅ Transfer ID | ❌ | ✅ | ✅ 官方 Go client | ❌ 独立数据库 |
| Stripe Billing Credits | ❌ 账本不可作为通用 postings API | ✅ monetary billing credits | ❌ invoice finalization 模型 | ✅ POST idempotency | ✅ Stripe Product/Price，但限 invoice billing | ❌ SaaS | ✅ 官方 Go SDK | ❌ 外部 SaaS |

## 逐项评估

### Lago

Lago 是成熟、活跃的 open-source usage billing platform；官方仓库在调研时最新稳定版为 [`v1.50.0`](https://github.com/getlago/lago/releases/tag/v1.50.0)，采用 AGPL-3.0，可 self-host。[来源：官方仓库](https://github.com/getlago/lago)

适合的部分：

- 支持 customer Wallet、paid/free credits、余额、top-up、void、expiration、usage scope 和 consumption/funding traceability。[Wallet overview](https://docs.getlago.com/guide/wallet-and-prepaid-credits/overview)、[top-up and void](https://docs.getlago.com/guide/wallet-and-prepaid-credits/wallet-top-up-and-void)、[traceability](https://docs.getlago.com/guide/wallet-and-prepaid-credits/traceability)
- 支持 Plans、Billable Metrics、charge models 与 custom pricing units。[Custom pricing units](https://docs.getlago.com/guide/plans/custom-pricing-units)
- 官方 Docker self-host 使用 PostgreSQL，并另有 Redis/worker 等组件。[Self-host Docker](https://docs.getlago.com/guide/lago-self-hosted/docker)

不适合 Credits Core 的部分：

- Wallet Credits 的主要语义是抵扣 subscription/progressive billing invoice；官方文档明确余额在 invoice finalized 时更新，且 prepaid credits 不适用于 one-off invoice。这不是 Task submit 时立即 Hold、Task terminal 时 Capture/Release 的生命周期。[Wallet overview](https://docs.getlago.com/guide/wallet-and-prepaid-credits/overview)
- Void 是即时扣除且不可撤销，不等价于关联原 Charge 的受限 Refund。[Top-up and void](https://docs.getlago.com/guide/wallet-and-prepaid-credits/wallet-top-up-and-void)
- 官方公开 contract 没有把 Wallet 暴露为可编排的 Double-entry postings，也没有 Task-scoped Hold/Capture API。
- 即使与 IdeaBoard 共用同一 PostgreSQL server，Lago 仍是独立应用与独立 transaction owner，IdeaBoard 的 pgx transaction 不能包含 Lago API 操作。

结论：**适合未来真实支付、subscription、invoice、tax 与 dunning 需求，不适合作为当前生态 Credits Ledger。**

### OpenMeter

OpenMeter 是 active、Apache-2.0 的 Go billing/metering platform，提供 Metering、Product Catalog、Billing、Entitlements、Prepaid Credits 与官方 Go SDK。[官方 README](https://github.com/openmeterio/openmeter#readme)

适合的部分：

- Credit Grant 支持 promotional/paid funding、priority、expiration、feature filter 和创建 key；Customer Credit Transaction 被定义为 immutable。[Credit Grant source](https://github.com/openmeterio/openmeter/blob/main/api/spec/packages/aip/src/customers/credits/grant.tsp)、[Credit Ledger API source](https://github.com/openmeterio/openmeter/blob/main/api/spec/packages/aip/src/customers/credits/ledger.tsp)
- Product Catalog 的 Plan 明确 versioned，避免修改运行中 subscription；Rate Card 支持 `credit_only` settlement。[Plan source](https://github.com/openmeterio/openmeter/blob/main/api/spec/packages/aip/src/productcatalog/plan.tsp)、[Rate Card source](https://github.com/openmeterio/openmeter/blob/main/api/spec/packages/aip/src/productcatalog/ratecard.tsp)
- Self-host 与 Go SDK 都是官方能力。

不适合当前 Credits Core 的部分：

- 调研时最新 release 仍为 [`v1.0.0-beta.230`](https://github.com/openmeterio/openmeter/releases/tag/v1.0.0-beta.230)。Credits create/get/list/balance/transaction/adjustment endpoints 在官方 TypeSpec 中均带 `Shared.UnstableExtension`，不能视为稳定公共 contract。[Operations source](https://github.com/openmeterio/openmeter/blob/main/api/spec/packages/aip/src/customers/credits/operations.tsp)
- 公共 Credit lifecycle 是 funded/consumed/expired，没有 Task-scoped Hold/Capture/Release API。
- Product model 主要围绕 Plan、Subscription、Metered usage 和 invoice settlement，不是简单的全局 `SKU × integer quantity` 即时消费。
- 官方 quickstart 需要 PostgreSQL、Kafka、ClickHouse、Redis、Svix 和多个 worker，远高于 IdeaBoard 第一版所需的 operational footprint。[Quickstart Compose](https://github.com/openmeterio/openmeter/blob/main/quickstart/docker-compose.yaml)
- 它是独立服务，无法加入 IdeaBoard Task/River 的本地 transaction。

结论：**如果未来商业模式转向 usage metering + subscription + invoice，可重新评估；现在不应把 unstable Credits API 放在核心货币路径。**

### Formance Ledger / Wallets

Formance Ledger 是本次调研中最接近“成熟通用 Ledger Service”的方案。Ledger CE 是 MIT、Go、PostgreSQL-backed，调研时最新 release 为 [`v2.3.21`](https://github.com/formancehq/ledger/releases/tag/v2.3.21)。官方将 transaction 定义为永久记录的 atomic multi-posting container，余额由 posting history 派生。[Postings and Transactions](https://docs.formance.com/modules/ledger/core-concepts/transactions)

适合的部分：

- 原生 atomic multi-posting 与 programmable Numscript。
- 所有 write endpoints 支持 `Idempotency-Key`；相同 key/相同请求返回原结果，不同参数返回 validation error；另有业务 `reference` 唯一键。[Idempotency](https://docs.formance.com/modules/ledger/working-with/idempotency)
- 官方 Go SDK 活跃维护。[Go SDK](https://github.com/formancehq/formance-sdk-go)
- CE Ledger self-host 主要依赖 PostgreSQL；官方生产部署推荐 managed PostgreSQL。[Ledger deployment](https://docs.formance.com/deploy/self-hosted/modules/ledger)、[PostgreSQL deployment](https://docs.formance.com/deploy/self-hosted/infrastructure/postgresql)

Wallets 的额外能力：

- 原生 Hold、Confirm、Void、partial confirm，并可在 final confirm 时自动释放剩余金额。[Managing holds](https://docs.formance.com/modules/wallets/managing-holds)、[Holding and confirm](https://docs.formance.com/modules/wallets/holding-confirm)
- 但 Wallets 是 Enterprise Edition。官方源码许可证只允许无许可证的 development/testing；production 使用需要 Enterprise/Subscription Agreement 与有效 license。[Wallets license](https://github.com/formancehq/wallets/blob/main/LICENSE)、[Self-host Wallets](https://docs.formance.com/deploy/self-hosted/modules/wallets)

不适合当前单库路线的部分：

- Formance Ledger/Wallets 是独立 transaction owner。使用同一 PostgreSQL 实例不等于共享 transaction。
- Ledger 不提供 IdeaBoard 的 SKU Catalog、price snapshot、AI feature/model mapping 或 Task state integration。
- 只用 CE 时，需要用 `user:available -> task:reserved -> system:revenue` 等账户自行实现 Hold semantics；购买 EE 才获得 opinionated Wallet API。
- 引入后必须把原子 Task/Billing contract 改造成 Saga/TCC，并增加 reconciliation、retry、stuck settlement 和外部服务 outage 处理。

结论：**若未来明确接受独立 Ledger Service，Formance Ledger CE 是第一 shortlist；若愿意付费并希望少写 Wallet lifecycle，则评估 Wallets EE。**

### TigerBeetle

TigerBeetle 是专用 Financial Transactions Database，Apache-2.0，官方提供 Go client；调研时最新 release 为 [`0.17.9`](https://github.com/tigerbeetle/tigerbeetle/releases/tag/0.17.9)。[Official clients](https://docs.tigerbeetle.com/coding/clients/)

适合的部分：

- Two-phase Transfer 原生支持 pending reserve、partial/full post、void 和 optional timeout；pending 只能 resolve 一次，所有 Transfer immutable。[Two-phase Transfers](https://docs.tigerbeetle.com/coding/two-phase-transfers/)
- Account invariants 可在 pending 阶段阻止超扣。
- Transfer ID 的主要用途就是 idempotency；重复相同 ID 可识别为 existing，字段不同会返回精确 conflict。[Data Modeling IDs](https://docs.tigerbeetle.com/coding/data-modeling/#id)、[Create Transfers results](https://docs.tigerbeetle.com/reference/requests/create_transfers/)
- 单一静态 binary、无外部 runtime dependency；自身提供复制与一致性。[Deploying](https://docs.tigerbeetle.com/operating/deploying/)

不适合当前阶段的部分：

- 它不是 PostgreSQL extension，而是独立数据库，绝无可能加入 Task/River 的 pgx transaction。
- 它只解决 financial transfers，不解决 Customer/SKU/Pricing/Admin workflow；仍需在 PostgreSQL 维护业务模型与 query projection。
- 官方 production recommendation 是每台专用机器一个 replica、六 replicas 加 supervisor，或使用 managed service；这对当前 IdeaBoard 明显过重。[Deploying](https://docs.tigerbeetle.com/operating/deploying/)
- Schema 是高性能固定结构，只有有限 user data fields；审计展示、metadata 查询与 reconciliation projection 需要额外 PostgreSQL 模型。

结论：**只有在 Credits throughput、availability 或跨产品规模真正超过 PostgreSQL 后才值得引入；现在属于正确但过度的技术。**

### Stripe Billing Credits

Stripe Billing Credits 是成熟 SaaS 能力，支持 prepaid/promotional Credit Grants、expiration、priority、append-only balance ledger 与全局 POST idempotency。[Billing Credits](https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits)、[Idempotent requests](https://docs.stripe.com/api/idempotent_requests)

不适合当前 Credits Core 的关键限制：

- Credit Balance API 当前只支持 `monetary`，且必须使用 Stripe 支持的 ISO currency。[Credit Balance Summary](https://docs.stripe.com/api/billing/credit-balance-summary/object)
- Credits 只应用于使用 Stripe Meters 上报 usage 的 metered subscription items，并在 invoice finalization 时真正应用；不能用于普通 one-off invoice 或任意即时 Task transaction。[Billing Credits eligibility](https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits#credit-grant-eligibility)
- 没有 Task-scoped Hold/Capture/Release。
- 不 self-host，也不可能与 IdeaBoard PostgreSQL transaction 原子提交。
- Stripe 文档禁止将 Billing Credits 作为一般 stored value、第三方支付或数字钱包链接；IdeaBoard 的 closed-loop 产品消费本身不冲突，但 Stripe 的能力边界仍是 Stripe Billing，而不是项目内通用 Ledger。[Approved/prohibited uses](https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits#use-cases-for-billing-credits)

结论：**未来可作为真实支付与 invoice 系统，不应成为「点子」事实源。支付成功后，由 webhook/outbox 触发内部 Credits Grant。**

### Go / PostgreSQL 可嵌入 Ledger Libraries

没有找到同时成熟、可作为 Go library 直接嵌入、PostgreSQL-backed，并完整提供本项目 Hold/Capture/Release/Refund、幂等和 SKU pricing 的可靠方案。

最接近的是 `pgledger`：

- 全部实现在 PostgreSQL tables/functions/views 中，可以与应用业务写入共享 transaction。[README](https://github.com/pgr0ss/pgledger#why-pgledger)
- 提供 balanced transfers、atomic multi-transfer、固定顺序锁账户以降低 deadlock、禁止负/正余额约束、immutable entries 与 metadata。[SQL source](https://github.com/pgr0ss/pgledger/blob/main/pgledger.sql)
- 示例使用独立 `pending_outbound` account 表达 in-flight funds，说明 Hold 可以通过 chart of accounts 建模。[Reconciliation example](https://github.com/pgr0ss/pgledger/blob/main/examples/reconciliation.sql)

但它不应被当作完整 Credits dependency：

- 当前 `v0.7.0`，项目创建于 2025 年，成熟度低于 Formance/TigerBeetle。
- 没有 Command idempotency key、参数 hash conflict、Hold object/state、Capture/Release 关联约束或 Refund ceiling。
- 使用任意 `NUMERIC`，而 IdeaBoard 已决定 Credits 为正整数。
- migration series 和 versioned views/functions 仍在 TODO，直接跟随 upstream 升级有 schema risk。
- 不包含 SKU、price version、quote snapshot 或 Admin audit。

因此建议：**借鉴或有选择地 vendor 少量 SQL pattern，但最终 schema、migrations 与 domain API 归 IdeaBoard 所有。**

## 路线一：保留单 PostgreSQL 原子事务

这是最终推荐路线。

建议结构：

```text
Billing: SKU + price version + quote snapshot
                 |
                 v
Credits: Wallet + Ledger Transaction + balanced Postings
                 |
                 v
PostgreSQL transaction: Task + Hold + River Job
PostgreSQL transaction: Task succeeded + Capture + Outbox/Event
```

最低可持续模型：

- `credit_accounts`：用户 available/reserved、system issuance/revenue/refund 等账户；金额不直接作为可编辑事实。
- `credit_transactions`：不可变 business transaction，保存 command type、business reference、actor、reason、idempotency key 和 request hash。
- `credit_postings`：每笔 transaction 至少两个 posting，同一 asset 的 signed amount 合计为 0。
- `credit_balances`：可锁定的余额 projection；通过同一 transaction 内 posting 更新，定期与 Ledger 聚合 reconciliation。
- `credit_holds`：业务生命周期对象，关联 Task、quote snapshot、hold/capture/release transaction，并用 unique constraints 保证单次 transition。
- `billing_skus`、`billing_price_versions`、`billing_quotes`：全局不可复用 SKU、versioned integer price 与提交时 snapshot。

关键数据库保障：

- `BIGINT` 或足够范围的 integer amount；禁止 floating point。
- 所有 Command 必须有唯一 `(scope, idempotency_key)`，并保存 canonical request hash；相同 key/相同参数返回原结果，不同参数报 conflict。
- 账户按稳定顺序 `SELECT ... FOR UPDATE`，避免并发提交超扣和常见 deadlock。
- Posting balance、正金额、合法 transition、退款上限由 constraint/trigger/受控 SQL function 与 Go service 双层校验。
- 应用 runtime role 禁止直接 `INSERT/UPDATE/DELETE` Ledger tables，只允许调用受控 repository/function；migration/admin role 才能 DDL。
- NATS 仍只发布 commit 后事件，不成为账务事实源。

这条路线保留现有 Tasking invariant，也能兑现已经确认的 Billing 原子性，无需新 ADR 改写一致性模型；但 Credits 作为跨 context 的 durable technology/domain boundary，仍建议新增一篇 ADR 记录 Double-entry、single-PostgreSQL ownership 与禁止外部 service 双写。

## 路线二：拆成独立 Ledger Service

如果未来愿意明确放弃本地 transaction atomicity，可选顺序是：

1. **Formance Ledger CE**：长期通用性、可审计性、幂等与 Go/PostgreSQL 生态最均衡；Hold 用账户建模。愿意购买生产 license 时再评估 Wallets EE。
2. **TigerBeetle**：当实际 throughput/availability 指标证明 PostgreSQL Ledger 是瓶颈时采用；它的 two-phase transfers 最强，但业务 projection 与运维成本最高。
3. **OpenMeter**：仅当产品商业模式演化为 Metering + Subscription + Invoice，且 Credits API 离开 unstable/beta 后重新评估。

Lago 与 Stripe 更适合作为未来外部 Billing/Payment system，不适合作为内部 Credits Ledger。

### 必须采用的分布式流程

独立 Ledger 不能靠“两个系统都用 PostgreSQL”获得原子性。合理方式是 TCC/Saga + Outbox + 全局幂等：

1. 根据 `client_action_id` 与 quote 生成确定性 Hold idempotency key。
2. 先在 Ledger Service 建立 Hold；余额不足则终止，不创建 Task。
3. 在 IdeaBoard PostgreSQL transaction 中写 Task、Ledger reference 和 River Job。
4. 如果第 3 步失败，通过幂等 compensation Release Hold；reconciliation 负责清理未绑定 Task 的 Hold。
5. Worker 得到 Provider result 后先持久化为不可见的 `settlement_pending` result。
6. 通过 outbox 请求 Capture；成功后再在 PostgreSQL 中把 Task 标记为 `succeeded` 并发布可见事件。
7. 任一步超时都由相同 idempotency key 重试，不能假定 HTTP timeout 等于失败。

### 对现有 contract 的影响

- **Task insert + River enqueue 同 PostgreSQL transaction 可以保留。**
- **Task + Credit Hold 同 transaction 必须废弃**，替换为“先 Try Hold，再原子创建 Task/Job，失败时 Cancel”的 TCC contract。
- **Task succeeded + Credit Capture 同 transaction 必须废弃**，替换为 `settlement_pending -> captured -> succeeded` 的可恢复 Saga。
- 必须增加 external ledger outage、orphan Hold、captured-but-task-not-succeeded、released-but-task-running 等 reconciliation cases。
- 必须新增 ADR，并更新 `DESIGN.md` 的 Credits/Tasking cross-context contract；这是 data ownership 与 consistency model 的 system-wide change，不是实现细节。

### 分布式路线的长期代价

- 多一个核心有状态服务及 backup/restore/upgrade/on-call surface。
- 失去数据库 foreign key 与单 transaction invariant，测试从 ACID assertions 变成 failure matrix。
- 用户可见成功需要等待 settlement convergence，或接受短暂账实不一致。
- Debug 需要串联 Task ID、idempotency key、external transaction ID、outbox event 与 reconciliation report。
- 即使购买 Ledger/Wallet 产品，SKU、price version、quote、AI mapping、Admin policy 和 user-facing terminology 仍要自建。

## 最终 Build-vs-Buy Recommendation

**Build Credits Core，Buy/Integrate future Payment/Billing edges。**

- 现在：在 IdeaBoard PostgreSQL 内实现专用 Credits Double-entry Ledger 与 Billing SKU Catalog。
- 参考：吸收 pgledger 的 ordered locking、atomic multi-transfer、balance constraints 与 reconciliation patterns，但不直接绑定其未稳定 schema。
- 未来收真钱：Stripe/支付宝/微信等只负责 payment order；支付确认通过幂等 Command 向内部 Ledger Mint/Grant Credits。
- 未来 subscription/invoice 变复杂：在边缘接 Lago、OpenMeter 或 Stripe Billing；内部「点子」仍保持自己的事实源。
- 未来真实指标证明单 PostgreSQL 无法满足 throughput/availability：执行 ADR 驱动的 Ledger Service extraction，首选 Formance Ledger CE；极高规模再考虑 TigerBeetle。

这条路径既没有锁死未来外部化，也避免在最早阶段为尚不存在的规模问题牺牲最重要的账务原子性。

