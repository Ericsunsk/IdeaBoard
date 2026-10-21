# ADR-0034: Workspace Shared Consumption Budget

## Context

Issue #945 requires a Workspace-level spending cap over the shared funds
pool: a funding manager sets one total budget per Workspace, and that
space's paid generations plus Vision Search jointly consume it while every
member sees authorized usage and block reasons. #915 landed stable payer
resolution with financial Owner/Admin roles, #916 moved Wallets under
Billing Accounts, #917/#918 settled Task/Vision on the stored payer with
a marked-but-unbuilt limit boundary, and #944 made creation-time payers
explicit. Without enforcement, one member can burn a shared pool that
other workspaces (or businesses on sibling accounts) depend on. Product
rules confirmed for this ticket: monthly UTC-calendar periods plus
unlimited (default unlimited, no stacking); submit-time period
attribution locked at Quote/Hold; the Hold is the cap (actuals never
exceed the pre-authorization, over-runs are blocked before submit, never
settled over); the budget is a cumulative cap, never a wallet, grant, or
reservation — limit changes and period resets neither top up nor erase
accrued consumption. Ledger and Task/River atomic-transaction contracts
stay intact; member quotas (#946) are a sibling ticket and stay a
separate boundary; Workspace wallets, internal grants, balance transfers,
auto top-up, subscriptions, expiries, and payer switches stay out of
scope.

## Decision

1. One row per Workspace (`workspace_budgets`: monthly limit or
   unlimited; absent means unlimited). Only Billing Owner/Admin of the
   resolved account writes (space-only Workspace admins get
   `billing_account_forbidden`, so a Workspace admin cannot raise a
   pool-granted budget); every member reads cap, live usage, and
   remaining. API and UI permissions agree off the same billing-account
   read.
2. The check joins the settlement transaction after Quote (amount known)
   and before Hold, locking the budget row `FOR UPDATE` and reading the
   same snapshot. Concurrent submits serialize on that row; the loser
   observes the winner's committed Hold and fails closed, so concurrency
   and retries cannot bypass. Enqueue failure still rolls the whole unit
   back (no Hold, no task, no job, no budget residue). Terminal
   Capture/Release needs no re-check: in-flight Holds converge on their
   submit-time payer and period facts and never switch pools.
3. Usage is derived, never stored: settled net captures attributed by
   submit time (task `quoted_at` / vision `created_at`) minus linked
   refunds via `credit_charges.refunded_amount` (each refund corrects
   its original capture's period, so a cross-period refund never grants
   current quota), plus pending Holds. Success converges usage down to
   the actual capture (partial captures release the remainder);
   failure/cancel releases to zero. Limit changes and UTC-month resets
   only move the cap/window; history rows are retained.
4. Three funding signals stay distinct end to end (quote, submit, and
   UI copy): balance shortfall is 402 `task_insufficient_credits`,
   budget exhaustion is 403 `workspace_budget_exhausted`, and other
   funds restrictions stay 403 `billing_wallet_restricted`.
5. Management and usage ship together: `GET/PUT
   /api/v1/workspaces/{id}/budget` (typed SQL via sqlc, runtime least
   privilege, Orval contract), a workspace-settings budget section with
   four-language copy, and focused tests pinning cross-member totals,
   Task/Search mixing, concurrency fencing, boundary resets, limit
   changes, refunds, and failure recovery.

## Consequences

- Task and Vision share one workspace cap through one budget Store;
   member quotas (#946) join as a second, independently effective check
   at the same boundary when they land.
- No sub-wallets, no transfers, no reservations, no auto top-up; no
   legacy/alias/compat paths; historical funds facts are never deleted.
- Follow-up tickets (#946 member quotas, final integration gate) build
   on this cap and boundary; intermediate steps are not released
   separately.

## Status

Accepted
