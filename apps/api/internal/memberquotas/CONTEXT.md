# MemberQuotas Context

Unified member defaults and role matrix (#953, ADR-0038) is live: new
Organization Members and Personal collaborators default to unlimited
(absent row reads as unlimited); invites never gate on a default (the
#947 default table, domain, API, and fail-closed gate are retired, no
dual implementation). Organization Owner is always unlimited via the
check short-circuit; Owner sets Admin, Owner/Admin set Member, Admin
never touches Owner/Admin (403). Personal collaboration (#952) aggregates
by (Personal Billing Account, User) with the holder as manager; Org scopes
aggregate by (Organization Billing Account, User) across all Org Workspaces.

English agent docs: this file records the member cross-workspace
consumption quota (#946) domain language. Human-facing product copy stays
in Simplified Chinese in the frontend locales.

## Package Map

- `quota.go`: pure period/validation vocabulary (`PeriodDaily`,
  `PeriodWeekly`, `PeriodMonthly`, `PeriodFixed`, `PeriodUnlimited`,
  `MaxLimitCredits`, `ValidateSet`, `IsUnlimited`, UTC day / Monday-start
  week / month bounds). No I/O.
- `policy.go`: Organization quota read/write role matrix behind the
  `OrganizationRoleReader` seam. `CanManage` consumes the Personal account
  holder fact; Billing Admin and Workspace Admin standing alone do not
  manage Personal quotas. `CanSet` checks manager authorization itself; callers
  do not need to invoke `CanManage` first. Both methods preserve role lookup
  errors so HTTP renders 503 and never writes on an unavailable role lookup.
  `NormalizeRead` owns absent-row and Organization Owner normalization,
  including read errors; HTTP only maps the result to its contract.
- `store.go`: postgres persistence plus the settlement check. The only
  writer of `billing_account_member_quotas`; delegates complete Used Credits
  reads to `consumptionread`. It never writes the Credits Ledger. Owner
  short-circuit (`isOrganizationOwner`) runs before the quota-row lock in
  both `CheckQuotaInTx` and `CheckQuotaForQuote`.

## Domain Language

| Term              | Meaning                                                                                                                                                                                    |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Member Quota      | One cumulative cap per (Billing Account, User) on settled + pending consumption in the current window, across every Workspace sharing the account. Not a wallet, grant, or reservation. |
| Period            | `daily` (UTC day), `weekly` (UTC Monday-start week), `monthly` (UTC month), `fixed` (lifetime cumulative), or `unlimited` (no cap, the default). No stacking, no rollover.            |
| Used Credits      | Settled net captures attributed by submit time minus linked refunds (each refund corrects its original capture's window) plus pending Holds, for one member across shared spaces.      |
| Remaining Credits | Capped limit minus used, clamped at zero; absent for unlimited.                                                                                                                           |
| Quota Exhausted   | A request that would exceed the member's remaining quota (`ErrQuotaExhausted`, wire `member_quota_exhausted` 403). Distinct from insufficiency (402) and restriction (403).    |

## Public Seams

- `Store.GetQuotaRow` / `Store.SetQuota` / `Store.ListQuotas` /
  `Store.UsageForPeriod`: handler reads and quota administration.
  `SetQuota` validates shape only; callers enforce the #953 role matrix
  (Org: Owner sets Admin, Owner/Admin set Member; Personal: holder manages
  collaborators). Rejoining never re-issues: rows and history survive
  membership changes.
- `Store.CheckQuotaInTx`: settlement-transaction enforcement (Task
  submit / Vision reserve). Runs after Quote and before Hold in
  the same transaction, short-circuits Organization Owners to unlimited,
  then locks the quota row, reads
  the same snapshot, fails closed with `ErrQuotaExhausted`. Nil checker
  at the call site means unlimited.
- `Store.CheckQuotaForQuote`: preflight read check for quote/readiness.
  Advisory only; the settlement check is the fence. Mirrors the Owner
  short-circuit.

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
