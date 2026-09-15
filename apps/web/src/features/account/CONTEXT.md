# Account & Billing Surface

Account 是用户端管理个人凭据、额度钱包、充值收银台、用量统计明细、兑换码兑换与安全设置的垂直域。遵循严格的 DDD 分层结构（`domain`、`application`、`infrastructure`、`presentation`）。

## Language

**Idea / Credit**:
用户端统一展示文案为 `Idea / Ideas`；内部代码、领域模型、schema 与传输层保持使用 `Credit / credits`。
_Avoid_: 用户可见文案直露 `Credit`，领域层随意改名 `Idea`

**Wallet Balance**:
用户的额度账户状态（`balance`、`held`、`available = balance - held`）。事实源为服务端 PostgreSQL 账本，前端通过 `fetchCreditWallet` / `creditsQueryKeys`（`infrastructure/credits/resource.ts`）轮询与事件同步。
_Avoid_: 客户端本地自行加减计算永久余额，无服务端回包时乐观修改

**Recharge Session**:
Stripe 充值流程的状态机：选择面额套餐（PurchaseCard）-> 发起支付结账（Stripe Checkout Session）-> 外部网关重定向 -> 回跳轮询结果（ReturnStatus）。
_Avoid_: 缺少回跳终态处理，未落库订单提前标记成功

**Reward Redemption**:
兑换码兑换流程（`redeemRewardCode`），遵循实例级与 IP 级防刷频控，409 重试安全。
_Avoid_: 无频控重试，兑换成功后不同步刷新额度钱包

**Account Overlay**:
由 `accountOverlay.ts` 单例控制的全局弹窗宿主，支持激活特定 Tab（`profile / recharge / usage / billing / rewards / security`），由 `AccountDialogHost.vue` 挂载渲染。
_Avoid_: 各路由分散自建 AccountDialog 实例

## Module Ownership

| Layer / Module    | Responsibility                                                                                                                                     | Public Interface                                                                                  | Implementation Detail Boundary                                |
| :---------------- | :------------------------------------------------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------ | :------------------------------------------------------------ |
| `domain/`         | 纯数学计算、金额格式化（`formatCurrencyMinor`）、用量图表数据整形、Tab 注册表。                                                                    | `domain/index.ts`（`formatCredits`、`centsToDollars`、`createUsageChartConfig`、`accountTabs`）。 | 零 Vue 响应式与网络传输依赖，全量 deterministic 单测覆盖。    |
| `infrastructure/` | 传输适配层：封装 `credits`、`payments`、`rewards`、`usage`、`billing` 等子领域的资源传输。                                                         | 各子目录下的 `resource.ts`（经 `infrastructure/index.ts` 统一导出）。                             | 隔离 generated DTO，完成 snake_case 到 camelCase 的归一化。   |
| `application/`    | 用例编排 Composables：`useRecharge`、`useCreditWallet`、`useCreditsSync`、`useRewardRedemption`、`useUsage`、`useAvatarUpload`、`accountOverlay`。 | `application/index.ts` 导出各用例 Hook。                                                          | 协调加载态、错误反馈与 Query Cache 失效，不负责 UI 模板渲染。 |
| `presentation/`   | 视图呈现层：各 Tab Pane（`RechargePane`、`UsagePane`、`RewardsPane`、`SecurityPane`、`BillingPane`）及全局 `AccountDialogHost`。                   | `presentation/index.ts` 导出 `AccountDialogHost` 与组件。                                         | 纯展示与事件分发，所有异步动作委派给 `application`。          |

## Rules

- 外部调用该域弹窗必须统一走 `accountOverlay.open(tab?)`，严禁在页面中直接局部实例化 `AccountDialog`。
- 额度显示遵循事实源：充值成功或兑换码兑换成功后，必须显式触发 `invalidateCreditWallet()` 刷新余额缓存。
- 领域层 `domain/` 严禁引入任何 Vue 依赖或网络传输 DTO。
- 任何金额处理均在领域模型中以整数（分 / minor unit）存储与计算，仅在 presentation 层通过格式化器转为货币字符串展示。
