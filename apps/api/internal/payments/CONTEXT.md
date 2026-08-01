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

## Public Seams

- `Service.GetStripeConfiguration` 只返回可安全展示的配置摘要，不返回 API Key、Webhook Secret 或其密文。
- `Service.ConfigureStripe` 先通过 `StripeGateway` 准备并验证 Stripe account、Product 与 Webhook endpoint，再原子发布当前配置和成功 audit；准备失败时保留旧配置。
- `Service.ReceiveStripeWebhook` 对 raw body 验签、去重并写入最小 Webhook Inbox。后续 payment order、refund 与 dispute lifecycle 继续收口在 Payments context。
- 任何支付结果引起的 Idea 余额变化都必须调用 Credits 的 typed Commands；Payments 不直接写 Ledger 或 balance projection。

## Invariants

- Stripe API Key 与 Webhook Secret 使用平台 Secret material 加密存储，禁止进入 HTTP response、日志、Webhook Inbox 或 Admin audit。
- Webhook 必须在业务解析和持久化前使用 raw body、当前 Webhook Secret 与 5 分钟 tolerance 验证签名；无效或过期签名不写 Inbox。
- Stripe Event ID 在全局唯一。重复投递保持幂等；已签名但未知的事件只记录最小 metadata 并标记 `ignored`。
- Webhook Inbox 不保存原始 payload，只保存 Event ID、type、object ID、livemode、Stripe created time、接收/处理状态与时间。
- 配置与 Webhook Secret 在 request-time 从 PostgreSQL 读取并实时生效，不使用 application-lifetime 配置缓存。
- Payments 不改变任务基础设施 ownership：River + PostgreSQL 仍是 Task queue/state fact source，NATS JetStream 仍只承担 event layer。
