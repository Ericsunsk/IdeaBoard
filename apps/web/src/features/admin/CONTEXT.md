# Admin Context
Admin Surface owns operator management of users, providers, ledger, orders, storage, and system settings under `/admin/*`.
It has its own routes, layout shell, and feedback contract, separate from user-side Workspace.
It does NOT own user-facing pages, domain ledger semantics (backend Credits Ledger is fact source), or cross-vertical workflows beyond five explicit exceptions.

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Admin Surface | Operator surface under `/admin/*`: layout shell, lazy routes, side nav; pages compose only and own no domain state. |
| Resource | Per-vertical transport boundary (`*Resource.ts` / `create*Resource()`): wraps generated API calls, DTO normalization, error copy; injectable interface for test doubles. |
| Workspace / Context | Per-vertical use-case orchestration (`*Workspace.ts` / `*Context.ts`): composes resource plus feedback plus confirm plus async action. |
| Collection State | TanStack Query list contract: query key carries filter/cursor, key change refetches (no data is first-screen `initialError`), same-key `refetch` is background refresh (stale kept, failure is `backgroundError`), `retry: false` fail-fast; cursor lists converge on `shared/composables/useAdminCursorCollection` plus `shared/components/AdminCollectionPageShell`; sample in `credits/rewards/AdminRewardCodesPage.vue`; legacy `useCollectionState` is retired. |
| Feedback Boundary | Admin toast exit contract: all user-visible feedback goes through `shared/lib/toast.ts`; direct `useAppToast` is forbidden (enforced by `adminFeedbackBoundary.contract.spec.ts`). |
| Idempotency Key | Wallet grant/deduction/refund idempotency key (`admin-ui:kind:uuid`); 409 means safe replay. |
| Monitor Stream | Task-monitor SSE snapshot channel with `idle` / `unsupported` / `connecting` / `connected` / `degraded`; snapshots write Query cache directly. |
| Checkout Intent | Recharge intent in `account/application/recharge/useRecharge.ts`: `checkoutIntent{fingerprint: quantity:terms:refund, idempotencyKey: uuid}`; `checkout_request_rejected` / `session_expired` / `payment_already_completed` rotate in order; pending is `checkout_open` / `async_payment_pending` / `paid_pending_credit`, advanced by SSE `payment.updated` with invalidation of pending/history/credits/billing. |
| Return Status | Priority in `presentation/recharge/ReturnStatus.vue`: `credited` to `cancel(returned)` to `pending(confirming)` to `error` (retryable) to `failed` to `delayed` (60s timer) to `confirming`. |
| Pending Order Item | States in `PendingOrders.vue`: `checkout_open` (continue plus abandon) / `async_payment_pending` / `paid_pending_credit` (`crediting` vs over-60s `creditingDelayed`) / `expired` / `cancelled` / `payment_failed` / `credited`; `expiryText` on 5s ticker. |
| Wallet Dialog | States in `credits/wallets/AdminUserWalletDialog.vue`: `wallet.status = frozen` or deactivated user is frozen (lists `activeRestrictions`); `grant` / `deduction` disabled when `commandPending`, no wallet, or amount below 1; ledger rows map `hold` to frozen, `capture` to consumed, `grant` to granted, `deduction` to deducted, `release` to unfrozen, `refund` to refunded, `correction` to corrected; `+` only for grant/release/refund. |
| Order Presentation | States in `payments/orders/paymentOrderPresentation.ts`: `creating` / `checkout_open` / `async_payment_pending` / `paid_pending_credit` / `credited` / `cancelled` / `expired` / `payment_failed`, with stripe/credit sub-labels. |
| Vision Search Billing | Pricing-only display in `search/infrastructure/visionBillingResource.ts` (`sku=ai-search:vision`) with `insufficient_credits` / `pricing_unavailable`; `hold` / `capture` never show user-side, only in the admin ledger. |

## Public Seams

- `app/routes.ts` — lazy admin route table referenced by the global router in `app`.
- `app/navigation.ts` — `navigationByKey` / `groups` consumed by `AdminLayout` in `app` (`navigationItems` is an internal intermediate, not exported).
- `pages` — thin login route assembly plus per-vertical pages under `<vertical>/pages/`; lazy-loaded by `app/routes.ts`; pages import sibling-domain workspace/context, never build their own transport.
- `shared` — sole Admin horizontal dependency: Dialog/Table/Toolbar/Pager kit, confirm/toast, collection/pagination/polling primitives, chart, validation; deep `from '../shared/...'` imports, no barrel.
- `useAdminUsersResource` (`users/resource.ts`) — user search/role/ban/kick/impersonate/OAuth-settings transport.
- `useAdminUsersContext` (`users/context.ts`) — user use-case orchestration.
- `useAdminUserCommands` (`users/commands.ts`) — user commands.
- `credits/wallets/index.ts` — sole cross-vertical barrel exporting `AdminUserWalletDialog` (referenced by users pages); all other verticals have no barrel.
- `createAdminPaymentOrderResource` / `createAdminStripeSettingsResource` — order query/refund, dispute view, Stripe settings with injectable fake adapter in `payments`; snake-to-camel normalization stays in the adapter layer.
- `createAdminStorageResource` / `createAdminStorageWorkspace` — OSS provider/profile config, test, bucket domain in `storage`; pages assemble via `useAdminStorageContext`; card-view normalization lives in workspace.
- `useSettings` / `useSettingsLifecycle` / `useOutboundPolicyResource` — branding/access/mediaUpload plus mail/outbound cards in `settings`; dirty-check/validation/cache-sync live in lifecycle, not cards.
- `useAdminStatisticsResource` plus `useUsageStatisticsData` — 7d/30d/1y trends and quality/health charts in `statistics`; shared query when range matches; assembly in `usageStatistics*Charts.ts`.
- `useAdminTaskListResource` plus query-key generators / `useAdminTaskList` / `useAdminTaskCancel` / `useAdminMonitorStream` — task list/detail/cancel, SSE snapshot, runtime snapshot in `monitor`; explicit demo mode (`?demo=1`, see `demoData.ts`) with banner, local pagination, write-disabled cancel/detail, off by default.
- `createAdminPromptConfigResource` / `createAdminPromptConfigWorkspace` — prompt-template CRUD by feature slug with dirty-draft retention in `prompts`.
- `createAdminGatewayResource` — model-library catalog/connection/mappings with generated client plus snake-to-camel normalization in `models`.
- `AdminModelSelect` — sole selection exit for the pricing page in `models` (`allowCustom=false`); pricing consumes catalog and mappings via `usePricingModelAdapter` on one shared queryKey.

## Invariants

- Pages stay thin: data via resource, actions via workspace/context; pages only lay out and bind.
- Call chain is `generated-query -> *Api.ts/*ApiAdapter.ts -> *Resource.ts -> Workspace/Context -> Page.vue`; route pages never call the generated client directly.
- No Pinia: state is TanStack Query (key generators owned by resource) plus local refs; cross-mutation invalidation is explicit `invalidateQueries`.
- Dangerous actions (ban/delete/refund/cancel) require `useAdminConfirm` plus feedback dual-channel; never bare `mutate`.
- Only five known cross-vertical refs are allowed (wallets barrel, Stripe dialog, logo picker, storage full-page embedded in settings, shell anomaly-radar aggregating statistics); no new cross-vertical deep imports.
- Collection load (TanStack Query): key change fetches (fetching without data is first-screen loading, failure is `initialError`); same-key `refetch` is background refresh (stale kept, failure is `backgroundError`); key isolation means late responses never overwrite the current page.
- Settings lifecycle (`useSettingsLifecycle`): `hydrate(source)` to dirty (`isDirty`) to `validate` (`validationError`) to `save` to `hydrate + synchronizeCache`; skip source overwrite while `saving || isDirty`.
- Wallet idempotent command: `validate` (refund requires chargeTransactionId) to idempotency-key assembly to mutate (`commandPending`) to `invalidateWallet`; 409 is treated as already-applied replay, never double-charges.
- Monitor stream: `idle` / `unsupported` / `connecting` / `connected` / `degraded`; `snapshot` writes `setQueryData` and returns to `connected`, `snapshot_error` / `onerror` enters `degraded`, `close` returns to `idle` (reconnect when visible).
- Task cancel: only `pending` / `running` cancellable; single-flight via `pendingTaskID`; success refreshes list/detail/monitor keys.
- Prompt draft: `refresh({preserveDirtyDraft: true})` never overwrites dirty drafts; empty template is a warning (savable), not an error.
- User side never shows `hold` / `capture` intermediate states; amount `+` only for grant/release/refund; `system_adjustment` is ledger vocabulary unrelated to image retouch and must not be renamed.
- Only `shared/lib/toast.ts` may touch `useAppToast`.
- Admin messages are zh-only and colocated with their feedback factory (pattern: `payments/messages.ts`); never `t()`-wrapped. Cross-surface rules live in `shared/CONTEXT.md` (Copy Policy).

## Non-goals

- No Workspace routes or user-facing pages in Admin.
- No page-local fetch; no direct generated-client calls from pages.
- No page-owned workflows; no global store.
- No per-page loading flags; no uncancelled reloads; no hand-rolled epoch/abort lifecycles.
- No scattered toast calls; no raw error prose.
- No retry without key; no double-submit guard alone.
- No polling-only monitor; no snapshot as local state.
- No ledger write semantics in frontend; frontend only assembles commands and idempotency keys.
- No hand-entered off-catalog model IDs on the pricing page; new models enter the catalog first.
