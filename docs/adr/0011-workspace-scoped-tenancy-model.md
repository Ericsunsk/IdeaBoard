# ADR-0011: Workspace-Scoped Tenancy Model

## Context

Phase 3b (#809) removed the legacy user-scoped data access (tasks keyed by
`user_id`, library/media keyed by owner) and Phase 4 (#818–#821) restored
each module on workspace-scoped tables (`tasks`, `library_folders`,
`library_assets`, `upload_sessions`, and `media_assets` carry `workspace_id`;
`task_outputs` has no tenancy column of its own and inherits tenancy through
its `task_id` foreign key; see `DESIGN.md` §2 and the Backend Context Map).
This ADR records the tenancy rules every restored module follows so future
changes stay convergent.

## Decision

1. `workspaces.public_id` (`ws_…`) is the only tenancy key that crosses the
   API boundary. Internal numeric ids never appear in URLs, payloads, or
   events (`users.public_id` stays the public identity; `users.id` is Join-only).
2. Membership gating lives at the HTTP handler layer
   (`RequireWorkspaceCapability`): hidden workspaces and cross-workspace ids
   converge to 404 (never 403, never a distinct status), following Baseline 4
   anti-enumeration. Stores additionally key every statement by resolved
   `workspace_id`, so a missed gate still cannot leak a cross-workspace row.
3. Each module owns its workspace-scoped persistence behind a narrow seam
   (`WorkspaceLibraryStore`, `WorkspaceMediaStore`,
   `workspacetasks.WorkspaceTaskStore`); repository methods take
   `context.Context` first and use the `dbWithContext` pattern.
4. Task submission settles Quote → Hold → Insert → Enqueue with compensating
   rollback (Release + Delete); a single shared PostgreSQL transaction across
   all four participants is the documented follow-up, not today's invariant.
5. Library assets store provider URL + source metadata only (never bytes);
   media owns bytes. Task outputs reference media by `asset_…` public id or
   by the explicit remote-output contract (URL + checksum, no storage internals).

## Consequences

Convergent 404 behavior and per-module seams make workspace isolation testable
per module and composable across modules (#821 proves four cross-module
scenarios). The compensating (non-atomic) settlement and the unwired worker
(`workspace_task_execute` enqueued but not yet consumed), derivative renderer,
and library materialization (503) are accepted Phase-4b residuals, not silent
gaps: each fails closed with a documented code.

## Status

Accepted — Decision 4 and three of the four residuals in Consequences are
superseded; see below

Decisions 1–3 and 5 remain the current tenancy contract. The four residuals
named in Consequences have since split; only the last one still holds:

- **Compensating (non-atomic) settlement — resolved.** Decision 4's "a single
  shared PostgreSQL transaction across all four participants is the documented
  follow-up, not today's invariant" no longer holds. Settlement is now one
  transaction: `apps/api/internal/tasking/workspacetasks/settle_tx.go`
  `createInTx` runs Claim → Resolve → Quote → Hold → Insert → RoutingSnapshot
  pin → Enqueue on a single `pgx.Tx`, and `TxEnqueuer.EnqueueInTx` commits the
  `river_job` row with the business task row. Every participant is discovered
  from that one transaction or the store fails closed (`settlementSeams` →
  `ErrSettlementUnavailable`); a missing transaction is a wiring error, not a
  fallback (`newPoolRoutingSnapshotSaver` in
  `apps/api/internal/runtime/composition/http_wiring.go`). The compensating
  path survives only as a test fixture (`AllowNonTransactionalTaskSettlement` →
  `workspacetasks.NewTestServiceForTests`), never in production.
- **Unwired worker — resolved.** `river.AddWorker(workers, workspaceWorker)` in
  `apps/api/internal/runtime/composition/task_wiring.go:355`;
  `workspace_task_execute` is now consumed.
- **Derivative renderer — resolved.**
  `apps/api/internal/tasking/application/runtime/workspace_task_renderer.go`
  (#827, Phase 4b-03) runs between provider Generate and settler Complete and
  renders outputs into workspace-scoped media assets.
- **Library materialization (503) — still current.**
  `apps/api/internal/library/domain/errors.go`
  `ErrV1MaterializationUnavailable` still fails closed; the postgres Service
  still renders 503 rather than half-persisting jobs.

Current contract for the atomicity rule: `DESIGN.md` §3 "Tasking 的事实源是
PostgreSQL + River" and `apps/api/internal/tasking/CONTEXT.md`. ADR-0012 rule 2a
restates the same ownership ("Task lifecycle owns idempotency, the atomic
hold/task/River enqueue transaction") and its rule 9 requires the
`RoutingSnapshot` to be pinned inside that same submit transaction.

ADR-0012 supersedes Decision 4 only. It is a routing ADR, not a tenancy ADR,
so this record stays Accepted rather than Superseded.
