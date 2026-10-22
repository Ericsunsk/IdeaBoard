---
status: accepted
---

# ADR-0038：Personal 与 Organization 资金池及 Workspace 归属

产品规则已接受并已在生产行为中生效：账户创建/归属/换户/转账面已随 #950 收敛（独立 Workspace 账户创建、link 读写、换 payer、跨账户余额移动均已删除；解析只认创建时 scope）；组织基数、角色与 Workspace 访问映射及管理面切换已随 #951 落地；Personal 协作（Default 拒邀、无审批、缺席 unlimited）已随 #952 落地；额度矩阵（默认门退役、Owner 短路、403 矩阵）已随 #953 落地；Workspace Budget 已随 #954 全拆；组织关闭/恢复/余额处置已随 #955 落地。本 ADR 只记最终结构。

## 背景

当前已接受的计费设计允许同一用户管理多个独立 Billing Account、创建 Workspace 时选择共享或独立账户，并提供 Workspace Budget、账户级 Member Quota 和必须预先配置的默认成员额度。产品讨论决定收敛成 Personal 与可选 Organization 两个资金范围：个人业务空间共享 Personal 资金池，组织业务空间共享 Organization 资金池；Workspace 是内容与授权边界，不再是资金边界。

此决策改变既有产品合同，但保留资金事实的稳定性：Wallet/Ledger、支付、退款、Task/Vision submit-time payer snapshot、事务预占与结算等账务正确性机制继续有效。实现迁移必须单独处理既有账户和限制数据，不得静默合并或改写不可变资金记录。

## 决策

### 1. 资金范围与 Workspace 归属

- 每个用户恰有一个 Personal Billing Account；用户最多加入或创建一个 Organization，每个 Organization 恰有一个 Organization Billing Account。
- Personal 与 Organization 账户相互独立；加入 Organization 不合并、不迁移 Personal 余额。
- 每个 Workspace 创建时永久归属 Personal 或 Organization 之一，消费从该账户扣除。Workspace 不持有独立钱包或余额；不支持 Workspace 间真实余额转账、账户间资金拨款或创建后切换资金归属。
- 新建 Workspace 时可选择 Personal 或当前 Organization；用户没有 Organization 时隐藏 Organization 选项，并提供创建 Organization 的入口。创建 Organization 不改变既有 Workspace 的 Personal 归属。
- Organization 下所有 Workspace 共用该 Organization 账户余额；同一用户所有 Personal Workspace 共用其 Personal 账户余额。
- Personal `Default` 是不可邀请成员的个人空间。Organization `Default` 是组织管理入口，不是内容 Workspace，不承载画布或消费。
- 删除 Workspace 不改变其资金账户余额；历史账务与结算快照保留。

### 2. Organization 角色与内容访问

| 角色 | 组织管理 | Workspace 访问 | 成员额度 |
| --- | --- | --- | --- |
| Owner | 完整组织与资金管理；可充值；Owner 变更必须显式交接 | 自动访问并可编辑所有 Organization Workspace | 永远不限额；可给 Admin 设置额度 |
| Admin | 可邀请/移除成员并授权 Member 访问 Workspace；不可充值、不可变更 Admin 角色 | 自动访问并可编辑所有 Organization Workspace | 可给 Member 设置额度；自身额度仅 Owner 可设置 |
| Member | 无组织管理权限 | 必须逐 Workspace 获得访问权；获准后可编辑 | 按组织级额度限制消费 |

- Owner/Admin 可邀请或移除 Organization 成员，并管理 Member 的 Workspace 授权。只有 Owner 可提升或降级 Admin；Owner 只能通过显式 Owner 交接变更。
- 只有 Owner/Admin 可以创建 Organization Workspace；创建时使用 Organization 共享资金池。
- Admin 降级为 Member 后，立即失去全组织自动访问，只保留其已明确获得的 Workspace 权限。
- 组织成员资格本身不授予 Member 访问所有 Workspace 的权限。Organization Workspace 不允许组织外协作者；用户必须先加入该 Organization。
- 成员离开 Organization 后立即失去 Organization 及其全部 Workspace 的访问。该成员创建的 Workspace 和内容仍属于 Organization。

### 3. 消费额度

- 保留额度周期 `unlimited`、`daily`、`weekly`、`monthly`、`fixed`。额度是消费限制，不是子钱包、余额或资金预拨。
- Organization Member 的额度以 `(Organization Billing Account, User)` 为范围，在该 Organization 所有 Workspace 间汇总；新 Member 默认 `unlimited`，邀请不要求预先配置默认额度。
- Owner 永远 `unlimited`。Owner 可设置 Admin 的额度；Owner/Admin 可设置 Member 的额度；Admin 不能设置 Owner 或 Admin 的额度。
- Personal Default 不可邀请。Personal 下其他 Workspace 可邀请协作者，其消费扣 Personal Billing Account。额度以 `(Personal Billing Account, User)` 为范围跨 Workspace 汇总，由 Personal 账户持有人管理；未配置时默认 `unlimited`。
- Personal Workspace Owner/Admin 可直接邀请和移除协作者，无需 Personal 账户持有人逐次审批。新协作者默认 `unlimited`，相关消费由 Personal 账户承担；账户持有人之后可设置额度。
- 达到消费额度时，阻止该用户发起新的消费；额度限制与账户可用余额共同生效。
- 移除成员或协作者不得删除历史账务事实；重入同一资金范围时沿用该成员既有额度与消费历史，除非有明确管理操作修改。

### 4. Organization 关闭、恢复与余额

- Organization Owner 离开前必须将 Owner 身份显式交接给另一位组织成员；组织余额永远不转入任何人的 Personal 账户。
- Owner 关闭 Organization 后，立即暂停全部成员访问与新消费，结清在途任务、退款和争议；保留成员关系、Workspace、权限及账务历史。
- 未消费余额按充值条款处理：允许退款时退回原支付渠道；不支持退款或按条款保留时，余额留在已关闭的 Organization 账户，不转为个人资金。具体退款资格及时限由充值条款及适用法规决定。
- Owner 可恢复 Organization；恢复后恢复关闭前的成员关系和权限。关闭本身不删除资金账户或历史账务。

## 后果

- 不再向用户提供 Workspace 独立 Billing Account 选择、Workspace Budget 或 Workspace 余额转账。Organization 资金直接共享；Personal 与 Organization 是仅有的用户资金范围。
- Workspace 内容访问仍需遵循 Organization 角色映射和 Member 的逐 Workspace 授权；资金范围不能替代内容授权。
- 消费额度的实际使用在 Task/Vision 的事务性 Quote/Hold 边界执行；钱包余额检查、预占、结算、退款关联、并发保护和失败释放继续保留。独立 Workspace 账户创建、Workspace Link 读写、换 payer、跨账户余额移动、Workspace Budget、账户默认成员额度门均已删除，无残留、无双实现。
- 既有多账户余额按 #949 处置规则收敛（空库收敛，不合并、不重写 Ledger、不改写历史 payer 快照）；Organization 关闭状态对读写授权的门禁、在途结算与余额处置按本 ADR §4 执行。

## 取代关系

- 本 ADR 取代 [ADR-0033](./0033-workspace-creation-billing-account.md) 中 Workspace 创建时选择独立 Billing Account、同一用户多个独立 Personal Billing Account，以及按 Workspace 新建/共享资金账户的产品规则。
- 本 ADR 取代 [ADR-0034](./0034-workspace-shared-consumption-budget.md) 的 Workspace 级消费预算决策。账务预占和账户余额保护继续保留。
- 本 ADR 取代 [ADR-0035](./0035-member-cross-workspace-consumption-quota.md) 中额度按任意 Billing Account 聚合的范围表述；保留五种周期、submit-time 归属、并发防护、退款修正和失败收敛机制，目标额度主体限定为 Personal 或 Organization。
- 本 ADR 取代 [ADR-0037](./0037-account-default-member-quota.md) 的未配置默认额度时邀请 fail-closed 规则；新成员默认不限额，邀请不依赖默认额度配置。
- [ADR-0036](./0036-studio-handover-and-delete-funds-retention.md) 的不可变账务历史、显式 Owner 交接和删除期间保护继续适用；Organization 关闭/恢复与资金规则由本 ADR 取代或补充。

以上是决策关系；旧实现已按上述票号删除，旧 ADR 描述的实现合同仅作历史理解，生产行为以本 ADR 为准。
