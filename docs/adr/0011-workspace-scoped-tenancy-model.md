# ADR-0011: Workspace-Scoped Tenancy Model

## Context

Phase 3b (#809) removed the legacy user-scoped data access (tasks keyed by
`user_id`, library/media keyed by owner) and Phase 4 (#818–#821) restored
each module on workspace-scoped tables (`tasks`, `library_folders`,
`library_assets`, `upload_sessions`, `media_assets`, `task_outputs` all carry
`workspace_id`; see `DESIGN.md` §2 and the Backend Context Map). This ADR
records the tenancy rules every restored module follows so future changes
stay convergent.

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

Accepted
