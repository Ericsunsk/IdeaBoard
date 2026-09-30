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
- `syncBoardToServer` — revision-gated save entry in `application/sync`.
- `isBoardNameConflictError` — conflict classifier in `application/sync`.
- `joinSharedBoardByToken` — share-join workflow entry in `application/collaboration`.
- `boardsResource.ts` — sole generated-query `public/boards` adapter in `infrastructure/api`: `fetchWorkspaceBoards`, `fetchWorkspaceBoardDocument`, `createWorkspaceBoard`, `saveWorkspaceBoard` (PATCH rename + PUT document with `If-Match`), `archiveWorkspaceBoard` / `restoreWorkspaceBoard`, `joinWorkspaceBoardByShareToken`, plus fail-closed `requireBoardWorkspaceId` (selection store, personal default; no selection throws `BoardWorkspaceUnavailableError` before any network call).
- `serializeBoardState` / `parseBoardState` — serialization and version-migration adapters (via `persistence/index.ts`) in `infrastructure/persistence`.
- `renderBoardPreviewDataUrl` / `createBoardPreviewRefreshQueue` — preview generation and refresh queue in `infrastructure/preview`.
- `createCatalog` / `useCatalog` / `BoardsPage` — gallery page, controller, and views (via root `board/index.ts`) in `presentation/catalog`.
- Gallery copy keys live only under `workspace.boards.*` in the shared locale bundles; there is no `board.*` namespace (the h1 included: `workspace.boards.title`).
- `board/testing` — snapshot/state factories for tests only; never enters the production barrel.
- `board/index.ts` — public entry for non-Canvas consumers and route composition; Canvas uses the narrow port below.
- `board/ports/canvas.ts` — narrow BoardCanvas port for Canvas callers (issue #861): snapshot facts, local persistence, revision-gated sync, preview rendering, and media-ref extraction only; Canvas production code must use this subpath, never the full barrel.

## Invariants

- `domain` stays pure: no Vue reactivity, no browser DOM; pure functions only.
- Exactly one repository/sync fact source: `application/catalog` plus `application/sync`; Canvas `board-session` does orchestration only.
- Only `infrastructure/api/boardsResource.ts` may import the generated-query `public/boards` boundary; application/presentation consume via adapter.
- Every board transport call resolves its workspace id per call from `features/workspace` selection (store or `useWorkspaceSelection`); Board never duplicates the selection store; missing workspace fails closed (sync returns `{ ok: false }`, saves/joins throw guidance) — never an inferred workspace.
- Board ids/revisions are server-minted: create/duplicate POST first, register the minted identity locally, then PUT the document. There is no PUT-upsert and no hard delete (delete archives).
- Canvas saves are transactionally consistent snapshots; image refs must first persist to `mediaAssetId`.
- Sync writes must carry `revision`; conflicts follow domain rollback/reconcile policy, never silent last-write-wins.
- Schema evolution keeps load-migration functions for historic versions (e.g. `latestBoardState.ts`); never break old board data in place.
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
