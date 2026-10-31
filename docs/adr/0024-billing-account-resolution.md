# ADR-0024: Billing Account Stable Resolution and Financial Permissions

## Context

Issue #915 要求统一 Billing Account 归属解析与财务权限：各入口从 Workspace
解析稳定的付款账户，区分财务管理员、空间管理员与实际消费成员，有无
Organization 都适用。产品模型见桌面共识《IdeaBoard-团队账单池设计共识》
（Billing Account 持有统一余额，多 Workspace 共享，Workspace 记录消费位置，
User 记录实际操作人；Organization 可选；Workspace 预算与成员额度是消费限制，
不是独立余额）。Task 已持久化 billing_account_id、workspace_id、
submitter_user_id，但解析仍是每 Workspace 1:1 的 owner stub，且充值/账单读取
以 Workspace Owner 个人钱包为代理，不满足归属稳定与权限分离。Ledger 与
Task/River 原子事务契约必须保留；额度计算（945/946）、Workspace 钱包、内部
拨款、跨账户转移、订阅/有效期、更换付款账户不在本票范围。

## Decision

1. Billing Account 是稳定的资金身份（ba_），持有统一余额；多 Workspace 可
   共享一账户，Workspace 只记录消费位置，User 只记录实际操作人。身份稳定，
   不以当前 Workspace Owner 个人钱包为付款目标。
2. 解析顺序固定且可加入调用方 transaction：显式 workspace_billing_links，
   否则 Organization 账户（组织下多 Workspace 共享），否则 per-Workspace
   账户。缺失、歧义或归属冲突 fail closed，不静默换钱包；加入组织或空间不
   合并个人资金。
3. 无 Organization 的多业务空间可显式关联同一账户（link，需 Workspace
   Owner/Admin + 目标 Billing Owner/Admin）；组织 Workspace 只能关联本组织
   账户，个人空间不能关联组织账户；已有关联的 payer 切换拒绝
   （billing_account_conflict，更换付款账户不在本批范围）。
4. 财务管理权与资金身份分离：Billing Owner/Admin 均可管理充值、账单、成员
   额度与 Workspace 预算；空间管理员仅有空间权限时不能提额（充值要求
   Billing Owner/Admin，渲染 billing_account_forbidden）；财务管理权不授予
   Board 内容访问；全账户 Ledger、账单与用量读取同样要求 Billing
   Owner/Admin，空间身份本身不授予全账户财务明细；正式成员发起 Canvas 付费生成仍需 Board 编辑权限，不新增
   canSpend 开关。
5. 同一 User 可管理多独立账户（余额不合并）；非组织账户可设获授权 Admin；
   所有权可交接（前 Owner 降为 Admin，身份/余额/历史稳定），不把终身绑定
   登录身份写成不变量；最后 Owner 不可删除或降级。
6. 首期不做 Workspace 钱包、内部拨款、跨账户转移、auto topup、订阅/有效期；
   预算/额度执行（945/946）只做权限区分到位，不实现额度计算。

## Consequences

- Task 结算继续在同一 PostgreSQL transaction 完成 Quote/Hold/Task/River，
  解析加入该事务；历史资金事实不删除。
- 现有 per-Workspace 账户行保留并作为默认解析；共享通过显式 link 与组织
  账户实现，不做静默迁移。
- 后续票（945/946 额度执行、最终集成发布 gate）在此解析与权限之上构建；
  中间步骤不单独发布。

## Status

Accepted

Superseded by [ADR-0038](./0038-personal-organization-billing-model.md)：显式 `workspace_billing_links` 解析顺序、per-Workspace 独立账户与 payer 切换规则作废；Workspace 只记录消费位置，不再是资金边界，资金范围仅为 Personal 与 Organization。当前产品合同以 ADR-0038 为准，本 ADR 仅作历史理解。
