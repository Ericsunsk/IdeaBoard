# ADR-0032: Billing Account Integration Gate and Deferred Limits

## Context

Issues #915–#923 landed the shared Billing Account funds pool on one working
tree: stable resolution + financial permissions (#915, ADR-0024), Wallet
accounting + migration (#916, ADR-0025), Task settlement (#917, ADR-0026),
Vision Search settlement (#918, ADR-0027), Welcome/Reward beneficiary (#921,
ADR-0028), payment orders (#919, ADR-0029), refunds/disputes (#920, ADR-0030),
and Admin explicit accounts (#923, ADR-0031). Issue #924 is the single
integration gate: one funds model everywhere, continuity of money facts, and
all recovery paths — without shipping quota/budget enforcement that belongs
to a later batch (#945 workspace budgets, #946 member quotas, #947 invite
defaults, #948 handover).

## Decision

1. One funds identity, one settlement path. The Billing Account (`ba_`) holds
   the unified balance; every Command addresses exactly one account with the
   real operator on `Metadata.Actor`. Generic commands run
   `execute_credit_command_v3`; external-payment commands run the typed v2
   account routines. The superseded user-keyed routines are dropped; no
   owner-wallet fallback, alias route, or dual settlement remains in code.
   History rows keep their amounts and postings untouched — only additive
   attribution columns were backfilled.
2. Settlement facts are row-pinned. Task/Vision/Payment rows persist their
   payer at submit/checkout time; terminal Capture/Release, retries,
   reconciliation, and refunds converge on the stored payer and never
   re-resolve from live ownership or membership. Concurrency is fenced by
   guard-lock-first ordering (L1 before the L0 idempotency claim), account-row
   locking in stable order, and same-key/same-hash replay with same-key/
   different-hash conflict.
3. Scope deletion never deletes money. Workspace hard delete removes boards,
   tasks, links, members, and the per-workspace `billing_accounts` row;
   Wallets, projections, Ledger, snapshots, and operation rows stay readable
   under their `billing_account_id` (same precedent as snapshot `task_id`
   carrying no task FK). Verified at SQL level and by `DeleteWorkspace` tests.
4. Limits are hooks, not enforcement — explicitly PENDING. Criterion-3
   quota/budget co-enforcement, limit-change-without-reset, cross-cycle, and
   linked-consumption refunds, plus criterion-1 invite/handover links, are
   verified only to the transaction-boundary and permission-hook level their
   ADRs promise (ADR-0024 §6, ADR-0026 §4). They MUST NOT be reported as
   effective until #945–#948 land; no quota math was added to pass this gate.
5. No new acceptance-test framework. Browser/manual acceptance stays with the
   user; this gate reuses `go test`, vitest suites, `schemarelease`, Atlas
   validation, sqlc/Orval drift, and contract scripts. Cutover/stop-write/
   recovery/rollback procedure lives in
   [billing-account-cutover.md](../runbooks/billing-account-cutover.md);
   image rollback never implies database restore.

## Consequences

- Canonical docs record only the final structure (DESIGN.md, backend
  CONTEXT.md files, transaction-lock-ordering.md); product rules previously
  marked needs-info are confirmed in ADR-0024–ADR-0031, none left open.
- Migration releases stay append-only with Fresh-DB clean breaks; the
  `destructive: approved` + `ATLAS_ALLOW_DESTRUCTIVE=1` path requires explicit
  review per release.
- Follow-up batches (#945–#948) build quota/budget checks inside the
  settlement transaction this gate pinned (after stable resolution, before
  Hold, on the same snapshot).

## Status

Accepted
