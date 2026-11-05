# MemberQuotas Context

MemberQuotas is Organization-only under [ADR-0042](../../../../docs/adr/0042-personal-payer-is-actor.md),
which supersedes the Personal quota rules in ADR-0038. New Organization
Members default to unlimited (absent row reads as unlimited); invites
never gate on a default (the #947 default table, domain, API, and
fail-closed gate are retired, no dual implementation). Organization Owner
is always unlimited via the check short-circuit; Owner sets Admin,
Owner/Admin set Member, Admin never touches Owner/Admin (403). Quotas
aggregate by (Organization Billing Account, User) across all Org Workspaces.
Personal consumption bills the Actor's own Personal Billing Account and
has no quota checks or holder-managed limits. Personal callers return the
unlimited display constant without entering the quota module; unsupported
Personal quota operations fail closed.

English agent docs: this file records the member cross-workspace
consumption quota (#946) domain language. Human-facing product copy stays
in Simplified Chinese in the frontend locales.

## Package Map

- `quota.go`: pure period/validation vocabulary (`PeriodDaily`,
  `PeriodWeekly`, `PeriodMonthly`, `PeriodFixed`, `PeriodUnlimited`,
  `MaxLimitCredits`, `ValidateSet`, `IsUnlimited`, UTC day / Monday-start
  week / month bounds). No I/O.
- `policy.go`: Organization-only (#975) quota read/write role matrix
  behind the `OrganizationRoleReader` seam. Every entry point
  (`CanManage`/`CanSet`/`NormalizeRead`/`NormalizeReadBatch`) fails
  closed on Personal scopes with `ErrPersonalQuotaUnsupported` (no
  Personal quotas, never a silent denial or success); callers return
  the unlimited constant directly without entering Policy.
- `read.go`: `Reader.Self` / `Reader.Page` own normalized quota readings,
  one UTC window plan per page, and required batched role/usage reads.
  Both are Org-only and fail closed on Personal scopes; Personal callers
  return `Reading{Period: unlimited}` directly without entering
  Reader/Policy/Scope.
  There is no per-member fallback for page reads.
  The overview and individual HTTP reads use this same module; callers pass
  the ambient snapshot context. `Reader.Read` derives usage for a saved or
  already normalized cap. Identity and HTTP rendering stay outside.
- `store.go`: postgres persistence plus the Organization-scope
  settlement check. The only writer of `billing_account_member_quotas`;
  delegates complete Used Credits reads to `consumptionread`. It never
  writes the Credits Ledger. The Organization-only fence
  (`requireOrganizationAccount`) runs before the Owner short-circuit
  (`isOrganizationOwner`), which runs before the quota-row lock in
  both `CheckQuotaInTx` and `CheckQuotaForQuote`.

## Domain Language

| Term              | Meaning                                                                                                                                                                                    |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Member Quota      | One cumulative cap per (Organization Billing Account, User) on settled + pending consumption in the current window, across every Org Workspace sharing the account. Not a wallet, grant, or reservation. |
| Period            | `daily` (UTC day), `weekly` (UTC Monday-start week), `monthly` (UTC month), `fixed` (lifetime cumulative), or `unlimited` (no cap, the default). No stacking, no rollover.            |
| Used Credits      | Settled net captures attributed by submit time minus linked refunds (each refund corrects its original capture's window) plus pending Holds, for one member across Org Workspaces sharing the account.      |
| Remaining Credits | Capped limit minus used, clamped at zero; absent for unlimited.                                                                                                                           |
| Quota Exhausted   | A request that would exceed the member's remaining quota (`ErrQuotaExhausted`, wire `member_quota_exhausted` 403). Distinct from insufficiency (402) and restriction (403).    |

Canonical accounting (see `GLOSSARY.md` and Credits `CONTEXT.md`): Used/Remaining Credits are quota-window projections, never Wallet balances, and refunds correct windows only under the full names linked Refund / linked Purchase Refund.

## Public Seams

- `Store.GetQuotaRow` / `Store.SetQuota` / `Store.ListQuotas` /
  `Store.UsageForPeriod`: Organization quota reads and administration.
  `SetQuota` validates shape only; callers enforce Organization scope and
  the #953 role matrix (Owner sets Admin, Owner/Admin set Member; Admin
  never touches Owner/Admin). Organization membership is authoritative;
  direct Workspace roles grant no quota-management authority. Rejoining
  never re-issues: rows and history survive membership changes.
- `Store.CheckQuotaInTx`: Organization-scope settlement-transaction
  enforcement (Task submit / Vision reserve). Personal scopes skip this
  check at the callers (no quota concept); a Personal account reaching it
  fails closed with `ErrPersonalQuotaUnsupported`. Runs after Quote and
  before Hold in the same transaction, short-circuits Organization Owners
  to unlimited, then locks the quota row, reads
  the same snapshot, fails closed with `ErrQuotaExhausted`. Nil checker
  at the call site means unlimited.
- `Store.CheckQuotaForQuote`: Organization-scope preflight read check for
  quote/readiness. Advisory only; the settlement check is the fence.
  Mirrors the Owner short-circuit and the Personal fail-closed.

## Invariants

- Every check receives the required authoritative Quote timestamp. Task
  persists that timestamp as `quoted_at`; Vision explicitly persists it
  as `created_at`. Waiting for the quota lock never changes the
  reservation time, and quote/readiness uses the same timestamp contract.
- `consumptionread` owns Task/Vision state, actual/refund joins and submit-time
  attribution behind its Workspace and Billing Account + User read seams.
  This package retains cap policy, period bounds, locks and errors.
- Pending and settled usage are aggregated in one SQL statement snapshot;
  an independent terminal Capture cannot disappear between split reads.
- Attribution locks to submit time (`task_billing_snapshots.quoted_at`,
  `ai_search_operations.created_at`, UTC) and to the real operator
  (`user_id` + `billing_account_id`). Terminal settlement never
  re-checks and never moves in-flight Holds across windows or pools.
- Actuals never exceed the pre-authorized Hold (the Hold is the cap).
  Partial success captures the actual and releases the remainder;
  failure/cancel releases everything; linked refunds correct the
  original window.
- Adjusting the limit or switching windows never clears usage;
  periodic windows reset without deleting history. Absent row reads as
  unlimited. Cross-account rows stay independent.
- Cross-account rows stay independent. Workspace Budget (#945, retired
  by #954) is no longer checked at the settlement boundary; this package
  implements no workspace math.
- First release excludes Workspace wallets, internal grants,
  cross-account balance moves, auto top-up, subscriptions, expiries,
  and payer switches.
