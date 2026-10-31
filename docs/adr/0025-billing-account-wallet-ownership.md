# ADR-0025: Billing Account Wallet Ownership and Funds Identity
> Superseded（Resolution 部分）：被 [ADR-0038](./0038-personal-organization-billing-model.md) 取代，当前产品合同以 0038 为准。

## Context

Issue #916 要求将 Credits Wallet 与 Ledger 统一归属 Billing Account。
#915 已落地稳定的付款账户解析（显式 `workspace_billing_links` →
组织池 → per-Workspace 账户，可加入调用方事务，fail closed）与财务
Owner/Admin 权限，但资金仍是 user 钱包：`credit_wallets` 主键即
`user_id`，Hold/Capture/Release 以 submitter 为 Wallet 边界，V1 读经
owner 钱包投影。继续以个人钱包结算共享空间消费，会让资金归属随
Workspace Owner 漂移，且组织成员停用会误伤组织资金。产品模型见桌面共识
《IdeaBoard-团队账单池设计共识》（Billing Account 持有统一余额，多
Workspace 共享，Workspace 管消费位置，User 记录实际操作人）与
[ADR-0024](./0024-billing-account-resolution.md)。

## Decision

1. Wallet 唯一归属 Billing Account：`credit_wallets` 主键为
   `billing_account_id`；全部 Credit Commands、Restrictions、Wallet
   lifecycle 与 Reconciliation 使用统一账户契约。`Metadata.Actor` 记录
  真实调用身份（user/service/admin/system），资金 identity 与操作人永不
   混淆；不保留 user-wallet 回退、别名路由与双结算。
2. 调用方用 #915 解析：Workspace 域支出在调用方事务内解析稳定账户
   （Task 提交的 Quote/Hold/Task/River 同一事务；歧义或冲突 fail
   closed）；无 Workspace 域的支出（Rewards、Payments、Vision、Welcome、
   Admin）解析为付款人/操作人的确定性初始个人账户（其 Default 个人空间
   账户，按需 provision）。
3. 存量确定性归属：既有个人余额、历史交易、未完 Hold 按
   `ensure_personal_billing_account_for_user` 归入每人初始个人管理账户；
   只回填新增归属列，不复制、不合并、不重写 immutable Ledger 事实
   （金额/postings 不动），不把个人资金注入组织（组织/共享账户从 0 开始）。
   迁移仅加法 + 约束收敛（门禁合规：无 `SET NOT NULL`/`ADD COLUMN NOT NULL`；
   PK 切换与被取代函数下线带 `destructive: approved` 走评审门禁）。
4. 个人冻结/注销只处理明确归属：ban/注销仅冻结其sole-owned 非组织账户
   （`freeze_sole_owned_billing_wallets`），注销并从仍有其他管理人的账户
   移除成员关系（`remove_shared_billing_memberships_for_user`）；组织与
   他人资金不受影响，已有支付义务与 Hold 经 Capture/Release 照常收敛。
5. Scope 删除不删钱：Workspace hard delete 移除 scope（boards、tasks、
   links、members、`billing_accounts` 行），Wallet、projection、Ledger、
   snapshot、operation 行在 `billing_account_id` 归属下保留（与
   `task_billing_snapshots.task_id` 无 task FK 的先例一致）；命令函数逐
   次校验钱包存在，不遍历悬空引用。
6. 首期不做：Workspace 钱包、内部拨款、跨账户余额转移、auto topup、自动
   扣其他账户、订阅、积分有效期、Workspace 更换付款账户（沿用 #915
   `billing_account_conflict` 拒绝逻辑）。

## Consequences

- 资金写入唯一入口仍是 `credits.Commands`（v3/v2 账户例程），Billing 只
  定价不写账，Payments 只拥有订单生命周期不写账，均经 `InTx` 落账；
  runtime role 仅 SELECT 资金表 + EXECUTE 指定例程。
- Task 结算在同一事务完成解析/Hold/Task/River；Vision/Payments/Rewards
  在各自事务内解析个人账户；Admin（control）以 usr_ 为目标解析其个人
  账户（wire 不变，response 增 `billing_account_id` 可选字段）。
- 后续票（945/946 额度执行、消费/支付/运维入口迁入、最终集成发布 gate）
  在此契约上构建；中间步骤不单独发布。

## Status

Accepted
