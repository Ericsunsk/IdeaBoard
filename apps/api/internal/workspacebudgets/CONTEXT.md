# WorkspaceBudgets Context

English agent docs: this file records the Workspace shared consumption
budget (#945) domain language. Human-facing product copy stays in
Simplified Chinese in the frontend locales.

## Package Map

- `budget.go`: pure period/validation vocabulary (`PeriodMonthly`,
  `PeriodUnlimited`, `MaxLimitCredits`, `ValidateSet`, `IsUnlimited`,
  UTC month bounds). No I/O.
- `store.go`: postgres persistence plus the settlement check. The only
  writer of `workspace_budgets`; delegates complete Used Credits
  reads to `consumptionread`. It never writes the Credits Ledger.

## Domain Language

| Term              | Meaning                                                                                                                                                          |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Workspace Budget  | One cumulative cap per Workspace on settled + pending consumption in the current period. Not a wallet, grant, or reservation.                                     |
| Period            | `monthly` (UTC calendar month) or `unlimited` (no cap, the default). No stacking.                                                                                |
| Used Credits      | Settled net captures attributed by submit time minus linked refunds (each refund corrects its original capture's period) plus pending Holds, across all members. |
| Remaining Credits | Monthly limit minus used, clamped at zero; absent for unlimited.                                                                                                 |
| Budget Exhausted  | A request that would exceed the remaining cap (`ErrBudgetExhausted`, wire `workspace_budget_exhausted` 403). Distinct from insufficiency (402) and restriction. |

## Public Seams

- `Store.GetBudgetRow` / `Store.SetBudget` / `Store.UsageForPeriod`:
  handler reads and budget administration. `SetBudget` validates shape
  only; callers enforce Billing Owner/Admin.
- `Store.CheckBudgetInTx` / `Store.CheckBudgetByInternalIDInTx`:
  settlement-transaction enforcement (Task submit / Vision reserve).
  Locks the budget row, reads the same snapshot, fails closed with
  `ErrBudgetExhausted`. Nil checker at the call site means unlimited.
- `Store.CheckBudgetForQuote`: preflight read check for quote/readiness.
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
  `ai_search_operations.created_at`, UTC). Terminal settlement never
  re-checks and never moves in-flight Holds across periods or pools.
- Actuals never exceed the pre-authorized Hold (the Hold is the cap).
  Partial success captures the actual and releases the remainder;
  failure/cancel releases everything; linked refunds correct the
  original period.
- Adjusting the limit never clears usage; UTC-month resets never delete
  history. Absent row reads as unlimited.
- Member quotas (#946) join as a second, independently effective check
  at the same boundary (fixed lock order: budget row first, quota row
  second); this package implements no per-member math.
- First release excludes Workspace wallets, internal grants,
  cross-account balance moves, auto top-up, subscriptions, expiries,
  and payer switches.
