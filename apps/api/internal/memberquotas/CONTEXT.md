# MemberQuotas Context

English agent docs: this file records the member cross-workspace
consumption quota (#946) domain language. Human-facing product copy stays
in Simplified Chinese in the frontend locales.

## Package Map

- `quota.go`: pure period/validation vocabulary (`PeriodDaily`,
  `PeriodWeekly`, `PeriodMonthly`, `PeriodFixed`, `PeriodUnlimited`,
  `MaxLimitCredits`, `ValidateSet`, `IsUnlimited`, UTC day / Monday-start
  week / month bounds). No I/O.
- `store.go`: postgres persistence plus the settlement check. The only
  writer of `billing_account_member_quotas`; delegates complete Used Credits
  reads to `consumptionread`. It never writes the Credits Ledger.

## Domain Language

| Term              | Meaning                                                                                                                                                                                    |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Member Quota      | One cumulative cap per (Billing Account, User) on settled + pending consumption in the current window, across every Workspace sharing the account. Not a wallet, grant, or reservation. |
| Period            | `daily` (UTC day), `weekly` (UTC Monday-start week), `monthly` (UTC month), `fixed` (lifetime cumulative), or `unlimited` (no cap, the default). No stacking, no rollover.            |
| Used Credits      | Settled net captures attributed by submit time minus linked refunds (each refund corrects its original capture's window) plus pending Holds, for one member across shared spaces.      |
| Remaining Credits | Capped limit minus used, clamped at zero; absent for unlimited.                                                                                                                           |
| Quota Exhausted   | A request that would exceed the member's remaining quota (`ErrQuotaExhausted`, wire `member_quota_exhausted` 403). Distinct from insufficiency (402), budget (403), restriction.    |

## Public Seams

- `Store.GetQuotaRow` / `Store.SetQuota` / `Store.ListQuotas` /
  `Store.UsageForPeriod`: handler reads and quota administration.
  `SetQuota` validates shape only; callers enforce Billing Owner/Admin.
  Rejoining never re-issues: rows and history survive membership
  changes.
- `Store.GetDefaultPolicy` / `Store.SetDefaultPolicy`: account default
  new-member policy
  (#947). Absent default means NOT CONFIGURED (fail closed, never
  implicit unlimited); only Billing Owner/Admin writes. First join of
  the account scope seeds the member row from the default inside the
  tenancy membership transaction
  (`INSERT ... SELECT ... ON CONFLICT DO NOTHING`); later joins keep
  the existing row; default changes never touch existing rows.
- `Store.CheckQuotaInTx`: settlement-transaction enforcement (Task
  submit / Vision reserve). Runs after the workspace-budget check in
  the same transaction (fixed lock order), locks the quota row, reads
  the same snapshot, fails closed with `ErrQuotaExhausted`. Nil checker
  at the call site means unlimited.
- `Store.CheckQuotaForQuote`: preflight read check for quote/readiness.
  Advisory only; the settlement check is the fence.

## Invariants

- Every check receives the required authoritative Quote timestamp. Task
  persists that timestamp as `quoted_at`; Vision explicitly persists it
  as `created_at`. Waiting for budget/quota locks never changes the
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
- The workspace budget (#945) stays a second, independently effective
  check at the same boundary; this package implements no workspace math.
- First release excludes Workspace wallets, internal grants,
  cross-account balance moves, auto top-up, subscriptions, expiries,
  and payer switches.
