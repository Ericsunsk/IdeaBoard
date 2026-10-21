# ADR-0028: Welcome Grant and RewardCode Beneficiary Account

## Context

Issue #921 要求欢迎奖励与兑换权益进入明确的受益账户：操作资格与入账
账户分开，切换 Workspace 不会重复领取或误入组织账户。#915 已落地稳定
的付款账户解析与财务权限，#916 已把 Wallet 归属收敛到 Billing Account
（资金 identity 归账户、操作人归 Actor；无 Workspace 域的支出解析为
付款人/操作人的确定性初始个人账户）。但奖励链路仍有缺口：Redemption
的传输幂等 hash 绑定了 workspace，跨空间同 key 同码会冲突而非 replay；
Redeem 响应只带 beneficiary 余额不带账户身份，UI 无法明示实际入账账户；
Welcome Grant 的"个人身份资格→初始个人管理账户→同一 Commands 链路"
缺少显式测试钉。产品模型见桌面共识《IdeaBoard-团队账单池设计共识》
（Billing Account 持有统一余额，多 Workspace 共享，Workspace 管内容/
消费位置，User 记实际操作人）与
[ADR-0024](./0024-billing-account-resolution.md)、
[ADR-0025](./0025-billing-account-wallet-ownership.md)。

## Decision

1. Welcome Grant 资格按已验证个人身份（canonical registration identity
   HMAC）判定，进入其明确的初始个人管理账户（`ResolvePersonalAccount`
   确定性解析），经同一 Billing Account/Wallet/Commands 链路
   （`ClaimWelcomeGrant` + `ExecuteGrant` 同一事务）发放；重放不产生新
   Grant，HMAC key 漂移 fail closed。
2. RewardCode 领取资格保留既有按 User 规则（每人每码至多一次，
   `max_redemptions = 0` 为不限），受益人恒为领取人的初始个人账户：
   Redemption + Ledger Grant 同一事务提交（advisory 锁→容量预留→
   `InTx(tx).Grant`→Redemption insert），保留幂等、HMAC key 生命周期、
   停用、过期与容量限制；当前选中组织 Workspace 只做 authZ scope，
   不改变受益人，组织资金不被个人领取隐式改动。
3. 传输幂等 hash 只覆盖操作 + code（去工作区化）：同 key 同码跨
   Workspace replay 首个 Redemption，同 key 异码仍 409；durable 层
   `(user_id, reward_code_id)` 唯一事实继续兜底防重领。
4. Redeem 响应携带受益人 `billing_account_id`（ba_）与其可用余额；未知
   账户映射 fail closed（503），绝不把 Workspace 账户冒充为受益人。
   UI 明示"奖励入账至个人账户，当前工作区不改变受益人"。
5. 账户管理权转移（owner 交接/成员变更）本身不产生新的欢迎奖励资格：
   Welcome 指纹绑定身份而非账户角色，重放仍收敛。
6. 首期不做：Workspace 钱包、内部拨款、跨账户余额转移、auto topup、
   订阅、有效期、更换付款账户（沿用 #915 conflict 拒绝逻辑）。

## Consequences

- Rewards 经 `credits.Commands` 的唯一写入入口不变；幂等/HMAC/停用/
  过期/容量语义不变，只是 hash 去工作区化与响应增受益人字段。
- 后续票（945/946 额度执行、最终集成发布 gate）在此受益人契约上构建；
  中间步骤不单独发布。

## Status

Accepted
