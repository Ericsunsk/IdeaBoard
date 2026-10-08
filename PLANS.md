# ExecPlans

## Archived Image Catalog Structure

- Goal: concentrate existing Unsplash/Pexels knowledge and supplier HTTP handling.
- Scope: shared frontend provider vocabulary consumed by Search, Library and Admin;
  separate backend supplier adapters from shared HTTP handling; remove duplicate
  provider guards and unused search constants. No new supplier, UI or schema.
- Verification: existing provider IDs, errors, ordering, attribution, cursor
  encoding, URL allowlists and request behavior remain compatible; focused tests,
  frontend type/build/lint and backend build/tests pass.
- Rollback: revert only this refactor's working-tree changes; no data migration.

## Archived Workspace Task Restoration ExecPlans

> Status (2026-10-08): **ARCHIVED**. The plans below describe completed work.
> Issue acceptance comments and git history are the source of truth; do not use
> this file as an active queue. New work needs a new plan or an issue-specific
> execution note.

## Archived 8xx Batch ExecPlan (#886–#897)

This scheduler plan is archived. Issues #886–#897 were completed, verified, and
committed; the former working-tree-only constraint and lane sequencing are no
longer active. The issue comments and git history are the source of truth for
their acceptance evidence and commits. The detailed lane checklist was removed
because it duplicated the issue tracker and could imply that this work is still
pending.

## Archived Issue #886 ExecPlan: Payment Refund Persistence Boundary

- Goal: persist the admin refund reservation, `PurchaseRefundHold`, and phase
  audit before any Stripe I/O; make provider result convergence observe the
  committed refund and reuse its durable identity after retries.
- Scope: isolate the refund reservation and post-provider settlement writes
  from the Control guard's still-open transaction; preserve pending recovery,
  existing Credits commands, and the Stripe idempotency key derived from the
  durable refund ID; add focused production-path tests with a fake gateway.
- Success criteria: no Stripe call occurs before the reservation commit; a
  provider result can be recorded and settled from a fresh database connection;
  commit/audit failures leave a recoverable pending fact; retries reuse the same
  refund ID and Stripe idempotency key without a second cash refund.
- Rollback: revert the issue commit. No schema changes are expected.

## Archived Support Channels ExecPlan

- Goal: expose operator-configured external support channels from the account menu.
- Scope: typed runtime settings in the existing system config store; audited
  Control Plane reads/writes; anonymous Public API read; General settings editor;
  channel menu with a third-level QR Popover and external QR link.
- Success criteria: disabled channels never appear publicly, empty configuration
  hides Support, safe HTTP(S) links generate QR locally, channel order persists,
  and mouse/keyboard/touch interactions remain usable within the viewport.
- Verification: settings persistence and ambient rollback on isolated PostgreSQL
  schemas, route/control contract tests, frontend focused tests, browser geometry,
  type/build/lint and generated API drift.
- Rollback: revert the feature changes; no schema migration is required. The
  optional `support_channels` system config key can remain inert.

---

## Archived Workspace Task Restoration (#825–#830)

The workspace task production path, the #809 hard cut, and the runtime-grant
audit were completed and committed. The original detailed scope is retained
below as a historical acceptance summary.

### Goal

Complete the workspace task production path for issues #825 through #830:
submit, execute, render, settle, and materialize while preserving the #809
hard cut without weakening release guards.

### Scope

- #825: compile and persist a durable workflow execution plan for v1 submits.
- #826: persist task billing snapshots and settlement facts in the existing
  settlement transaction.
- #827: wire the production result fetcher and derivative renderer.
- #828: wire the library saver and make materialization billing/link state
  converge correctly on errors.
- #829: preserve the intentional #809 hard cut. No legacy routes, handlers,
  adapters, aliases, or dual-read/dual-write compatibility paths are kept.
- #830: retain the grant audit as documentation only.
- Restore the Board state contract guard removed by this diff.
- Cleanup: remove the unreferenced pre-canonical `TaskIntake` readiness,
  media-preparation, and submit-validation path. Keep the canonical workspace
  compiler, Admin monitor DTOs, and event hubs that remain production-owned.
- Routing: construct one `RoutingResolver` at the composition root and share it
  with worker settlement, task submit/readiness, task quotes, and billing
  quotes. Availability checks and routing snapshot persistence fail closed.
- Consistency: read model pools from one database transaction snapshot and keep
  task billing, routing snapshot, task insert, and River enqueue in the same
  settlement transaction.

### Success Criteria

- A submitted task carries a valid durable execution plan and can reach the
  provider gateway.
- Task state, Credits commands, and task billing facts commit atomically.
- Remote provider outputs become media assets and materialize into Quick Saves.
- The #809 hard cut is documented and tested.
- Targeted Go and Web checks pass with no generated-code hand edits.

### Risks And Rollback

- Transactional billing changes affect financial facts; all writes use the
  existing pgx transaction and Credits command seams.
- Remote output fetching is constrained to trusted provider origins and size
  limits. A fetch or render failure remains terminal and releases task billing.
- Each issue is committed independently, so rollback is a commit revert.
- Legacy cleanup is a hard cut: deleted symbols and files have no compatibility
  aliases, dual reads, dual writes, or fail-open fallbacks. Rollback is the
  commit revert before deployment.
