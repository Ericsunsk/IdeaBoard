# Admin Context

Admin Surface owns operator management of users, providers, ledger, orders, storage, and system settings under `/admin/*`.
It has its own protected routes, layout shell, and feedback contract. Authentication belongs to Auth through `/user/login`; access requires the Admin role.
It does NOT own user-facing pages, domain ledger semantics (backend Credits Ledger is fact source), or cross-vertical workflows beyond five explicit exceptions.

## Domain Language

| Term                | Meaning                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Admin Surface       | Operator surface under `/admin/*`: layout shell, lazy routes, side nav; pages compose only and own no domain state.                                                                                                                                                                                                                                                                                                                                 |
| Resource            | Per-vertical transport boundary (`*Resource.ts` / `create*Resource()`): wraps generated API calls, DTO normalization, error copy; injectable interface for test doubles.                                                                                                                                                                                                                                                                            |
| Workspace / Context | Per-vertical use-case orchestration (`*Workspace.ts` / `*Context.ts`): composes resource plus feedback plus confirm plus async action.                                                                                                                                                                                                                                                                                                              |
| Collection State    | TanStack Query list contract: query key carries filter/cursor, key change refetches (no data is first-screen `initialError`), same-key `refetch` is background refresh (stale kept, failure is `backgroundError`), `retry: false` fail-fast; cursor lists converge on `shared/session/collection/useAdminCursorCollection` plus `shared/ui/shell/AdminCollectionPageShell`; sample in `credits/ui/rewards/AdminRewardCodesPage.vue`.                |
| Feedback Boundary   | Admin toast feedback contract: Admin toast feedback goes through `shared/session/feedback/toast.ts`; direct `useAppToast` is forbidden (enforced by `scripts/checkAdminFeedbackBoundary.ts`).                                                                                                                                                                                                                                                       |
| Idempotency Key     | The identity of an intended mutation; the same key and request replay its result, while the same key with different content conflicts.                                                                                                                                                                                                                                                                                                              |
| Monitor Stream      | Task-monitor SSE snapshot channel with `idle` / `unsupported` / `connecting` / `connected` / `degraded`; snapshots write Query cache directly.                                                                                                                                                                                                                                                                                                      |
| Wallet Dialog       | States in `credits/ui/wallets/AdminUserWalletDialog.vue`: `wallet.status = frozen` or deactivated user is frozen (lists `activeRestrictions`); `grant` / `deduction` disabled when `commandPending`, no wallet, or amount below 1; ledger rows map `hold` to frozen, `capture` to consumed, `grant` to granted, `deduction` to deducted, `release` to unfrozen, `refund` to refunded, `correction` to corrected; `+` only for grant/release/refund. |
| Order Presentation  | States in `credits/core/orders/paymentOrderPresentation.ts`: `creating` / `checkout_open` / `async_payment_pending` / `paid_pending_credit` / `credited` / `cancelled` / `expired` / `payment_failed`, with stripe/credit sub-labels.                                                                                                                                                                                                               |

## Public Seams

- `app/routes.ts` — lazy admin route table referenced by the global router in `app`.
- `app/navigation.ts` — `navigationByKey` / `groups` consumed by `AdminLayout` in `app` (`navigationItems` is an internal intermediate, not exported).
- `pages` — per-vertical views under `<vertical>/ui/views/` or `<vertical>/ui/pages/` and settings cards under `settings/ui/cards/`; lazy-loaded by `app/routes.ts`; pages import sibling-domain workspace/context, never build their own transport.
- `shared` — sole Admin horizontal dependency via `shared/index.ts` (sole public entry): `shared/ui` holds the Dialog/Table/Toolbar/Pager kit, chart and logo pickers; `shared/session` holds confirm/toast plus collection/pagination/polling primitives; `shared/core` holds validation, tokens and pure transport helpers. New code imports from the barrel.
- `useAdminUsersResource` (`users/session/resource.ts`) — user search/role/ban/kick/impersonate transport.
- `createOAuthProviderSettingsResource` (`settings/session/oauth/resource.ts`) — OAuth login settings transport; `settings/session/oauth` owns the dialog, provider logos, and operator copy.
- `useAdminUsersContext` (`users/session/context.ts`) — user use-case orchestration.
- `useAdminUserCommands` (`users/session/commands.ts`) — user commands.
- `AdminUserWalletDialog` (`credits/ui/wallets/AdminUserWalletDialog.vue`) — cross-vertical dialog referenced directly by users pages; no vertical barrels (shared crosses only via `shared/index.ts`).
- `createAdminPaymentOrderResource` / `createAdminStripeSettingsResource` — order query/refund, dispute view, Stripe settings with injectable fake adapter in `credits/session/orders` and `settings/session/payment`; snake-to-camel normalization stays in the adapter layer.
- `createAdminStorageResource` / `createAdminStorageWorkspace` — OSS provider/profile config, test, bucket domain in `settings/session/storage`; pages assemble via `useAdminStorageContext`; card-view normalization lives in workspace.
- `useSettings` / `useSettingsLifecycle` / `useOutboundPolicyResource` — branding/access/mediaUpload plus mail/outbound cards in `settings`; dirty-check/validation/cache-sync live in lifecycle, not cards.
- `useAdminStatisticsResource` plus `useUsageStatisticsData` — 7d/30d/1y trends and quality/health charts in `statistics`; shared query when range matches; assembly in `usageStatistics*Charts.ts`.
- `useAdminTaskListResource` plus query-key generators / `useAdminTaskList` / `useAdminTaskCancel` / `useAdminMonitorStream` — task list/detail/cancel, SSE snapshot, runtime snapshot in `monitor`.
- `createAdminPromptConfigResource` / `createAdminPromptConfigWorkspace` — current prompt-template read/update by Service slug with dirty-draft retention in `prompts`.
- `createAdminGatewayResource` — model-library catalog/connection with generated client plus snake-to-camel normalization in `models`.

## Invariants

- Pages stay thin: data via resource, actions via workspace/context; pages only lay out and bind.
- Call chain is `generated-query -> *Api.ts/*ApiAdapter.ts -> *Resource.ts -> Workspace/Context -> Page.vue`; route pages never call the generated client directly.
- No Pinia: state is TanStack Query (key generators owned by resource) plus local refs; cross-mutation invalidation is explicit `invalidateQueries`.
- Dangerous actions (ban/delete/refund/cancel) require `useAdminConfirm` plus feedback dual-channel; never bare `mutate`.
- Confirmation returns a boolean; mutation transport carries the request ID and idempotency key. Audit records retain the actor, operation, target, outcome and timestamp.
- Only five known cross-vertical refs are allowed (wallets barrel, Stripe dialog, logo picker, storage full-page embedded in settings, shell anomaly-radar aggregating statistics); no new cross-vertical deep imports.
- Collection load (TanStack Query): key change fetches (fetching without data is first-screen loading, failure is `initialError`); same-key `refetch` is background refresh (stale kept, failure is `backgroundError`); key isolation means late responses never overwrite the current page.
- Settings lifecycle (`useSettingsLifecycle`): `hydrate(source)` to dirty (`isDirty`) to `validate` (`validationError`) to `save` to `hydrate + synchronizeCache`; skip source overwrite while `saving || isDirty`.
- Wallet idempotent command: `validate` (refund requires chargeTransactionId) to idempotency-key assembly to mutate (`commandPending`) to `invalidateWallet`; successful replay is reported explicitly, while HTTP 409 is a conflict requiring refresh and review.
- Monitor stream: `idle` / `unsupported` / `connecting` / `connected` / `degraded`; `snapshot` writes `setQueryData` and returns to `connected`, `snapshot_error` / `onerror` enters `degraded`, `close` returns to `idle` (reconnect when visible).
- Task cancel: only `pending` / `running` cancellable; single-flight via `pendingTaskID`; success refreshes list/detail/monitor keys.
- Prompt draft: `refresh({preserveDirtyDraft: true})` never overwrites dirty drafts; empty template is a warning (savable), not an error.
- User side never shows `hold` / `capture` intermediate states; amount `+` only for grant/release/refund; `system_adjustment` is ledger vocabulary unrelated to image retouch and must not be renamed.
- Only `shared/session/feedback/toast.ts` may touch `useAppToast`; Admin toast feedback consumers use `useAdminToast` from that adapter.
- Admin messages are zh-only and colocated with their feedback factory (pattern: `credits/ui/orders/messages.ts`); never `t()`-wrapped. Cross-surface rules live in `core/CONTEXT.md` (Copy Policy).

## Non-goals

- No Workspace routes or user-facing pages in Admin.
- No page-local fetch; no direct generated-client calls from pages.
- No page-owned workflows; no global store.
- No per-page loading flags; no uncancelled reloads; no hand-rolled epoch/abort lifecycles.
- No scattered toast calls; no raw error prose.
- No retry without key; no double-submit guard alone.
- No polling-only monitor; no snapshot as local state.
- No ledger write semantics in frontend; frontend only assembles commands and idempotency keys.
- No Provider Model as a price identity. Model Pool entries reference the model catalog; prices use Service SKUs and Output Resolutions.
