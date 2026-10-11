# Board Context

Board owns the Board document model, Schema, catalog repository, sync protocol, gallery use cases, and share-join workflow.
It is the sole repository and synchronization fact source for boards.
It does NOT own Canvas session orchestration, Vue Flow runtime instances, or viewport UI state.

## Domain Language

| Term                          | Meaning                                                                                                                                                                                                        |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Board Document                | Immutable pure-data snapshot (`BoardState` with `schemaVersion`, `imageNodes`, `promptNodes`, `fusionNodes`, `connections`, `groups`, `mediaRefs`); owned by `domain/document`.                                |
| Schema Versioning & Migration | Strict snapshot validation via Valibot `stateSchema`; deterministic forward-compatible migration when loading historic versions.                                                                               |
| Board State Envelope          | Safe transport/persistence envelope `BoardStateEnvelope`; unified by `domain/schema/boardStateEnvelopeContract.ts`.                                                                                            |
| Media Reference Extraction    | Extraction of persisted media-asset refs from canvas nodes on save; feeds backend refcount and GC lock.                                                                                                        |
| Board Repository              | Sole catalog fact source (`application/catalog/repository.ts`): local catalog state plus CRUD commands plus server hydrate facade.                                                                             |
| Board Sync & Revision         | Optimistic-lock sync protocol (`application/sync/boardSync.ts`): document PUT carries `If-Match` with the observed revision; the response document supplies the new `revision`; 409 enters rollback/reconcile. |
| Share Join                    | Share-collaboration join use case (`application/collaboration/joinSharedBoardWorkflow.ts`): token validation, join, hydrate.                                                                                   |
| Board Preview                 | Thumbnail generator (`infrastructure/preview`): lightweight preview snapshot data URL from node layout and key renders for gallery cards.                                                                      |
| Catalog Gallery               | Gallery UI (`presentation/catalog`): `BoardsPage` / `BoardCard`, gallery controller, command handlers, feedback, skin; pure view orchestration over repository data.                                           |

## Public Seams

- `domain/index.ts` — pure node definitions, snapshot model, fusion ratio, sizing and aspect-ratio rules (via `document/index.ts`, `schema/index.ts`, `media/index.ts`).
- `useBoardRepository()` / `createBoardRepository()` — single cross-layer catalog entry (via `catalog/index.ts`); local store plus CRUD commands plus server hydrate.
- `hydrateBoardsFromServer` — server hydrate entry in `application/sync`.
- `syncBoardToServer` — revision-gated document-only save entry in `application/sync` (PUT document with `If-Match`, never PATCHes the name); `syncBoardRenameToServer` is the sole explicit rename entry (PATCH name, no document write).
- `getBoardSyncConflictKind` / `isBoardSyncConflictError` — sync conflict classifiers in `application/sync` (revision/idempotency vs ordinary failure; archived is separate via `isBoardArchivedError`). v1 has no board-name uniqueness: only local `isBoardNameTaken` gates names.
- `joinSharedBoardByToken` — share-join workflow entry in `application/collaboration`.
- `boardsResource.ts` — sole generated-query `public/boards` adapter in `infrastructure/api`: `fetchWorkspaceBoards`, `fetchWorkspaceBoardDocument`, `createWorkspaceBoard`, `saveWorkspaceBoard` (document-only PUT with `If-Match`), `renameWorkspaceBoard` (explicit PATCH rename, no document write), `archiveWorkspaceBoard` / `restoreWorkspaceBoard`, `joinWorkspaceBoardByShareToken`, plus fail-closed `requireBoardWorkspaceId` (selection store, personal default; no selection throws `BoardWorkspaceUnavailableError` before any network call).
- `serializeBoardState` / `parseBoardState` — serialization and version-migration adapters (via `persistence/index.ts`) in `infrastructure/persistence`.
- `renderBoardPreviewDataUrl` / `createBoardPreviewRefreshQueue` — preview generation and refresh queue in `infrastructure/preview`.
- `createCatalog` / `useCatalog` / `BoardsPage` — gallery page, controller, and views (via root `board/index.ts`) in `presentation/catalog`.
- Gallery copy keys live only under `workspace.boards.*` in the shared locale bundles; there is no `board.*` namespace (the h1 included: `workspace.boards.title`).
- `board/testing` — snapshot/state factories for tests only; never enters the production barrel.
- `board/index.ts` — public entry for non-Canvas consumers and route composition; Canvas uses the narrow port below.
- `board/ports/canvas.ts` — narrow BoardCanvas port for Canvas callers (issue #861): snapshot facts, local persistence, revision-gated sync, preview rendering, and media-ref extraction only; Canvas production code must use this subpath, never the full barrel.

## Sync / Query Ownership (accepted P2.5 design, ADR-0014; production migration is P3)

Current production server catalog/document state remains in `application/catalog`, `application/sync`, and persistence stores. The ownership below is the accepted P3 target. The P2.5 design/specs are integrated on local main and accepted by the named delegated integrator `/root` under the current closeout instruction; historical checkpoint deviations are recorded in the canonical ledger. P3 remains blocked until separately authorized. `testing/syncDesignHarness.ts` is a private test-only executable design harness; it is neither a production integration nor a public export. Local design verification, review, plan-owner acceptance, merge, CI, and P3 production red/green evidence are separate milestones in the canonical `docs/architecture/frontend-contract-inventory.md` ledger (`behaviors` and `syncDesign`).

| Concern                     | Owner                                                |
| --------------------------- | ---------------------------------------------------- |
| Server snapshot (P3 target) | TanStack Query (catalog list + per-board document)   |
| Draft / optimistic          | Feature-owned store (`application/catalog` + states) |
| Scope                       | Selection port (`features/workspace` selection)      |
| SSE                         | Invalidation signal only, never snapshot transport   |
| Task fact                   | River + PostgreSQL                                   |
| Event transport             | NATS JetStream                                       |

- Query key shapes: `['boards', workspaceId, 'list']` for the catalog list; `['boards', workspaceId, 'board', boardId, 'document']` for each board document. No other board key roots.
- Freshness: list rides `accountScoped` (stale 15s); document uses the global default (stale 5s); `gcTime` 10m and `retry 1` stay on the global `queryClient` defaults.
- Focus: `refetchOnWindowFocus` stays `false`; explicit visible-invalidate is throttled to 5s; hidden visibility flushes the pending save and does not fetch.
- SSE mapping: `board.document_replaced` invalidates the document (500ms coalesce; frames at or below the local revision are dropped; draft-dirty boards are marked stale without refetch or draft overwrite); `board.member_changed` invalidates the list; `board.share_revoked` invalidates the list and removes the document query; a stream gap invalidates only `['boards', workspaceId]`, leaving other workspace and task caches unchanged. SSE never calls `setQueryData` on document content; optimistic `setQueryData` is limited to create/rename local write-back.
- Revision rules: document saves send the observed revision as `If-Match`; 409 keeps the draft and surfaces the conflict status until an explicit reload converges; archived writes stay on the banner path, never the conflict path; renames are PATCH-only (no document write) and adopt the server-advanced revision. ADR-0006 is deprecated; only its optimistic concurrency invariant remains current. `apps/api/openapi.public.yaml`, `DESIGN.md`, and the Board API adapter own the current `If-Match`/409 wire contract.
- 404 rules: document PUT 404 never creates/upserts and preserves the draft/base revision; document GET 404 fails the whole hydrate and preserves existing drafts/revisions. Only absence in a successfully completed catalog list removes previously synced boards; never-synced offline creations remain.
- Draft precedence (3-layer): local-newer drafts win over older remote snapshots; explicit rollback (or `preserveLocalNewer: false`) converges to the remote snapshot; never-synced offline creations are kept while deleted-elsewhere boards are dropped. The overlay is frozen to the captured scope: a workspace switch drops the late response.
- P3 preserves the exact race/broadcast/conflict trio in ADR-0014: bind the scenarios to the production Query boundary, record a real pre-implementation red and post-implementation green. Design-harness green does not satisfy production migration.
- Dual-view semantics: canvas renders the draft-overlay view (local draft over the server snapshot); gallery renders the Query snapshot view (catalog list only, never full board states for thumbnails).

## Invariants

- `domain` stays pure: no Vue reactivity, no browser DOM; pure functions only.
- Exactly one repository/sync fact source: `application/catalog` plus `application/sync`; Canvas `board-session` does orchestration only.
- Only `infrastructure/api/boardsResource.ts` may import the generated-query `public/boards` boundary; application/presentation consume via adapter.
- Every board transport call resolves its workspace id per call from `features/workspace` selection (store or `useWorkspaceSelection`); Board never duplicates the selection store; missing workspace fails closed (sync returns `{ ok: false }`, saves/joins throw guidance) — never an inferred workspace.
- Board ids/revisions are server-minted: create/duplicate POST first, register the minted identity locally, then PUT the document. There is no PUT-upsert and no hard delete (delete archives).
- Canvas saves are transactionally consistent snapshots; image refs must first persist to `mediaAssetId`.
- Sync writes must carry `revision`; conflicts follow domain rollback/reconcile policy, never silent last-write-wins.
- Schema evolution keeps load-migration functions for historic versions (e.g. `latestBoardState.ts`); never break old board data in place.
- `retouchStack` has been fully removed with no migration: historical boards containing that field can no longer be opened (explicit user decision).
- Protected contracts (persisted schema, API semantics, storage keys, route URLs, serialization semantics) live in `docs/architecture/frontend-contract-inventory.md`; no silent changes during restructuring.
- Never store a Vue Flow runtime instance as the document; never send dirty objects with runtime UI state to the backend.
- Never pass bare JSON across processes without a revision/version envelope.
- Never persist un-uploaded local Blob/Object URLs or temporary blob URLs as permanent layers.
- Never save without revision; never blind-overwrite on conflict.
- Never guess unknown fields dynamically; never feed unvalidated `JSON.parse` output into the canvas runtime.
- Gallery never fetches full board states to render thumbnails; gallery pages never call the generated client directly.

## Non-goals

- No second repository; no session-owned catalog truth.
- No Vue, Router, or runtime DOM in `domain`.
- No page-owned join transport; share join goes through the collaboration workflow.
- No gallery-owned catalog facts; gallery is pure view composition.
