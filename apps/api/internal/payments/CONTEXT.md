# Payments Context

Payments 拥有 Stripe 配置、签名 Webhook 接入与支付生命周期。它不拥有平台 Idea 余额或账本；Wallet、Double-entry Ledger、余额 projection 与所有 Credit 写入仍由 Credits 独占。

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Current Stripe Configuration | 当前唯一生效的 Stripe account、Product Tax Code、购买开关与 provider resource references；Secret 只保存密文 |
| Stripe Gateway | Payments 调用 Stripe API 的 adapter seam；domain service 不依赖 Stripe SDK 类型 |
| Webhook Inbox | 以 Stripe Event ID 全局去重的最小接收事实，只保存路由与处理所需 metadata，不保存原始 payload |
| Ignored Event | 签名有效但当前 Payments 不识别的事件；记录为 `ignored` 并成功确认，不触发业务状态变更 |
| Payment Admin Action | Stripe 配置操作的 append-only audit，包含 actor、outcome 与脱敏错误摘要 |
| Payment Order | 用户一次充值的 durable、幂等意图与最终支付事实；public token 仅用于已认证用户查询，绝非支付事实 |
| Checkout Return | Stripe browser return 的 UI 导航信号；服务端必须重新读取 Stripe Checkout Session，只有已支付的 canonical Stripe fact 才可到账 |
| Purchase Credit | 由已验证 Stripe Payment Order 触发的 `System Issuance → User Available` Credits 命令；不是 Admin Grant |
| Payment Reconciliation | River 定期读取 durable Payment Order 后向 Stripe 主动核验、收敛丢失 Webhook 与短暂故障的恢复流程 |
| Reconciliation Claim | 一对一的 Payment Order recovery state；token + generation fence 保护短租约，Stripe I/O 永远发生在 claim transaction 之外 |
| Payment Refund | Admin 或 Stripe Dashboard 发起的 durable 退款事实；本地先预留未消费 Ideas，再由 canonical Stripe status 幂等结算或释放 |
| Purchase Refund Hold | 独立 Credits typed Command，将未消费 Ideas 从 User Available 预留到 User Reserved；它不是消费，可在 active Restriction 下执行 |
| Purchase Refund | 独立 Credits typed Command，将退款预留从 User Reserved 结算回 System Issuance，不复用 Admin Refund |

## Public Seams

- `Service.GetStripeConfiguration` 只返回可安全展示的配置摘要，不返回 API Key、Webhook Secret 或其密文。
- `Service.ConfigureStripe` 在启用或变更 Stripe 配置时先通过 `StripeGateway` 准备并验证 Stripe account、Product 与 Webhook endpoint，再原子发布当前配置和成功 audit；准备失败时保留旧配置。
- `purchases_enabled` 从 true 切为 false 时只提交本地 fast path，并在同一 transaction 写入 durable Checkout shutdown run；River 按数据库 cutoff 幂等 Expire 存量 Session，periodic recovery 负责 enqueue/进程故障后的续跑。
- `Service.ReceiveStripeWebhook` 对 raw body 验签、去重并写入最小 Webhook Inbox。后续 payment order、refund 与 dispute lifecycle 继续收口在 Payments context。
- `Service.GetPurchaseOptions`、`CreateCheckout`、`ListPendingOrders`、`ContinuePendingOrder`、`AbandonPendingOrder`、`GetPaymentOrderReturnStatus` 与 `ListRecharges` 是用户充值 boundary。`ListRecharges` 保留已到账、已退款与用户主动放弃的订单事实；HTTP handler 只能使用 authenticated principal 的 user ID，并为订单、历史和 SSE 设定 user scope。
- `Service.EventHub` 只发布本实例已提交的用户充值状态提示；前端据此重新读取 Wallet、订单或充值历史，SSE 不是支付事实源。
- `Service.ReconcileDuePaymentOrders` 是 River recovery seam，并收敛 Checkout、Refund 与 durable Dispute；`ListPaymentReconciliationAnomalies`、`ListAdminPaymentOrders` 与 `GetAdminPaymentOrder` 是 Admin read seams，HTTP/Admin presentation 不拥有对账 transition。Admin detail 只返回脱敏 Stripe identity、durable recovery facts 和安全 Dashboard URL。
- 任何支付结果引起的 Idea 余额变化都必须调用 Credits 的 typed Commands；Payments 不直接写 Ledger 或 balance projection。

## Invariants

- Stripe API Key 与 Webhook Secret 使用平台 Secret material 加密存储，禁止进入 HTTP response、日志、Webhook Inbox 或 Admin audit。
- Payments 只持久化收敛和审计所需的最小 Stripe IDs、金额、币种、状态、时间、收据 URL、条款版本与受限错误摘要；禁止持久化卡信息、完整账单地址、完整 raw payload、Secret 明文或内部 stack trace。Payment 事实的 retention 由受控运维流程决定，不得手工删除关联 Ledger 或未结算状态。
- Checkout 不启用 Stripe Automatic Tax，也不主动要求或保存 Customer 地址；Stripe 或具体 Payment Method 因合规要求收集的最少 Billing 信息不属于应用可屏蔽范围。
- Webhook 必须在业务解析和持久化前使用 raw body、当前 Webhook Secret 与 5 分钟 tolerance 验证签名；无效或过期签名不写 Inbox。
- Stripe Event ID 在全局唯一。重复投递保持幂等；已签名但未知的事件只记录最小 metadata 并标记 `ignored`。
- Webhook Inbox 不保存原始 payload，只保存 Event ID、type、object ID、livemode、Stripe created time、接收/处理状态与时间。
- 配置与 Webhook Secret 在 request-time 从 PostgreSQL 读取并实时生效，不使用 application-lifetime 配置缓存。
- 创建 Checkout 前必须先以 `(user_id, client_idempotency_key)` 和 canonical request hash durable 创建或重放 Payment Order；相同 key 的不同请求 fail closed。Stripe Customer、Checkout Session 和其 provider identity 都必须绑定到该 Order。
- Stripe 明确拒绝创建 Customer 或 Checkout Session 时，`creating` Order 必须立即进入 `payment_failed`，允许用户用新 Order 重试；网络中断、限流、provider `5xx`、`idempotency_error` 或响应不完整等结果未知失败必须保留原 Order 与原 Stripe Idempotency Key。
- Payment Order 生命周期以 `creating → checkout_open → async_payment_pending` 表达未完成 Checkout，以 `cancelled|expired|payment_failed` 表达未支付终态，以 `paid_pending_credit → credited` 表达已确认支付到 Ledger 到账。已确认的 late paid fact 可以从未支付终态升级到 `paid_pending_credit`，其他终态不得回退；所有 transition 由数据库约束。
- 同一用户、同一 Idea 数量只允许一个未支付的 active Checkout；复用 `checkout_open` 或恢复 `creating` 前必须读取或收敛 Stripe canonical state，禁止返回已完成或已过期的旧 Checkout URL。确定已完成或已过期后，客户端才可轮换 Idempotency Key；结果未知的 provider failure 必须保留原 key。创建、继续和放弃 Checkout 按用户串行化，不在 Stripe 网络调用期间持有数据库 transaction。每个用户最多保留 3 笔不同数量的未支付 active Checkout；`paid_pending_credit` 已是支付事实，不占用待支付名额。
- 仅签名 Webhook 或服务端主动查询取得的、已验证身份绑定的 Stripe paid Checkout Session 可触发到账；浏览器 URL、public token、未支付或失败 Session 均不得到账。Stripe paid fact 先独立 durable 为 `paid_pending_credit`；随后 `purchase_credit` 与 `paid_pending_credit → credited` 在同一 PostgreSQL transaction 中原子提交，并对 replay/concurrency 幂等。
- 用户充值历史只返回当前用户最近 12 个自然月的 final `credited` 最小事实，以最大 20 条 opaque cursor 分页；不返回 payer PII、provider secret 或内部 Stripe metadata。cursor 必须在解码前限制长度。
- Dispute Webhook 只触发 canonical Stripe Retrieve。open/lost Dispute 在 Payment Order → Wallet 固定锁序的同一 transaction 中创建原因化 Restriction、执行 bounded `payment_dispute_debit` 并持久化未追回额度；`won` 与 `lost → won` 只恢复本 Dispute 的实际 debit 并精确解除对应 Restriction。
- User SSE 按 user ID 隔离，连接生命周期必须 heartbeat 并 cleanup；它只通知 `processing`/`credited`/`failed`/`refund_updated`/`dispute_updated`，不替代 Stripe 或 PostgreSQL 的支付事实。
- 每 5 分钟的 River 对账只 claim 超过 2 分钟未收敛的订单，每批最多 100 条。claim 的 lease 比 worker timeout 更长，token + generation 可阻止 stale worker 覆盖新结果；多 worker 的正确性不依赖队列并发数。
- `creating` Order 持久化经过校验的 immutable return base URL，以及首次 Stripe 请求使用的整秒 expiry。重启或创建 timeout 后必须用原 public order number 和完全相同的参数重放 Stripe Idempotency Key，而不是依据本地 timeout 直接宣告失败。
- `paid_pending_credit` 只重试 Credits 原子到账，不重复查询或重写 Stripe paid fact。Stripe transient failure 交给 River retry；确定性配置、权限、响应或身份绑定错误记录脱敏 operational failure 并延迟重试，避免 hot loop。
- 超过 24 小时仍未收敛的 Order 只增加 durable anomaly 标记，不改变支付业务状态；后续 canonical terminal Stripe fact 会清除该异常。
- Admin order read model 严格校验 canonical public order number、bounded user/status filters 和 opaque cursor；不得返回 public token、idempotency/request hash、return base URL、Secret 或原始 provider payload。
- Payments 不改变任务基础设施 ownership：River + PostgreSQL 仍是 Task queue/state fact source，NATS JetStream 仍只承担 event layer。
