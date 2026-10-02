---
status: deprecated
owner: Board
decision_date: 2026-09-11
supersedes: none
---

# Board saves use optimistic revision concurrency

Board writes must include the revision observed by the client as `base_revision`, and a successful write returns the persisted `new_revision`. The backend compares and advances the revision atomically with the update, returning HTTP 409 when the persisted revision has changed so the client can reconcile instead of silently applying last-write-wins. New Boards use base revision `0`; existing callers must hydrate or retain the latest server revision before saving.

This intentionally extends the protected Board API contract. Persisted Board schema, serialized state, media references, storage keys, and public routes remain unchanged. Review this decision if Board writes move away from PostgreSQL compare-and-swap semantics or if offline multi-writer reconciliation needs a richer conflict model; any replacement must supersede this ADR explicitly.

## Current state

The invariant this record protects is still in force; the wire encoding is not.

Still current: Board document writes use optimistic revision concurrency. The
persisted revision is compared and advanced atomically with the update, and a
stale writer gets HTTP 409 so the client reconciles instead of silently applying
last-write-wins. New Boards start at revision 1, not the `0` this record
describes.

Changed: the concurrency token no longer travels as request/response body
fields. `base_revision` / `new_revision` appear nowhere in `apps/api`; the
`BaseRevision int64` body field tagged `json:"base_revision"` was deleted in
`d6a3a972 refactor: complete workspace and model-pool cleanup`. The observed
revision is an `ETag` on reads and is sent back on writes as the `If-Match`
request header, and the persisted revision is read from the document's own
`revision` field.

Current contract:

- `apps/api/openapi.public.yaml` — required `If-Match` header
  (`components.parameters.IfMatch`) and the `ETag` response header.
- `apps/api/internal/runtime/interfaces/http/contract_v1_boards.go` —
  `parseBoardIfMatch` (missing / wildcard / unparsable render 400; stale values
  surface as 409 `board_revision_conflict` at write time).
- `apps/api/internal/boards/v1_workspace.go` — `ParseV1IfMatch`.
- `apps/web/src/features/board/infrastructure/api/boardsResource.ts` — the
  adapter maps the caller's `baseRevision` to the `If-Match` header and reads
  `newRevision` from the returned document's `revision`.
- `docs/architecture/frontend-contract-inventory.md` records the same delta.

No successor ADR exists for the Board wire format, so this record is
`deprecated` rather than `superseded` by an ADR; `DESIGN.md` and the OpenAPI
spec plus the Board adapter are the current source of truth.
