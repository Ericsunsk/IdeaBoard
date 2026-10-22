# 团队账单池竞品调研

初次调研日期：2026-10-20；关闭与退款规则补充核实：2026-10-23。本文记录公开产品行为及其来源，供 IdeaBoard 的账单归属讨论使用；不是已接受的架构决策，也不据此推断竞品数据库模型。

## 调研结论

个人付费与可选团队付费可以并存。OpenRouter、TapNow 的官方文档均明确区分个人资源与团队资源；共享资金池不要求每位用户先加入 Organization。

需要区分三个维度：实际操作人、资金所属账户、工作内容归属。竞品通常分别提供共享余额、成员消费管控和团队内容管理；它们在界面上使用 Organization、Team 或 Space 的名称，不能据此认定这些维度必须由同一个内部实体承担。

| 产品 | 个人与团队 | 资金池与管控 | 证据范围 |
| --- | --- | --- | --- |
| OpenRouter | 个人账户与 Organization 独立，可切换、多组织加入 | Organization 共享 credits；管理员购买及管理付款信息；组织 API Key 用量从组织余额扣除 | 官方帮助文档 |
| TapNow | 新团队拥有独立计费账户；个人与其他团队的订阅、权益、Tapies 不自动转入 | 共享团队余额；成员可设不限制、日/周/月周期额度或固定额度 | 官方帮助文档 |
| Lovart | 个人账户与团队版分开，个人账户不供多人共用 | 官方团队版弹层展示共享积分及各席位算力上限 | 官方 pricing 页面及团队版弹层 |
| LibTV | 官网有个人会员、积分超市及企业采购入口 | 二手报道描述团队积分池及成员额度；未取得相应官方帮助文档 | 官方入口已确认；团队具体规则仍待官方核实 |

## OpenRouter

来源：[Organization Management](https://openrouter.ai/docs/cookbook/administration/organization-management)。官方原文也可通过该路径的 `.md` 页面读取。

- 个人模式使用个人资源；组织模式中的 API 使用、credit 购买和 Key 管理代表 Organization 执行。
- Organization 有共享 credit pool；组织 Key 的用量计入组织资金池。
- 购买 credits、详细账单、付款方式和发票设置归组织管理员管理。
- 用户可属于多个 Organization；退出后失去该组织资源、credits 和 Key 的访问权，个人账户不受影响。
- 文档允许满足条件的个人 credits 显式转入 Organization，并设置 MFA、成员加入时长、转入冷却等约束。后付费组织不可接收预付 credits。**不能推断个人与组织余额可自动互相兜底。**
- Activity 可归集组织成员的用量，并按 API Key 过滤；用量可见性与详细账单可见性有不同权限。

补充官方来源：

- [Workspaces](https://openrouter.ai/docs/guides/features/workspaces)：Account Level Settings 明确写明 “Credits & Billing. Unified billing across all workspaces.”；Workspace 隔离资源，账户跨 Workspace 统一结算。
- [Workspace Budgets](https://openrouter.ai/docs/guides/features/workspaces/workspace-budgets)：Enterprise 可设置日、周、月、累计消费上限；成员额度与 Workspace 集体预算可以叠加。这是消费限制，不表示 Workspace 拥有独立余额。文档也说明已经派发的请求继续完成，预算可能小幅超出，不应照搬为严格资金冻结规则。

## TapNow

来源：

- [管理团队与权限](https://docs.tapnow.ai/zh/docs/account/manage-teams-and-permissions)
- [管理 Tapies 与套餐](https://docs.tapnow.ai/zh/docs/account/manage-tapies-and-plans)
- [与团队一起创作](https://docs.tapnow.ai/zh/docs/projects/create-with-your-team)

官方明确规定：

> 新团队拥有独立的计费账户，初始 Tapies 为 0。个人账户或其他团队中的订阅、权益和 Tapies 不会转入新团队。

成员额度有三种形式：

- 无额度限制：可使用团队余额，直到团队总余额耗尽。
- 周期额度：每天、每周或每月恢复；本周期额度耗尽后该成员暂停生成。
- 固定额度：用完后不自动恢复，需有权限的成员重新设置或恢复。

文档特别说明，无额度限制仍会消耗团队 Tapies，并不表示生成免费。这支持将成员额度理解为共享池上的使用限制，而不是成员个人资金。

团队有 Owner、Admin、Member。Owner 退出前必须先转让所有权；普通成员退出或被移除后立即失去团队资源访问权限。

创作帮助中允许画布在个人与团队空间之间移动。但文档未说明历史账单是否随画布迁移、在途任务应由谁付款，不能据此推断。

## Lovart

来源：[官方 pricing](https://www.lovart.ai/zh/pricing)。用户提及的名称为 Loveart，本次按 Lovart 调研；若所指产品不同，需要另行核实。

官方 pricing 的团队版弹层展示按席位收费、团队共享积分、各席位算力上限及每席位积分配置。调研时显示每席位每月 27,000 积分；价格和配额属于可变产品配置，不作为 IdeaBoard 架构依据。

页面 FAQ 明确说明个人账户仅供个人使用，多人使用需要购买团队版。积分 FAQ 也区分订阅周期积分与充值积分，两者有效期及滚存规则不同。

已确认的仅是产品计费与席位行为；未确认团队与 Workspace 的实体关系、个人到团队的余额转移、团队外协作者扣费或团队余额不足时的回退规则。

## LibTV

来源：

- [官网](https://www.liblib.tv/)：直接观察到个人会员、积分超市和企业采购入口；企业采购提供更多席位及专属服务。
- [极客电影团队版介绍，2026-05-20](https://www.163.com/dy/article/KTCAL91H0517DU86.html)：二手报道，已读取全文。
- [腾讯转载，2026-05-25](https://view.inews.qq.com/a/20260525A09IXT00)：搜索摘要补充席位汇总与成员上限。
- [腾讯转载，2026-05-19](https://news.qq.com/rain/a/20260519A03I2G00)：搜索摘要提及每月重置额度。

二手报道描述团队共享积分池、成员额度及消耗上限、团队空间资产保留和个人项目迁入。**这些规则未取得官方帮助文档核实，不作为已确认的官方产品承诺。**

未知项包括：是否一团队多 Workspace、外部协作者由谁付费、余额不足是否可用个人余额、迁移项目是否迁移历史账单。`libtv.gongke.net` 等第三方价格站不能当官方来源。

## 对 IdeaBoard 的讨论输入

初次调研时的代码快照（2026-10-20，非当前实现说明）：

- `apps/api/internal/credits/CONTEXT.md` 将 Wallet 定义为单个 User 的账户；`apps/api/internal/tasking/infrastructure/adapters/workspace_credits_adapter.go` 的 Hold/Capture/Release 仍传入 UserID。
- `apps/api/internal/tenancy/infrastructure/postgres/billing_account.go` 实际 provision Workspace 账单身份，Organization 支持仍为 future；这不等于已有团队共享余额。
- Task 已保存 BillingAccountID 与 SubmitterUserID，消费归集身份和操作人已有基础，资金归属链路尚未统一。

以下为设计推论，尚未决策：

1. 用户可以只有个人计费账户，Organization 不应成为个人使用的前提。
2. 团队共享资金需要稳定归属，不宜依附当前 Owner 的个人钱包。团队 Owner 更替与资金历史应分别处理。
3. Workspace 是否承担团队实体，取决于产品是否需要多个 Workspace 共用成员和资金；竞品名称本身不足以决定这一点。
4. Billing Account 可作为资金归属概念，Workspace 记录消费发生的位置，User 记录实际操作人。这是可讨论的职责划分，并不要求立即新增同名表或 Organization 层。
5. Board 的编辑权限不应自动等同于共享资金使用权；共享协作和支付授权需要分别明确。
6. 成员预算可以是共享资金池上的约束，不必先拆成个人子钱包。限制周期、并发预占和释放规则仍需单独设计。
7. 资金归属应在任务提交时确定并随结算保留；Workspace 迁移、成员退出、团队所有权转让不宜让历史消费随当前关系漂移。

下一轮应先确定：**一个 Workspace 的生成任务是否始终由同一个计费账户付款，还是允许成员在个人资金与团队资金之间选择？** 该选择会决定个人空间、团队空间、外部协作者和预算授权的模型，再决定 Organization 是否有独立存在的必要。

## 关闭、删除与退款：2026-10-23 补充

本次问题是 Organization 关闭后能否不退款。需要分别核实取消订阅、可恢复的组织关闭、永久删除账户、因违规终止，以及普通充值退款；这些动作不能互相替代。以下均为官方公开来源，尚未实际执行付款、删除或退款。

### 产品对照

| 产品 | 普通退款或取消订阅 | 终止、删除与余额 | 能否证明“组织关闭不退款、余额保留且可恢复” |
| --- | --- | --- | --- |
| TapNow | 原则上费用与已完成购买不可退款，法律要求或购买时明确承诺除外；取消订阅通常于当前付费周期结束时生效 | 终止后使用许可结束；公开条款没有承诺自动退款，也没有明确承诺关闭后保留余额、支持恢复 | 不能；可借鉴常规不退款规则，余额保留与恢复仍须自行定义 |
| OpenRouter | 未使用的购买额度可在交易后 24 小时内申请退款，平台费用不退，加密货币支付不退 | Terms §9 明示非违规终止时剩余额度 30 天内原路退还；FAQ 又提示删除后重建账户无法找回未使用额度，存在需澄清的口径差异 | 不能；其终止条款反而包含退款承诺，不能概括成“关闭一律不退” |
| Krea | 帮助文档允许满足低使用量等条件的订阅退款；取消订阅默认使用至本期结束 | 取消订阅后保留作品与账户历史；未核实 Organization 关闭或删除时的专门规则 | 不能；取消订阅保留数据不等于组织关闭永久保留余额 |

### TapNow：常规不退款，但未公开组织关闭后的完整规则

官方来源：[Terms of Service](https://www.tapnow.ai/terms-of-service) §10、§14、§22；[Enterprise](https://www.tapnow.ai/enterprise)。Terms 页面标注更新于 2025-09-01，合同主体为 Tamar AI Inc.。

- §10：“Except where required by law or expressly stated at purchase, fees and completed purchases are non-refundable.” 即原则不退，保留法律及购买承诺例外。
- Tapies/service credits 是使用功能的有限许可，无现金价值、不是法定货币；平台未另行说明时不可转让。不能将此概括成可以任意清零。
- §10 只明确 promotional Tapies 可能过期，不能据此推断全部付费额度永久有效，或全部付费额度到期作废。
- §14 规定终止后服务使用许可结束，支付等条款继续有效；没有说明组织关闭会自动退款，也没有说明关闭后余额冻结、保留或可恢复。
- Enterprise 页面明确 shared credits、成员与使用量管理；§22 允许单独签署的协议优先，因此通用条款不能代替企业合同。
- 适用范围限于已核实的国际站条款。`www.tapnow.top` 中国站请求出现 TLS 错误，中国区条款未核实；`/zh/terms-of-service` 仍显示同一英语合同。

### OpenRouter：普通退款窗口与账户终止退款必须分开

官方来源：[Terms of Service](https://openrouter.ai/terms)（页面标注更新于 2026-08-31）§4.1、§4.2、§9；[FAQ](https://openrouter.ai/docs/faq)。

- **普通购买退款**：Terms §4.1 和 FAQ 一致，未使用 Credits 须在交易处理后 24 小时内申请；未使用金额原路退回，平台费用不退，加密货币支付不退。超出窗口的普通退款不被承诺。
- **额度有效期**：Terms §4.2 保留自购买起 365 天后使未使用额度到期的权利，不等于保证额度永不过期，也不等于每笔额度必定自动到期。
- **账户终止**：Terms §9 明示：“Except for OpenRouter's termination of your account for violation of these Terms, any Credits remaining in your account upon termination will be refunded to you through your original payment method within thirty (30) days of termination.” 因违规被终止不退款；其余终止条款包含剩余额度 30 天内原路退款的承诺。
- **FAQ 口径差异**：删除账户的 FAQ 同时写道：“unused credits will be lost and cannot be reclaimed if you delete and later recreate your account.” 这明确否定删除重建后的额度找回，但没有解释它与 §9 终止退款承诺之间的关系。
- **证据边界**：不能自行判断 FAQ 已推翻 Terms，也不能推断所有删除操作都会自动退款。具体删除与终止路径的结算顺序、加密支付例外及企业合同规则，仍需 OpenRouter 澄清。未找到足以确认可恢复 Organization 关闭流程的公开规则。

#### 组织关闭后自动转入 Personal：未找到官方依据

2026-10-23 再次读取上述 Terms、FAQ 及 [Organization Management 的转移说明](https://openrouter.ai/docs/cookbook/administration/organization-management#transferring-credits-from-personal-to-organization)：

- 官方功能文档明确提供的是 **Personal → Organization** 的合格 Credits 自助转入，不是双向转账。需要启用 MFA、满足组织成员加入时长要求，并遵守组织接收转入后的冷却限制；按发票或后付费结算的组织不能接收预付 Credits。
- 同页说明，退出 Organization 会失去组织资源、Credits 和 API Keys 的访问权，Personal 账户不受影响。没有承诺组织关闭时把组织余额自动转给管理员或个人账户。
- Terms §4.4 同时写明 “Credits are non-transferable, including between accounts”，与上述已公开的单向转入功能存在文本口径差异；不能据此断言任何转账均不存在，也不能把功能文档扩大为 Organization → Personal 的许可。
- Terms §4.3 明确 promotional Credits 不可退款、不可兑换且不可转让。不能将付费 Credits 的有限退款规则应用到赠送额度。
- Terms §9 写的是符合条件的账户终止后 **原支付方式退款**，不是退成 Personal Credits；原渠道退款与账户内部余额转移是不同的产品行为。

因此，当前公开证据支持“个人与组织资金分离、满足条件的个人额度可单向转入组织、符合条款时原渠道退款”。**不支持将“组织关闭自动把余额转给 Owner 的 Personal”描述为 OpenRouter 已验证的模式。** 完整的组织关闭结算流程仍未公开确认。

### Krea：订阅退款有使用量门槛

官方来源：[Refunds and Billing](https://www.krea.ai/docs/user-guide/help-and-support/refunds-and-billing)、[Manage Subscriptions](https://www.krea.ai/docs/user-guide/help-and-support/manage-subscriptions)、[Compute Units](https://www.krea.ai/docs/user-guide/help-and-support/compute-units)、[Organization Management](https://www.krea.ai/docs/user-guide/help-and-support/organization-management)、[Terms](https://www.krea.ai/terms)。

- 退款帮助要求有效付费订阅、本月 Workspace 用量低于 650 compute units **且**少于 20 次生成，并说明通常处理近期扣款；退款预计 5–10 个工作日到账。符合条件时取消流程可提供退款选项。
- 取消订阅默认保留高级权限至本期结束；保留历史、作品与用量记录，可用原账户重新订阅。这不构成 Organization 关闭或删除规则。
- 一次性 compute pack 独立于订阅，购买额度须在 90 天内使用，不能以“数据保留”推断付费额度永久保留。
- 更换 Workspace 的 Billing Account 不会转移原账户套餐、订阅或剩余额度。
- Terms 页面标注 2024-05-20，原则上不按剩余周期比例退款、除明示例外不退已付费用；现行帮助文档明确了退款例外，引用时应一并说明。

### IdeaBoard 的产品决策（2026-10-23）

产品已明确选择购买原则上不退款，法律要求或购买时明确承诺除外；Organization 由 Owner
二次确认后永久关闭，不能恢复，剩余余额不能再使用或转入 Personal，关闭本身不自动退款。
不可变账务历史仍保留，例外退款和争议仍可按原支付事实处理。目标合同见
[ADR-0038 §4](../adr/0038-personal-organization-billing-model.md#4-organization-永久关闭与余额)。

常规不退款规则有 TapNow 官方条款依据；永久关闭及剩余余额使用权终止是本项目的明确决策，
不是已核实的 TapNow/OpenRouter 组织关闭行为。此前提出的可恢复关闭建议已撤回，不能再据此验收。
