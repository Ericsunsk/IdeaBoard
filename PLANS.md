# Workspace Task Restoration ExecPlan

## 8xx Batch ExecPlan (#886–#897, scheduler-owned)

- Goal: complete all `ready-for-agent` 8xx issues per acceptance criteria, working tree only, no commit.
- Lanes (parallel, file-scoped): A=#886+#897 payments (scheduler, sequential: #897 after #886); B=#888→#895→#896 auth chain (sequential); C=#887 deletion; D=#889 task events; E=#891 control idempotency; F=#892 billing/brand; G=#890+#894 board web (sequential); H=#893 canvas web.
- Success criteria: each issue's acceptance boxes verifiable; smallest relevant checks green (`apps/api go test`, `apps/web npm run test:run`, `git diff --check`); no generated-file hand edits; no cross-lane file edits.
- Rollback: no commit is made; revert working-tree files per lane if needed.
- Constraints: B chain order (#888 first); A order (#886 first); lanes must not edit `PLANS.md` (scheduler owns), must not touch other lanes' modules.
- Sequencing note: #889 fix (ambient post-commit publish) approved as option A (post-commit callback on ambient carrier, flush in guard `runAtomic` after commit), but implement only after lane E (#891) finishes since both touch `contract_control_guard.go`.
- Lane status: ALL 12 DONE and VERIFIED (A #886+#897 scheduler; B #888→#895→#896; C #887; D #889 option A on E; E #891; F #892; G #890+#894; H #893). Full backend green (parallel full run hit local-PG shared-memory limits only; all 18 affected packages re-ran serially green, zero assertion failures). gofmt/build/diff-check/sqlc-regen clean. No commit per instruction.
- Pre-existing (not from lanes): schemarelease drift `idx_payment_webhook_events_processing_lease` in migration `20261004000002` but missing from fragments; lanes touched no payments schema files. Fix separately, out of 8xx scope.
- Redundancy audit pass (scheduler, verified): R1 reclaim helper in idempotency Claim; R2 errNotWired + dead db check; R5 WithUserLock via Transact; R6 errorMessage; R7 queryIDs; R8 database.IsNotFound + PgError code (dropped privates); R9 withCurrentIntent; R10 supersedeWithRepair; R11 repairOpFor param drop; R12 versionStillCurrent via checkIntentCurrent; R13 reconciliation reuses authsync.UserSyncState + IsDemotionTarget; R14 billing comment trim; R15 board scope-key helper. Kept deliberately: BumpVersion (deadline-preserving) vs WithDeadline, beginDirectTx (external/dispute), L1+durable dual paths. All re-verified green, no commit.

## Issue #886 ExecPlan: Payment Refund Persistence Boundary

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

## Support Channels ExecPlan

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

## Goal

Complete the workspace task production path for issues #825 through #830:
submit, execute, render, settle, and materialize while preserving the #809
hard cut without weakening release guards.

## Scope

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

## Success Criteria

- A submitted task carries a valid durable execution plan and can reach the
  provider gateway.
- Task state, Credits commands, and task billing facts commit atomically.
- Remote provider outputs become media assets and materialize into Quick Saves.
- The #809 hard cut is documented and tested.
- Targeted Go and Web checks pass with no generated-code hand edits.

## Risks And Rollback

- Transactional billing changes affect financial facts; all writes use the
  existing pgx transaction and Credits command seams.
- Remote output fetching is constrained to trusted provider origins and size
  limits. A fetch or render failure remains terminal and releases task billing.
- Each issue is committed independently, so rollback is a commit revert.
- Legacy cleanup is a hard cut: deleted symbols and files have no compatibility
  aliases, dual reads, dual writes, or fail-open fallbacks. Rollback is the
  commit revert before deployment.
