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

- Cross-feature orchestration uses `account/index.ts`; Workspace creation, Member Quota, and Billing Transfer controllers are available there. Workspace Settings transport integration uses the four exact `session/accountWorkspace.ts`, `session/billing-account/resource.ts`, `session/billing-transfer/resource.ts`, and `session/quota/resource.ts` public subpaths declared in the [Seam inventory](../../../../../docs/frontend-seam-inventory.md), including typed transport mocks.
- `core/index.ts` pure quantity normalization (`normalizeIdeaQuantity`, `MIN_IDEA_QUANTITY`), payment-return query parsing, account-tab keys/resolution, and chart/quantity types.
- `session/index.ts` unified export for `credits`, `payments`, `rewards`, `usage`, `billing`, `billingAccount` transport resources, orchestration hooks, and runtime chart/date/currency formatting.
- `session/workspace-settings` owns Settings read orchestration and member actions. `provideWorkspaceSettingsReads` receives the dialog selection, viewer identity, and enabled gate; Member, Transfer, and Quota consume its roster/account reads through the same hooks used for independent mounting. Roster reuse matches the Workspace; account reuse additionally matches identity. Disabled sections and dialogs hide retained data, and retries target only the failed read. The module hides provide/inject, fallback query ownership, and shared-source selection from UI and other session modules.
- `session/quota` owns the member cross-workspace consumption quota (#946, #953): `fetchSelfMemberQuota`/`fetchMemberQuotaList`/`updateMemberQuota` over the generated workspace-billing member-quota endpoints with strict envelope parsing, `useMemberQuotaSection` for `WorkspaceLimitsSection` in the settings dialog (self read for every member plus manager list/editor with roster picker, query + invalidation, never optimistic). New members default to unlimited with no default gate; Organization scopes aggregate by (Organization Billing Account, User) with Owner always unlimited (Owner sets Admin, Owner/Admin set Member, Admin never touches Owner/Admin); Personal scopes use the Actor's Personal Billing Account and have no member quota checks or management (ADR-0042). Quota management UI consumes the server-resolved `can_manage_member_quotas` capability from the workspace Billing Account projection and enforces the Organization role matrix using Organization membership. Consumption visibility and roles in unrelated scopes never grant quota management. Removal/rejoin preserves quota state plus history. The backend still enforces 403 for every unauthorized write.
- `session/billing-transfer` owns the Workspace-settings handover interaction (#948): a Billing Owner starts it from a non-self member row behind an ordinary danger confirm, the recipient accepts/rejects from their own member row (replacing the leave/role controls there), and the slim transfer section only tracks invitations (pending/accepted plus initiator cancel) with the standing rules. Invites lapse 7 days after issue (`expires_at`): a lapsed invite moves nothing and the owner may send again. Decisions retain their idempotency key across retries; every handover confirmation mints a fresh intent so a later re-issue never replays an old invite. Every transfer mutation refreshes invites, managers and the workspace roster. Acceptance additionally refreshes the Workspace list, affected viewer-specific Billing Account capability projections, quotas, and scoped consumption reads; unrelated accounts/viewers and wallet balances stay untouched. Content membership remains independent.
- `session/useWorkspaceCreation` owns the scope-only create form (ADR-0038): Personal or an Organization by actual name (absent Organization offers the create-Organization path), no billing choice and no payer lookup; the scope is fixed at creation and coordinates explicit Organization setup before its first Workspace.
- Workspace settings dialog (in `workspace/ui/settings/WorkspaceSettingsDialog`) hosts Organization management opaquely through `UserAccountMenuWorkspace.organizationPanel` (component, opaque bindings, title, visibility and refresh). Organization facts, capabilities and commands belong to Workspace session; Account neither resolves roles nor forwards member-action callbacks. The dialog owns only local navigation, grouped Workspace rows, visibility and dismissal. Workspace creation retains its separate scope-only create form.
- `session` use-case hooks: `useRecharge`, `useCreditWallet`, `useCreditsSync`, `useRewardRedemption`, `useUsage`, `useBillingAccount`, `useAvatarUpload`, `accountOverlay`.
- `ui/index.ts` view layer: `RechargePane`, `UsagePane`, `RewardsPane`, `SecurityPane`, `BillingPane`, and `AccountDialogHost`.
- `accountOverlay.open(tab?)` the only entry to open the account dialog.

Workspace `session/useMemberQuotaSettings.ts` composes the Member Quota settings interaction around `useMemberQuotaSection`. Workspace owns Organization membership provenance, combined read readiness and retry, editor drafts, unlimited confirmation, and successful dismissal; Account keeps quota reads, target selection and labels, validation, Organization role policy, the live pre-write guard, feedback, and invalidation. The UI consumes the Workspace interaction without assembling these steps.

## Billing Account Scope (issue #922)

- Funds identity is the stable Billing Account (`ba_`): every Workspace in one scope reports the same balance (Personal account or Organization pool); scopes never share cache entries.
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
