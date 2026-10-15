# Board Context

Board owns the Board document model, Schema, catalog repository, sync protocol, gallery use cases, and share-join workflow.
It is the sole repository and synchronization fact source for boards.
It does NOT own Canvas session orchestration, Vue Flow runtime instances, or viewport UI state.

## Domain Language

| Term                       | Meaning                                                                                                                                                                                                                                 |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Board Document             | Immutable pure-data snapshot (`BoardState` with `schemaVersion`, `imageNodes`, `promptNodes`, `fusionNodes`, `connections`, `groups`, `mediaRefs`); owned by `core/document`.                                                           |
| Schema Version             | The version of the persisted document format; distinct from Board Revision, which orders document writes.                                                                                                                               |
| Board State Envelope       | Safe transport/persistence envelope `BoardStateEnvelope`; unified by `core/schema/boardStateEnvelopeContract.ts`.                                                                                                                       |
| Media Reference Extraction | Extraction of persisted media-asset refs from canvas nodes on save; feeds backend refcount and GC lock.                                                                                                                                 |
| Board Repository           | Sole catalog fact source (`session/catalog/repository.ts`): local catalog state plus CRUD commands plus server hydrate facade.                                                                                                          |
| Board Sync & Revision      | Optimistic-lock sync protocol (`session/sync/boardSync.ts`): document PUT carries `If-Match` with the observed revision; the response document supplies the new `revision`; 409 enters rollback/reconcile.                              |
| Share Join                 | Share-collaboration join use case (`session/share-join/joinSharedBoardWorkflow.ts`): token validation, join, hydrate.                                                                                                                   |
| Board Preview Scene        | Server-computed layout projection (`core/preview/scene.ts`, contract v1): geometry plus media references from the accepted snapshot. Gallery renders it as SVG (`ui/gallery/SceneThumbnail.vue`); never screenshots, never prompt text. |
| Catalog Gallery            | Gallery UI (`ui/gallery`): `BoardsPage` / `BoardCard`, gallery controller, command handlers, feedback, skin; pure view composition over repository data plus catalog scenes.                                                            |

## Public Seams

- `core/index.ts` — pure node definitions, snapshot model, fusion ratio, sizing and aspect-ratio rules (via `core/document/index.ts`, `core/schema/index.ts`, `core/media/index.ts`).
- `useBoardRepository()` / `createBoardRepository()` — single cross-layer catalog entry (via `session/catalog/index.ts`); local store plus CRUD commands plus server hydrate.
  Its optional `registerDraftGuard` method binds a live session dirty predicate to the repository lifecycle. Canvas unregisters it on Vue scope disposal; workspace navigation keeps the same session predicate scoped to the active selection.
- `hydrateBoardsFromServer` — server hydrate entry in `session/sync`.
- `syncBoardToServer` — revision-gated document-only save entry in `session/sync` (PUT document with `If-Match`, never PATCHes the name); `syncBoardRenameToServer` is the sole explicit rename entry (PATCH name, no document write).
- `getBoardSyncConflictKind` / `isBoardSyncConflictError` — sync conflict classifiers in `session/sync` (revision/idempotency vs ordinary failure; archived is separate via `isBoardArchivedError`). v1 has no board-name uniqueness: only local `isBoardNameTaken` gates names.
- `joinSharedBoardByToken` — share-join workflow entry in `session/share-join`.
- `boardsResource.ts` — sole generated-query `public/boards` adapter in `session/adapters`: `fetchWorkspaceBoards`, `fetchWorkspaceBoardDocument`, `createWorkspaceBoard`, `saveWorkspaceBoard` (document-only PUT with `If-Match`), `renameWorkspaceBoard` (explicit PATCH rename, no document write), `archiveWorkspaceBoard` / `restoreWorkspaceBoard`, `joinWorkspaceBoardByShareToken`, plus fail-closed `requireBoardWorkspaceId` (selection store, personal default; no selection throws `BoardWorkspaceUnavailableError` before any network call). Catalog metas carry the strict-parsed server Scene (`previewScene`) plus its opaque `contentVersion`; invalid scenes fail closed to null and never trigger document reads.
- `serializeBoardState` — serialization and current-schema validation adapters (via `session/persistence/index.ts`) in `session/persistence`. The retired `previewSnapshot*` cache fields are gone from the schema; storage boundaries strip them one-way so old drafts and server documents keep their content.
- `createCatalog` — Gallery read and command module: derives scene association reactively from the scoped Query catalog list and visible Board ids. Content identity preserves scene references; offline ids remain empty. Query observation follows the Catalog scope.
- `core/preview/renderer.ts` — pure Scene-to-thumbnail view-model (fixed `preview` tier, latest-output-wins picks, fusion slot grid, unit-space connection paths, bounded raster selection). `session/preview/thumbnailMedia.ts` resolves rasters through the authorized Workspace Media transport with bounded retry, terminal-error cutoffs, last-good retention, and visibility/disposal guards.
- `createCatalog` / `useCatalog` / `BoardsPage` — gallery page, controller, and views (via root `board/index.ts`) in `ui/gallery`. Cards render `SceneThumbnail` (SVG geometry plus token classes); theme switches re-map tokens only.
- Gallery copy keys are canonical under `board.*` in the locale bundles (including the h1: `board.title`). New callers use `board.*`; existing `workspace.boards.*` aliases have matching locale values.
- `board/testing` — Board-internal factories and sync fixtures; never enters the production barrel.
- `board/ports/testing.ts` — test-only snapshot/state factories for other contexts; production callers use `board/ports/canvas.ts`.
- `board/index.ts` — public entry for non-Canvas consumers and route composition; Canvas uses the narrow port below.
- `board/ports/canvas.ts` — narrow BoardCanvas port for Canvas callers (issue #861): snapshot facts, local persistence, revision-gated sync, and media-ref extraction only. Canvas saves documents only and never renders gallery previews; Canvas production code must use this subpath, never the full barrel.

## Sync / Query Ownership (ADR-0014)

Current production server catalog/document snapshots live in TanStack Query under the canonical keys below; the feature-owned store keeps the draft overlay and merge precedence. The production Query boundary is defined by `session/sync/boardQueryKeys.ts`, `boardQueries.ts`, and `boardSseInvalidation.ts`.

| Concern            | Owner                                              |
| ------------------ | -------------------------------------------------- |
| Server snapshot    | TanStack Query (catalog list + per-board document) |
| Draft / optimistic | Feature-owned store (`session/catalog` + states)   |
| Scope              | Selection port (`features/workspace` selection)    |
| SSE                | Invalidation signal only, never snapshot transport |
| Task fact          | River + PostgreSQL                                 |
| Event transport    | NATS JetStream                                     |

- Query key shapes: `['boards', workspaceId, 'list']` for the catalog list; `['boards', workspaceId, 'board', boardId, 'document']` for each board document. No other board key roots.
- Freshness: list rides `accountScoped` (stale 15s); document uses the global default (stale 5s); `gcTime` 10m and `retry 1` stay on the global `queryClient` defaults.
- Focus: `refetchOnWindowFocus` stays `false`; explicit visible-invalidate is throttled to 5s; hidden visibility flushes the pending save and does not fetch.
- SSE mapping: `board.document_replaced` invalidates the document (500ms coalesce; frames at or below the local revision are dropped; draft-dirty boards are marked stale without refetch or draft overwrite); `board.member_changed` invalidates the list; `board.share_revoked` invalidates the list and removes the document query; a stream gap invalidates only `['boards', workspaceId]`, leaving other workspace and task caches unchanged. SSE never calls `setQueryData` on document content; optimistic `setQueryData` is limited to create/rename local write-back.
- Draft detection compares normalized persisted content with a runtime server-content baseline, ignoring timestamps and derived preview caches. Unknown cold-storage content is conservative; accepted hydrate/PUT responses establish the baseline. Live Canvas dirty/saving predicates cover edits before autosave persistence. A PUT response establishes only its submitted server content, so edits made during the request remain dirty. Clean observer projections accept remote content despite local clock skew; dirty projections retain the draft and concurrency base.
- Ordinary Canvas navigation uses Query freshness (`force: false`); explicit recovery and visible refresh may force HTTP reads. Share-revocation hydration is injected by the production observer, keeping the SSE mapper independent of the sync orchestration module.
- Revision rules: document saves send the observed revision as `If-Match`; 409 keeps the draft and surfaces the conflict status until an explicit reload converges; archived writes stay on the banner path, never the conflict path; renames are PATCH-only (no document write) and adopt the server-advanced revision. ADR-0006 defines the optimistic concurrency contract. `apps/api/openapi.public.yaml`, `DESIGN.md`, and the Board API adapter own the current `If-Match`/409 wire contract.
- 404 rules: document PUT 404 never creates/upserts and preserves the draft/base revision; document GET 404 fails the whole hydrate and preserves existing drafts/revisions. Only absence in a successfully completed catalog list removes previously synced boards; never-synced offline creations remain.
- Draft precedence (3-layer): local-newer drafts win over older remote snapshots; explicit rollback (or `preserveLocalNewer: false`) converges to the remote snapshot; never-synced offline creations are kept while deleted-elsewhere boards are dropped. The overlay is frozen to the captured scope: a workspace switch drops the late response.
- Dual-view semantics: canvas renders the draft-overlay view (local draft over the server snapshot); gallery renders the Query snapshot view (catalog list only, never full board states for thumbnails). Scenes ride the catalog list response (`content_version`, `preview_scene`); the adapter strict-parses them and the gallery skips re-render while the content version is unchanged.

## Invariants

- `core` stays pure: no Vue reactivity, no browser DOM; pure functions only.
- Exactly one repository/sync fact source: `session/catalog` plus `session/sync`; Canvas `board-session` does orchestration only.
- Only `session/adapters/boardsResource.ts` may import the generated-query `public/boards` boundary; session/ui consume via adapter.
- Every board transport call resolves its workspace id per call from `features/workspace` selection (store or `useWorkspaceSelection`); Board never duplicates the selection store; missing workspace fails closed (sync returns `{ ok: false }`, saves/joins throw guidance) — never an inferred workspace.
- Board ids/revisions are server-minted: create/duplicate POST first, register the minted identity locally, then PUT the document. There is no PUT-upsert and no hard delete (delete archives).
- Canvas saves are transactionally consistent snapshots; image refs must first persist to `mediaAssetId`.
- Sync writes must carry `revision`; conflicts follow domain rollback/reconcile policy, never silent last-write-wins.
- `latestBoardState.ts` validates the current strict schema; unsupported versions fail validation. Schema evolution requires an explicit persistence-contract change, not an assumed load migration.
- Board state containing `retouchStack` is unsupported.
- Protected contracts (persisted schema, API semantics, storage keys, route URLs, serialization semantics) live in code and specs; no silent changes during restructuring.
- Never store a Vue Flow runtime instance as the document; never send dirty objects with runtime UI state to the backend.
- Never pass bare JSON across processes without a revision/version envelope.
- Never persist un-uploaded local Blob/Object URLs or temporary blob URLs as permanent layers.
- Never save without revision; never blind-overwrite on conflict.
- Never guess unknown fields dynamically; never feed unvalidated `JSON.parse` output into the canvas runtime.
- Gallery never fetches full board states to render thumbnails; gallery pages never call the generated client directly.
- Gallery never persists projections locally: scenes live in the TanStack Query catalog list; no screenshot cache, no theme-keyed preview store, no per-card document reads.
- Thumbnail rasters resolve through the authorized Workspace Media delivery at the fixed `preview` tier (independent of canvas zoom) with bounded retry and terminal-error cutoffs; missing media renders layout placeholders, never empty boards or success facts.

## Non-goals

- No second repository; no session-owned catalog truth.
- No second gallery renderer and no client-side scene extraction: the Go projection is the sole producer. Frontend parser and renderer tests consume the same expected wire fixtures as the Go producer tests.
- No Vue, Router, or runtime DOM in `core`.
- No page-owned join transport; share join goes through the collaboration workflow.
- No gallery-owned catalog facts; gallery is pure view composition.
