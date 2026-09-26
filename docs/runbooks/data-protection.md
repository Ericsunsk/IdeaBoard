# IdeaBoard 数据保护运维手册

本文档聚焦 PostgreSQL backup、restore 与密钥材料处理。账号安全操作边界见 [auth-operations.md](./auth-operations.md)，部署步骤见 [deployment.md](./deployment.md)。

## PostgreSQL Backup 安全基线

`ideaboard update --manifest ...` 生成的 `postgres-*.dump` 会包含完整业务库数据，其中包括：

- `api_account_secrets`：上游模型/API 渠道的 API key、token 或外部 secret 引用。
- `storage_profile_secrets`：S3-compatible 存储的 access key、secret key 等对象存储凭据。
- `payment_stripe_configurations`：Stripe Restricted API Key 与 Webhook Secret 的密文，以及当前 Stripe resource references。
- `event_subscriptions`： outbound webhook 的 endpoint signing secret（`encrypted` 行为 AES-256-GCM 密文，`plaintext` 行为 cutover 前遗留的 `whsec_` 明文）与 `signing_secret_key_version` 主密钥版本指纹。
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
3. Outbound webhook signing secret，对应 `event_subscriptions`（见下节）。
4. 生产环境 Better Auth secret 与 auth internal key，只在怀疑 backup 泄露或传输边界失控时轮换。

轮换后执行 smoke 检查，确认登录、任务提交、结果回写、上传与 `/uploads/*` 访问正常。

## Outbound Webhook Signing Secret 轮换

`event_subscriptions.signing_secret` 以 `APP_SECRETS_ENCRYPTION_KEY` 信封加密落盘
（AES-256-GCM，密文覆盖 `whsec_` transport 字符串；`signing_secret_key_version`
为当前主密钥的 SHA-256 指纹，不含密钥材料）。读链路只返回 masked view
（`has_secret`），明文仅在创建 server-minted secret 时返回一次。

### 订阅级轮换（subscriber 泄露 / 常规轮换）

删除并重建订阅即可：新订阅 mint 32 字节 secret，加密落盘，`whsec_` 明文仅返回一次。
通知 subscriber 更新验签密钥。旧订阅删除后，已有 River `webhook_deliver` job
按未知订阅直接丢弃（不重试、不泄露）。

### 主密钥轮换（`APP_SECRETS_ENCRYPTION_KEY`）

1. 保留旧 key，用同时持有新旧 key 的离线脚本逐行解密再加密：
   `UPDATE event_subscriptions SET signing_secret = <new_ct>, signing_secret_material_kind = 'encrypted', signing_secret_key_version = <new_fp> WHERE public_id = ...`。
2. 切换环境变量并重新部署；启动时 `LoadEncryptionKey` fail closed，无 key 拒绝启动。
3. `key_version` 与当前 key 不一致的行读取直接失败（fail closed，错误不携带任何 secret 材料），
   以此发现遗漏行并补做步骤 1。
4. 切 key 窗口期的新写入仍用旧 key 落盘，切换后需要对该窗口行再执行一次步骤 1；
   要求零窗口时先短暂冻结订阅创建。

### 已知 Residual

- 进程内只有单 key 解密，没有新旧 key 双活 verify：轮换必须走上面的离线重加密流程，
  不能只换环境变量（否则旧行 fail closed，webhook 投递中断直到补加密）。
- `plaintext` 遗留行在首次被读到时由 re-encrypt-on-read 自动收敛为密文；
  该路径是 best-effort（失败只记 `subscription` + `key_version`，不记任何 secret 材料，
  不影响本次读取）。长期无人访问的遗留行仍以 `whsec_` 明文存在，
  可用 `SELECT public_id FROM event_subscriptions WHERE signing_secret_material_kind = 'plaintext'`
  审计并手动触发一次读取完成收敛。

## Payments 数据边界与保留

Payments 只保留支付收敛、退款、Dispute 和审计所需的最小事实：Stripe Account/Product/Customer/Checkout Session/Payment Intent/Charge/Refund/Dispute/Event IDs、金额、币种、状态、时间、订单接受的 Terms/Refund Policy version、Receipt URL，以及受限长度的错误摘要。Payment Order、Refund、Dispute、Webhook Inbox、Payment admin audit 和对应 Credits Ledger 是对账与法务审计事实；不得通过手工删除、匿名化或 truncate 破坏其关联关系。

以下数据禁止写入 PostgreSQL、日志、Admin read model、Webhook Inbox、River payload、SSE、Issue、截图或未加密 artifact：

- Stripe API Key、Webhook Secret 或任何可恢复的 secret 明文。
- 完整 Stripe webhook/provider payload、完整 request/response body、Authorization header 或内部 stack trace。
- 卡号、CVC、银行卡 token、完整账单地址、身份证明或其他付款人 PII。

Stripe API Key 与 Webhook Secret 只能以平台 Secret material 密文保存；HTTP/Admin API 只显示“是否已配置”和截断后的 provider identity。Webhook Inbox 不保存 raw payload。错误摘要必须使用用户安全、受限长度的分类信息；排障需要的 provider 原始资料只在 Stripe Dashboard 的受控访问边界查看。

当前 Payments 事实没有自动删除任务。任何新增 retention 或清理策略必须先确定适用会计、税务、争议和隐私义务，完成数据恢复演练，并保证不会删除未结算 Payment/Refund/Dispute、关联 Ledger 或用户注销保护所需的状态。到期清理由数据库 migration 和可审计运维流程实现，不能靠临时 SQL 或 Dashboard 导出替代。
