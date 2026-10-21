# 团队账单池竞品调研

调研日期：2026-10-20。本文记录公开产品行为及其来源，供 IdeaBoard 的账单归属讨论使用；不是已接受的架构决策，也不据此推断竞品数据库模型。

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

当前代码事实：

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
