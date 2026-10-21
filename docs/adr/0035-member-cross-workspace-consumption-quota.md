# ADR-0035: Member Cross-Workspace Consumption Quota

## Context

Issue #946 requires a member-level spending cap over the shared funds
pool: a funding manager sets per-member quotas on one Billing Account,
and that member's paid generations plus Vision Search accumulate across
every Workspace sharing the account while the member sees their own
usage and remaining quota. #915 landed stable payer resolution with
financial Owner/Admin roles, #916 moved Wallets under Billing Accounts,
#917/#918 settled Task/Vision on the stored payer with the real operator
persisted on every snapshot, #944 made creation-time payers explicit,
and #945 enforced the Workspace shared cap with the settlement pattern
this ticket copies (Quote → check with row lock → Hold in one
transaction, usage derived from submit-time-attributed facts net of
linked refunds). Without member quotas, one member can burn a shared
pool that sibling spaces depend on, and there is no per-operator
visibility. Product rules confirmed for this ticket: five quota words
(unlimited default, daily / weekly / monthly UTC windows plus a fixed
lifetime cap, no rollover of unused periodic quota); account-fixed UTC
with Monday-start weeks; full-window caps for mid-period joiners (no
proration); limit/mode changes keep accrued usage; submit-time window
attribution locked at Quote/Hold with pre-submit re-estimation blocking
overruns; linked consumption refunds correct the original window while
cross-window refunds return to the account balance without widening the
current window. Ledger and Task/River atomic-transaction contracts stay
intact. Workspace wallets, internal grants, balance transfers, auto
top-up, subscriptions, expiries, and payer switches stay out of scope.

## Decision

1. One row per (Billing Account, User) (`billing_account_member_quotas`:
   daily / weekly / monthly / fixed / unlimited; absent means
   unlimited). Only Billing Owner/Admin of the resolved account writes
   (space-only Workspace admins get `billing_account_forbidden`, so a
   Workspace admin cannot raise a pool-granted quota); managers list
   every row while members read only their own row with live usage and
   remaining. API and UI permissions agree off the same billing-account
   read. Cross-account rows stay independent; new credentials, new
   spaces, and rejoining never re-issue (rows and immutable settlement
   history survive membership changes; only user deletion cascades).
2. The check joins the settlement transaction after the workspace-budget
   check (fixed lock order: budget row first, quota row second) and
   before Hold, locking the quota row `FOR UPDATE` and reading the same
   snapshot. Concurrent submits by one member serialize on that row; the
   loser observes the winner's committed Hold and fails closed, so
   concurrency and retries cannot bypass. Enqueue failure still rolls
   the whole unit back (no Hold, no task, no job, no quota residue).
   Terminal Capture/Release needs no re-check: in-flight Holds converge
   on their submit-time payer and window facts and never switch pools.
   With a workspace budget present, both constraints stay independently
   effective.
3. Usage is derived, never stored: settled net captures attributed by
   submit time (task `quoted_at` / vision `created_at`) minus linked
   refunds via `credit_charges.refunded_amount` (each refund corrects
   its original capture's window, so a cross-window refund never grants
   current quota), plus pending Holds, across every Workspace sharing
   the account. Success converges usage down to the actual capture
   (partial captures release the remainder); failure/cancel releases to
   zero. Limit and window changes only move the cap/window; history rows
   are retained.
4. Four funding signals stay distinct end to end (quote, submit, and UI
   copy): balance shortfall is 402 `task_insufficient_credits`,
   workspace exhaustion is 403 `workspace_budget_exhausted`, member
   exhaustion is 403 `member_quota_exhausted`, and other funds
   restrictions stay 403 `billing_wallet_restricted`.
5. Management and self reads ship together: `GET
   /api/v1/workspaces/{id}/member-quotas` (managers),
   `GET .../member-quotas/self` (every member),
   `PUT .../member-quotas/{user_id}` (managers, typed SQL via sqlc,
   runtime least privilege, Orval contract), a workspace-settings quota
   section with four-language copy, and focused tests pinning
   cross-workspace totals, Task/Search mixing, concurrency fencing,
   limit changes, rejoining, window resets, failure recovery, and
   refunds.

## Consequences

- Task and Vision share one member quota through one quota Store; the
  workspace budget (#945) stays a second, independently effective check
  at the same boundary.
- No sub-wallets, no transfers, no reservations, no auto top-up; no
  legacy/alias/compat paths; historical funds facts are never deleted.
- Follow-up tickets (final integration gate) build on these caps and
  boundaries; intermediate steps are not released separately.

## Status

Accepted
