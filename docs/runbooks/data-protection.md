# IdeaBoard 数据保护运维手册

本文档聚焦 PostgreSQL backup、restore 与密钥材料处理。账号安全操作边界见 [auth-operations.md](./auth-operations.md)，部署步骤见 [deployment.md](./deployment.md)。

## PostgreSQL Backup 安全基线

`ideaboard update` 生成的 `postgres-*.dump` 会包含完整业务库数据，其中包括：

- `api_account_secrets`：上游模型/API 渠道的 API key、token 或外部 secret 引用。
- `storage_profile_secrets`：S3-compatible 存储的 access key、secret key 等对象存储凭据。
- `payment_stripe_configurations`：Stripe Restricted API Key 与 Webhook Secret 的密文，以及当前 Stripe resource references。
- Better Auth 的 `user`、`session`、`account`、`verification` 等身份与会话相关表。
- 业务用户、任务、画板、媒体索引、Payment Order/Refund/Dispute/Webhook Inbox 与系统配置。

因此 PostgreSQL backup 必须按 secret artifact 处理，而不是普通日志或普通导出文件。

## 处理要求

- 建议对 backup 加密落盘，并把密钥交给既有 secret 管理流程。
- 限制下载、传输、复制与共享范围，只允许最小必要运维人员访问。
- 不要把 `postgres-*.dump` 上传到工单、聊天工具、未加密对象存储或个人网盘。
- 排障完成后清理临时副本、本地解压文件与一次性传输位置。
- 记录 restore 操作人、时间、来源 backup、目标环境与后续 rotation 结果。

## Restore 后 Rotation

完成 restore 后，按受影响范围轮换：

1. 上游模型/API 渠道密钥，对应 `api_account_secrets`。
2. S3-compatible 对象存储 access key/secret，对应 `storage_profile_secrets`。
3. 生产环境 Better Auth secret 与 auth internal key，只在怀疑 backup 泄露或传输边界失控时轮换。

轮换后执行 smoke 检查，确认登录、任务提交、结果回写、上传与 `/uploads/*` 访问正常。

## Payments 数据边界与保留

Payments 只保留支付收敛、退款、Dispute 和审计所需的最小事实：Stripe Account/Product/Customer/Checkout Session/Payment Intent/Charge/Refund/Dispute/Event IDs、金额、币种、状态、时间、订单接受的 Terms/Refund Policy version、Receipt URL，以及受限长度的错误摘要。Payment Order、Refund、Dispute、Webhook Inbox、Payment admin audit 和对应 Credits Ledger 是对账与法务审计事实；不得通过手工删除、匿名化或 truncate 破坏其关联关系。

以下数据禁止写入 PostgreSQL、日志、Admin read model、Webhook Inbox、River payload、SSE、Issue、截图或未加密 artifact：

- Stripe API Key、Webhook Secret 或任何可恢复的 secret 明文。
- 完整 Stripe webhook/provider payload、完整 request/response body、Authorization header 或内部 stack trace。
- 卡号、CVC、银行卡 token、完整账单地址、身份证明或其他付款人 PII。

Stripe API Key 与 Webhook Secret 只能以平台 Secret material 密文保存；HTTP/Admin API 只显示“是否已配置”和截断后的 provider identity。Webhook Inbox 不保存 raw payload。错误摘要必须使用用户安全、受限长度的分类信息；排障需要的 provider 原始资料只在 Stripe Dashboard 的受控访问边界查看。

当前 Payments 事实没有自动删除任务。任何新增 retention 或清理策略必须先确定适用会计、税务、争议和隐私义务，完成数据恢复演练，并保证不会删除未结算 Payment/Refund/Dispute、关联 Ledger 或用户注销保护所需的状态。到期清理由数据库 migration 和可审计运维流程实现，不能靠临时 SQL 或 Dashboard 导出替代。
