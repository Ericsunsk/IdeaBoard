---
status: accepted
---

# ADR-0042：Personal payer = actor（各付各的）

[#973](https://github.com/Ericsunsk/IdeaBoard/issues/973) 已接受 Personal（无组织非 Default）工作区的资金语义：谁提交谁付款（payer = actor），Personal 侧无任何限额。T1（#974）在工作区先行钉死 Task 结算的 actor 边界。本 ADR 把该决策固化为 invariant，防止后续回退到 holder/共享语义；Organization 路径与共享结算核心原封不动。

## Context

此前 Personal 工作区执行 holder-pays：协作者的消费全部记到空间持有人的 Personal Billing Account，并受持有人管理的按人限额（MemberQuota）约束。这带来三个问题：持有人为他人消费买单与其余额预期不符；Personal 侧要维护一整套按人限额管理（聚合、角色矩阵、holder 管理语义）；资金归因与真实操作人分离，心智负担高。

项目无存量用户与资金数据，绿地直改，不做任何迁移与兼容路径。

## Decision

### 1. payer 恒等于 actor

- Personal 范围（= 无组织非 Default 工作区）提交时，Quote / Hold / Task 行 Billing Account 均取提交人自己的确定性 Personal Billing Account；Ledger Hold 的 Actor 为提交人。不存在代付；解析失败 fail-closed，不回退到空间持有人或任何其他账户。
- 行内 BillingAccountID 照常钉死（恒等于 actor 的账户）；成功终态 Capture、失败、取消或最终超时 Release 读行收敛机制不变（与 Task 状态翻转同一事务，重试用新 execution token，不做二次 Hold；免费任务走现有 free 路径）。
- Vision Search 同样各付各的：reserve / settlement 记 actor 的 Personal 账户，Actor 为提交人；余额不足同样 fail-closed。
- Quote / readiness 预检执行同一账户与同一消费资格判定（预检读 actor 自己的账户余额与资格，无资金副作用）；Board 编辑权限门控不变（Viewer 永不生成，纯财务管理不授予 Board 权限）。
- 余额不足 402 / 资金限制 403（与现行语义一致），不产生 Task 行、不产生 Hold、不入队；充值后可重提。成员被移除后，在途与终态结算仍按提交时行内事实（actor 账户 + actor）收敛，不改写、不迁移。
- 账单与用量视图按 actor 归属呈现：同一空间出现多付款账户时，按（Billing Account + User）分列，不得把成员消费合并记到空间所有者名下；Workspace 本身仍不持有余额。

### 2. Personal 侧无限额

- MemberQuotas 收缩为只保 Organization；Personal 范围跳过结算事务内的限额检查（不是"默认不限额"，而是无此检查），无按人限额、无 holder 管理、无额度耗尽（`member_quota_exhausted` 在 Personal 范围内永不出现）。
- 调 Personal 限额无此能力，必须 fail-closed 得清晰（不可用，而非静默成功）。
- 术语（与 #973 Further Notes 一致）：用户侧继续称 Idea，后端与本 ADR 称 Credit；资金身份只称 Billing Account（`ba_`），容器只称 Credit Wallet，操作人只称 Actor；Personal = 无组织非 Default 工作区，Organization = 纯管理壳（只做成员与管理边界，从不消费）。

### 3. 否决的方案

- 维持 holder-pays + Personal 按人限额：否决。归因错误（持有人为他人买单）、Personal 侧限额管理复杂（聚合、角色矩阵、holder 语义）、资金归因与操作人分离。
- Personal 空间级独立账户 / 子钱包 / 代付 / 转账：否决。Personal 侧是归属规则变化，不是账务机制变化；复用既有 Billing Account、Wallet、Ledger、Hold / Capture / Release、幂等与事务机制，不引入新表与新资金原语。
- 双实现 / 兼容路径 / 存量迁移：否决。绿地直改，Fresh Database 验收。

### 4. 与 Organization 的关系

- Organization 路径原封不动：Owner / Admin / Member 提交仍共享 Organization 资金池扣款；Member 限额仍按（Organization Billing Account，User）跨空间汇总；Owner 恒不限额；Owner 设 Admin、Owner / Admin 设 Member、Admin 不可改 Owner / Admin 的角色矩阵不变；永久关闭语义不变。
- Personal Default 仍为不可邀请的私人空间；非 Default Personal 空间的拉人链、人数上限、Welcome Grant / RewardCode 入账账户（各归各）不变。

## Consequences

- Personal 空间消费视图首次出现"同一空间、多付款账户"；聚合口径与权限过滤需与后端只读口径对齐（SQL 授权过滤先行、分页在后），避免把他人消费金额暴露给无权查看者。
- 后续 T2（MemberQuotas 收缩）、T3（holder 语言清理）、T4（Vision）、T5（readiness/终态）、T6（账单视图）、T8（全链验收）在本决策下并行；行为变更以对应 issue 为准，本 ADR 只定决策。
- 验收以 Fresh Database 为前提；上线后产生的不可变资金记录不得改写。

## 取代关系

- 本 ADR 以 supersede 方式取代 [ADR-0038](./0038-personal-organization-billing-model.md) 的 Personal 章节：§1 中 Personal 共享池表述（同一用户所有 Personal Workspace 共用其 Personal 账户余额）与 §3 中 Personal 按 holder 汇总与管理表述。ADR-0038 §2（组织角色与内容访问）与 §4（Organization 永久关闭与余额）继续有效。
- 本 ADR 明确：Personal payer=actor 语义下，`member_quota_exhausted` 在 Personal 范围内永不出现；Personal 限额管理语义无承载对象。

## Status

Accepted
