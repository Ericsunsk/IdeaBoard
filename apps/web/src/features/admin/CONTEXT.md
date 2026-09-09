# Admin Surface

Admin Surface 是运营人员管理用户、供应商、账本、订单、存储与系统设置的上下文。它与用户侧 Workspace 分离，拥有独立路由、布局与反馈契约。

## Language

**Admin Surface**:
`/admin/*` 下的运营操作面，包含 layout 壳、懒加载路由与侧边导航。页面只做组合，不拥有领域状态。
_Avoid_: Workspace route, user-facing page

**Resource**:
单个 Admin 域的传输边界，封装 generated API 调用、DTO 归一与错误文案，暴露可注入接口供测试替换。命名形如 `*Resource.ts` / `create*Resource()`。
_Avoid_: page-local fetch, direct generated-client call

**Workspace / Context**:
单个 Admin 域的用例编排，组合 resource + feedback + confirm + async action，对应 `*Workspace.ts` / `*Context.ts`。
_Avoid_: page-owned workflow, global store

**Collection State**:
列表类页面的通用加载契约（`data/loading/initialError/backgroundError/hasLoadedResult` + epoch 中止），由 `shared/composables/useCollectionState.ts` 拥有。
_Avoid_: per-page loading flags, uncancelled reload

**Feedback Boundary**:
Admin 的 toast 出口契约：所有用户可见反馈必须经 `shared/lib/toast.ts`，禁止直调 `useAppToast`（由 `adminFeedbackBoundary.contract.spec.ts` 强制）。
_Avoid_: scattered toast calls, raw error prose

**Idempotency Key**:
钱包 grant/deduction/refund 的幂等键（`admin-ui:kind:uuid`），409 视为可安全重放。
_Avoid_: retry without key, double-submit guard only

**Monitor Stream**:
任务监控的 SSE 快照通道，状态含 `idle/unsupported/connecting/connected/degraded`，快照直接写 Query cache。
_Avoid_: polling-only monitor, snapshot as local state

## Module Ownership

| Module            | Responsibility                                                                                                   | Public Interface                                                                                                                 | Implementation Detail Boundary                                               |
| ----------------- | ---------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `app`             | 布局壳、懒加载路由表、侧边导航、Prime 本地化。                                                                   | `routes.ts`（被全局 router 引用）、`navigationItems/byKey`（来自 `navigation.ts`，被 `AdminLayout` 消费）。无 barrel。           | 各 `Page.vue` 的内部组合不属于路由契约。                                     |
| `pages`           | login 薄路由装配（无垂直归属）；各垂直业务页与其领域同目录（`<vertical>/pages/`）。 | 被 `app/routes.ts` 懒加载；内部 import 兄弟域的 workspace/context。无 barrel。 | 真正的 resource/workspace 在兄弟目录，Page 不得自建传输调用。 |
| `shared`          | 唯一的 Admin 横向依赖：Dialog/Table/Toolbar/Pager 套件、confirm/toast、集合/分页/轮询原语、chart 与 validation。 | 无 barrel，深路径 `from '../shared/...'` 导入。                                                                                  | `useAppToast` 只能被 `shared/lib/toast.ts` 触碰（contract spec 强制）。      |
| `users`           | 用户搜索/角色/封禁/踢下线/impersonate/OAuth 设置。                                                               | `resource.ts`（`useAdminUsersResource`）、`context.ts`（`useAdminUsersContext`）、`commands.ts`（`useAdminUserCommands`）。     | 钱包弹窗复用 `credits-billing/wallets` 的 barrel，不得反向依赖 users。       |
| `credits-billing` | Idea 账本运营：钱包流水/grant、health 对账、pricing、reward codes。                                              | 唯一的跨域 barrel：`wallets/index.ts` 导出 `AdminUserWalletDialog`（被 users 页引用）。其余无 barrel。                           | 账本写入语义以后端 Credits Ledger 为准，前端只做命令组装与幂等键。           |
| `payments`        | 订单查询/退款、争议视图、Stripe 配置。                                                                           | `createAdminPaymentOrderResource`、`createAdminStripeSettingsResource`（可注入 fake adapter）。                                  | snake→camel 归一化在 adapter 层完成，不泄漏到 Page。                         |
| `provider-config` | 供应商端点 CRUD/探测、账号分页/限额/模型 patch。                                                                 | `createAdminProviderConfigResource`、`useProviderConfig`。`SearchProviderCard`/`AdminLogoPickerPopover` 可被 users 域复用。      | 20s 探测超时、page_size=100 全拉等传输细节留在 resource。                    |
| `storage`         | OSS provider/profile 配置、测试、bucket 域名。                                                                   | `createAdminStorageResource`、`createAdminStorageWorkspace`，Page 经 `useAdminStorageContext` 装配。                             | 卡片视图归一化在 workspace，不在 Page。                                      |
| `system-settings` | branding/access/mediaUpload + mail/outbound 卡片。                                                               | `useSystemSettings` 外观、`useSystemSettingsLifecycle` 通用表单机、`useOutboundPolicyResource`。                                 | 脏检查/校验/缓存同步在 lifecycle，不在各卡片。                               |
| `statistics`      | 7d/30d/1y 趋势与质量/健康图表。                                                                                  | `useAdminStatisticsResource` + `useUsageStatisticsData`（range 相同时复用 query）。                                              | 图表组装在 `usageStatistics*Charts.ts`，数据获取在 resource。                |
| `task-monitor`    | 任务列表/详情/取消、SSE 快照、运行时快照。                                                                       | `useAdminTaskListResource` + query key 生成器、`useAdminTaskList`、`useAdminTaskCancel`、`useAdminTaskMonitorStream`。           | 取消仅 `pending/running` 可用且单飞（`pendingTaskID`）。                     |
| `prompt-config`   | 按 feature slug 存取提示词模板，脏草稿保留。                                                                     | `createAdminPromptConfigResource`、`createAdminPromptConfigWorkspace`。                                                          | 空模板 warning 可存，覆盖保护在 workspace。                                  |

## Rules

- Page 必须是薄的：数据经 resource，动作经 workspace/context，Page 只做布局与绑定。
- 调用链固定为 `generated-query -> *Api.ts/*ApiAdapter.ts -> *Resource.ts -> Workspace/Context -> Page.vue`；路由页不得直调 generated client。
- 无 Pinia：状态只有 TanStack Query（key 生成器归 resource 所有）与局部 refs；跨 mutation 的失效必须显式 `invalidateQueries`。
- 危险动作（ban/删除/退款/取消）必须经 `useAdminConfirm` + feedback 双通道，不得裸调 mutate。
- 已知的三处跨域引用（wallets barrel、Stripe dialog、logo picker）是显式例外，不得新增跨域深导入。

## State Machines and Invariants

- **Collection load** (`useCollectionState`): `reload({preserveData})` → epoch 自增并 abort 旧请求 → `success`（`data/hasLoadedResult`）或 `error`（首屏 `initialError` / 后台 `backgroundError`）。旧 epoch 的 late response 必须丢弃。
- **Settings lifecycle** (`useSystemSettingsLifecycle`): `hydrate(source)` → 脏（`isDirty`）→ `validate`（`validationError`）→ `save` → `hydrate + synchronizeCache`。`saving || isDirty` 时跳过 source 覆盖。
- **Wallet idempotent command**: `validate`（refund 必带 chargeTransactionId）→ 组装 `idempotency_key` → mutate（`commandPending`）→ `invalidateWallet`。409 按“已生效可重放”文案处理，不得重复扣账。
- **Monitor stream**: `idle/unsupported/connecting/connected/degraded`；`snapshot` 直接 `setQueryData` + 回 `connected`，`snapshot_error/onerror` 进 `degraded`，`close` 回 `idle`（可见则重连）。
- **Task cancel**：仅 `pending/running` 可取消；`pendingTaskID` 单飞，成功后联刷三 key（list/detail/monitor）。
- **Prompt draft**：`refresh({preserveDirtyDraft: true})` 不得覆盖脏草稿；空模板为 warning（可存）而非 error。

## Payment / Credit UI States

与 `features/account`（`application/recharge`、`presentation/recharge`）共享同一账本读模型；前端只做状态呈现与命令组装，Quote/Hold/Capture/Release 语义以后端 Credits Ledger 为准。

- **Checkout intent**（`account/application/recharge/useRecharge.ts`）：`checkoutIntent{fingerprint: quantity:terms:refund, idempotencyKey: uuid}`；`checkout_request_rejected/session_expired/payment_already_completed` 按序轮转；pending 态为 `checkout_open/async_payment_pending/paid_pending_credit`，由 SSE `payment.updated` 推进并联动失效 pending/history/credits/billing。
- **Return status**（`presentation/recharge/ReturnStatus.vue`，优先级从高到低）：`credited` → `cancel(returned)` → `pending(confirming)` → `error`（可 retry）→ `failed` → `delayed`（60s timer）→ `confirming`。
- **Pending order item**（`PendingOrders.vue`）：`checkout_open`（继续 + 放弃双钮）/ `async_payment_pending` / `paid_pending_credit`（`crediting` vs 超 60s `creditingDelayed`）/ `expired` / `cancelled` / `payment_failed` / `credited`；`expiryText` 5s ticker。
- **Wallet dialog**（`credits-billing/wallets/AdminUserWalletDialog.vue`）：`wallet.status = frozen` 或用户停用即冻结态（列出 `activeRestrictions`）；`grant/deduction` 钮在 `commandPending`、无 wallet 或金额 < 1 时禁用；账本行按 `hold→冻结 / capture→消费 / grant→赠送 / deduction→扣减 / release→解冻 / refund→退款 / correction→修正` 呈现，`+` 仅 grant/release/refund。
- **Order presentation**（`payments/orders/paymentOrderPresentation.ts`）：`creating→创建中 / checkout_open→待支付 / async_payment_pending→异步确认中 / paid_pending_credit→待到账 / credited→已到账 / cancelled→已取消 / expired→已过期 / payment_failed→支付失败`，附 stripe/credit 二级标签。
- **Vision search billing**（`search/infrastructure/visionBillingResource.ts`）：`sku=ai-search:vision` 纯询价展示 + `insufficient_credits/pricing_unavailable`；`hold/capture` 不在用户侧展示，只进 admin 账本。
- Invariants：用户侧不得展示 `hold/capture` 中间态；金额颜色 `+` 仅三类（grant/release/refund）；`system_adjustment` 是账务域词汇，与图像 retouch 无关，不得改名。
