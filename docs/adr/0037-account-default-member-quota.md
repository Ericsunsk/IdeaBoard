# ADR-0037: Account Default New-Member Consumption Quota and Invite Gate

## Context

Issue #947 requires invite-time reuse of the account default: a funding
manager sets one default new-member quota on a Billing Account, Workspace
inviters reuse it, and first joins seed the per-member row while later
joins preserve it. #915 landed stable payer resolution with financial
Owner/Admin roles, #916 moved Wallets under Billing Accounts, #944 made
creation-time payers explicit, #945 enforced the Workspace shared cap,
and #946 enforced per-member caps (PK account+user, five periods,
SetQuota, usage-preserving adjustments, rejoin retention) that this
ticket reuses for initialization. Without a default, invites either
silently mint unlimited members (burning the shared pool) or force every
inviter to pick a cap (letting a space-only admin raise a pool-granted
quota). Product rules confirmed for this ticket: fail closed when
unconfigured (block the invite, guide to set the default; never implicit
unlimited); three monthly presets (small 200 / medium 1000 / large 2000
Ideas) plus the five #946 words (unlimited / daily / weekly / monthly /
fixed); default changes affect only later joins (existing rows tuned
per-member with usage kept); leaving all spaces retains the quota row and
readable history while freezing eligibility (rejoin restores, never
re-issues); inviters reuse only (never raise). Ledger and Task/River
atomic-transaction contracts stay intact. Workspace wallets, internal
grants, cross-account moves, auto top-up, subscriptions, expiries, and
payer switches stay out of scope.

## Decision

1. One row per Billing Account (`billing_account_default_member_quotas`:
   daily / weekly / monthly / fixed / unlimited; absent means NOT
   CONFIGURED, never unlimited). Only Billing Owner/Admin of the resolved
   account writes (space-only Workspace admins get
   `billing_account_forbidden`, so a Workspace admin cannot raise the
   pool default); any member reads, absent reads as 409
   `default_member_quota_not_configured`. API and UI permissions agree
   off the same billing-account read. Cross-account defaults stay
   independent; default changes never touch existing per-member rows.
2. Invites reuse the default atomically: the membership insert, stable
   payer resolution, default fail-closed check, and quota seed commit in
   one transaction (postgres `AddWorkspaceMember` locks the workspace
   memberships, resolves the payer in order, checks the default, then
   `INSERT ... SELECT ... ON CONFLICT DO NOTHING`). The inviter passes
   no quota params, so the default cannot be raised at invite time.
   Duplicates keep the existing role and quota; shared-account second
   joins preserve the first row; rejoins restore eligibility without
   re-issuing. Concurrent joins collapse on the workspace lock plus the
   quota unique index.
3. Removal freezes but never deletes: `RemoveWorkspaceMember` leaves the
   quota row, settlement history, bills, and refund links readable under
   the account; the member simply holds no workspace to submit from.
   Rejoining restores the same cap and accrued usage.
4. Board editing stays the generation gate: formal members with Board
   edit permission generate within quota, Viewers cannot generate; no
   per-person canSpend switch is added and external-collaborator spend
   authorization is not expanded.
5. Management, invite, and self reads ship together: `GET/PUT
   /api/v1/workspaces/{id}/default-member-quota` (typed SQL via sqlc,
   runtime least privilege, Orval contract), the invite gate on `POST
   .../members` (409 when unconfigured), a workspace-settings default
   section with four-language copy and monthly presets, and focused
   tests pinning unconfigured/duplicate/concurrent/multi-space/remove/
   rejoin paths.

## Consequences

- New members enter the shared pool under an explicit, manager-owned
  cap; existing members keep their tuned caps and usage when the
  default moves.
- No sub-wallets, no transfers, no reservations, no auto top-up; no
  legacy/alias/compat paths; historical funds facts are never deleted.
- Follow-up work (final integration gate) builds on this default and
  the #945/#946 caps; intermediate steps are not released separately.

## Status

Accepted
