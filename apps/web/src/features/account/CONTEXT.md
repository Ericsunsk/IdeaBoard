# Account Context

Account owns the user-side surface for credentials, credit wallet, recharge checkout, usage details, reward redemption, and security settings, in strict `core` / `session` / `ui` lanes.
It does NOT own the server-side ledger, the Stripe gateway itself, or pricing policy.

## Domain Language

| Term              | Meaning                                                                                                                                                                     |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Idea / Credit     | User-visible copy is always `Idea`/`Ideas`; code, core models, schemas, and transport stay `Credit`/`credits`.                                                              |
| Wallet Balance    | Available and reserved Ideas reported by Credits; UI values are projections of the server Ledger, not a separate balance.                                                   |
| Recharge Session  | The user-facing top-up flow from quantity selection through hosted Checkout and confirmation of the resulting Payment Order.                                                |
| Reward Redemption | A user's successful RewardCode claim and credited award; retrying the same claim must not issue another award.                                                              |
| Account Overlay   | Global dialog host singleton in `session/overlay.ts` for tabs (`profile` / `recharge` / `usage` / `billing` / `rewards` / `security`), rendered by `AccountDialogHost.vue`. |

## Public Seams

- `core/index.ts` pure quantity normalization (`normalizeIdeaQuantity`, `MIN_IDEA_QUANTITY`), payment-return query parsing, account-tab keys/resolution, and chart/quantity types.
- `session/index.ts` unified export for `credits`, `payments`, `rewards`, `usage`, `billing`, `billingAccount` transport resources, orchestration hooks, and runtime chart/date/currency formatting.
- `session/budget` owns the workspace shared consumption budget (#945): `fetchWorkspaceBudget`/`updateWorkspaceBudget` over the generated workspace-billing budget endpoints with strict envelope parsing, `useWorkspaceBudgetSection` for the settings-dialog section (query + invalidation, never optimistic). Only billing managers see management controls; every member reads usage. The backend still enforces 403 for every write.
- `session/quota` owns the member cross-workspace consumption quota (#946): `fetchSelfMemberQuota`/`fetchMemberQuotaList`/`updateMemberQuota` over the generated workspace-billing member-quota endpoints with strict envelope parsing, `useMemberQuotaSection` for the settings-dialog section (self read for every member plus manager list/editor with roster picker, query + invalidation, never optimistic). Only billing managers see management controls; members read their own usage. The backend still enforces 403 for every write.
- `session/billing-transfer` owns the Workspace-settings handover and financial exit interaction (#948): financial Owners initiate for an existing Workspace member, recipients discover addressed invites before becoming financial managers, recipients accept/reject and initiators cancel. Decisions retain their idempotency key across retries; account, Workspace, roster and money reads invalidate after success. Financial manager removal is confirmed and protects the last Owner; content membership remains independent.
- `session/useWorkspaceCreation` pins the selected personal payer before submit, exposes loading/error/retry, ignores obsolete resolver responses, and coordinates explicit Organization account setup before its first Workspace. Organization scope never sends personal billing overrides.
- `session/quota` also owns the account default new-member quota (#947): `fetchDefaultMemberQuota`/`updateDefaultMemberQuota` over the generated default-member-quota endpoints, `useDefaultMemberQuotaSection` for the settings-dialog default section (monthly presets plus five periods, query + invalidation, never optimistic). Absent defaults fail closed with guidance (never implicit unlimited); changes affect only later joins; invites reuse the default atomically and the inviter cannot raise it.
- `session` use-case hooks: `useRecharge`, `useCreditWallet`, `useCreditsSync`, `useRewardRedemption`, `useUsage`, `useBillingAccount`, `useAvatarUpload`, `accountOverlay`.
- `ui/index.ts` view layer: `RechargePane`, `UsagePane`, `RewardsPane`, `SecurityPane`, `BillingPane`, and `AccountDialogHost`.
- `accountOverlay.open(tab?)` the only entry to open the account dialog.

## Billing Account Scope (issue #922)

- Funds identity is the stable Billing Account (`ba_`): every Workspace sharing one account reports the same balance, with or without an Organization; independent accounts never share cache entries.
- `useBillingAccountContext` owns account resolution, identity isolation and read readiness for a caller-selected Workspace. Settings keeps its local target; `useBillingAccount` supplies the global Workspace selection to that same context. Handover and Settings share its cache/readiness contract, so financial-role changes refresh every settings reader. It supplies immutable identity-aware contexts to query keys: `walletScope` for balance and `readScope` (including server-derived consumption visibility) for Usage/Billing Records. Dependent queries never assemble or re-resolve this context themselves.
- The same module owns prerequisite-error precedence, prerequisite-first retry, and matching retained consumption results against the current authorized context. Disabled reads and failed resolution hide cached financial data; query-key snapshots pin the requested identity and Workspace even when responses arrive late.
- Account resolution failures propagate to balance, billing, and usage error states; retry reloads the failing prerequisite before the dependent read. Successful usage ranges may be retained only within the same Workspace and Billing Account.
- Raw Ledger and recharge orders require Billing Owner/Admin. Billing Records and Usage share server-derived consumption scopes: financial managers see the account, Workspace Owner/Admin sees the current Workspace, and ordinary members see their own consumption across the account. Scoped Billing Records exclude non-consumption entries and historical account balances. The balance projection remains available to Workspace members for consumption UI.
- Query functions read scope from the query key, never from the closure, so cross-account switches and late responses cannot mix data; balance responses naming a different payer fail closed.
- Settlement attributes the real operator and the consuming Workspace and never rewrites history; there is no owner-wallet projection or fallback.
- `invalidateUserMoney` refreshes credits + billing records + usage together; reward history stays at the redemption site.

## Invariants

- External callers open the dialog only via `accountOverlay.open(tab?)`; never instantiate `AccountDialog` locally per page.
- Balance display follows the source of truth: after recharge or redemption success, explicitly call `invalidateCreditWallet()`.
- Never compute a permanent balance by local add/subtract; never optimistically mutate without a server round-trip.
- `core/` never depends directly or transitively on Vue, runtime i18n, DOM, or transport DTOs; it stays deterministic with unit coverage. Display adapters live in `session/chart.ts`, `session/formatters.ts`, and `session/money.ts`.
- All money is stored and computed as integers (minor units); `ui` displays currency strings formatted by `session`.
- Amount display uses the shared `formatIdeaAmount` formatter.
- Transport normalization (snake_case to camelCase) happens in adapter/resource layers under `session/`.
- `session` coordinates loading, error feedback, and Query Cache invalidation; it never renders UI templates.
- `ui` is display plus event dispatch; all async work delegates to `session`.
- Recharge returns must always handle a terminal state; never mark an order successful before it is persisted.
- Redemption always respects rate limits and refreshes the wallet on success; no unbounded retries.

## Non-goals

- No user-visible `Credit` copy; no `Idea` renaming inside the core layer.
- No per-route `AccountDialog` instances.
- No Vue reactivity or network dependencies in `core/`.
