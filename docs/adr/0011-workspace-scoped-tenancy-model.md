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
4. At the time this ADR was accepted, task submission settled Quote → Hold →
   Insert → Enqueue with compensating rollback (Release + Delete); a single
   shared PostgreSQL transaction across all four participants was the documented
   follow-up. The current atomicity rule is recorded in ADR-0012 and in this
   ADR's Status section below.
5. Library assets store provider URL + source metadata only (never bytes);
   media owns bytes. Task outputs reference media by `asset_…` public id or
   by the explicit remote-output contract (URL + checksum, no storage internals).

## Consequences

Convergent 404 behavior and per-module seams make workspace isolation testable
per module and composable across modules (#821 proves four cross-module
scenarios). Task submission now performs the billing hold, task insert, routing
snapshot pin, and River enqueue in one PostgreSQL transaction. The workspace
worker consumes the pinned snapshot, and the derivative renderer runs in the
workspace execution path. Library materialization remains intentionally
internal-only and returns the documented 503 when its durable service is not
available; it does not create a partial job or silently fall back to a legacy
path.

## Status

Accepted (updated 2026-10-08). Decisions 1–3 and 5 remain the current tenancy
contract. Decision 4 is superseded by ADR-0012 rule 2a and the production
settlement implementation:

- `apps/api/internal/tasking/workspacetasks/settle_tx.go` runs Claim → Resolve →
  Quote → Hold → Insert → RoutingSnapshot pin → Enqueue on one `pgx.Tx`.
- `TxEnqueuer.EnqueueInTx` commits the `river_job` row with the business task
  row. Missing transaction wiring fails closed with `ErrSettlementUnavailable`.
- `workspace_task_execute` is consumed by the registered workspace worker, and
  the derivative renderer persists workspace-scoped media assets.
- The compensating path remains only in test fixtures; it is not a production
  fallback.

The still-valid materialization residual is `ErrV1MaterializationUnavailable`:
the internal-only materialization service returns 503 rather than partially
persisting a job. Resolving that residual requires a separate contract change.

Current contract for the atomicity rule: `DESIGN.md` §3 "Tasking 的事实源是
PostgreSQL + River" and `apps/api/internal/tasking/CONTEXT.md`. ADR-0012 rule 2a
restates the same ownership ("Task lifecycle owns idempotency, the atomic
hold/task/River enqueue transaction") and its rule 9 requires the
`RoutingSnapshot` to be pinned inside that same submit transaction.

ADR-0012 supersedes Decision 4 only. It is a routing ADR, not a tenancy ADR,
so this record stays Accepted rather than Superseded.
