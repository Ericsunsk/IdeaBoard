# Library Context
Library owns the workspace asset collection: external-material (Unsplash/Pexels) saves, Folder management, and shared access plus bridging for Canvas/Search.
It keeps provider attribution and never copies objects into Media Storage.
It does NOT own Canvas graph state, Search feed state, or gallery rendering in consuming surfaces.

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Folder | Workspace folder (`WorkspaceLibraryFolder`: opaque `libf_` id, `name`); the virtual `all` aggregate is synthesized client-side; there are no system/virtual folders, no `quick-saves`, and no asset counts (the backend returns none). |
| Asset | `WorkspaceLibraryAsset` (opaque `liba_` id, `provider: unsplash\|pexels`, `sourceId`, camelCase `source` / `preview` / `originalUrl`, `title` / `author` / `w` / `h`, optional `folderId`); URLs are display-only. Saving without a folder leaves the asset unfiled (visible under `all`). |
| Asset Key | `provider:sourceId`; identity across Feed, floating island, and bridging. |
| Search Reference | Same-origin reference state as Asset for passing between vision/search and Canvas; not a persisted asset. |
| Materialize | Async job (`pending -> running -> completed\|failed`, opaque `libm_` id) converting a URL asset into a managed Media asset (`asset_...`) for Canvas; the sole materialization path from Library to Canvas. `materializeLibraryAssetForCanvas` polls the job and resolves display media; no numeric media link is recorded (board media-identity migration is a Phase 4 residual). |
| Folder Facts | Workspace-scoped folder collection snapshot (`folders` / `loading` / `load` / `refresh` / `reset` / `dispose` plus revision dedup: `current` / `stale` / `disposed` / `error`); shared by multiple islands. The fetch closes over the caller's workspace id. |
| Mutation Publisher | Broadcasts `save` / `remove` to all subscribers (`asset-saved` / `removed`); drives each surface `galleryEffect` (`none` / `reload-current` / `selection-will-reload`). |
| Transfer | Drag MIME (`application/x-ideaboard-{search-reference,library-folder-move,canvas-library-import}`) with `copy` / `move` / `none` semantics; asset ids are opaque (`liba_...`). |

## Public Seams

- `features/library` (`index.ts`) — sole public entry: `workspace`, `transfer`, `folderFacts`, `confirmations`, `useLibraryWorkspace`, floating-island lazy loader; external features never deep-import internals.
- `workspace.ts` — sole transport boundary: folders/assets pagination, folder create/rename/delete, asset save/move/remove, resolve lookup, async materialize, save/remove reference use-cases; talks to `/api/v1/workspaces/{id}/library` through `api.request`; snake_case maps to camelCase here; malformed ids throw before any network call.
- `createLibraryFolderFacts` plus `createLibraryMutationPublisher` — folder facts and mutation broadcast in `folderFacts.ts`; multiple surfaces share one instance; callers only see `galleryEffect` plus `affectedFolderKeys`. Facts require an explicit workspace-scoped fetch.
- `useLibraryWorkspace.ts` — asset-browsing use case: tabs/pagination/selection/lightbox/delete/edit/move/retry; in `{workspaceId/syncRoute/autoLoad/initialFolder/folderFacts/mutationPublisher}`, out `{workspaceId/folders/assets/selectedFolder/... + commands/resetWorkspace}`; normalizes `?folder=` (`all` / `<libf_ id>`).
- `write` / `read` / `getLibraryDropEffect` / `toCanvasLibraryImportDescriptor` — DnD read/write and effect resolution plus Canvas import descriptors in `transfer.ts`.
- `deleteFolderConfirmation` / `removeAssetConfirmation` — danger confirmations to `UserConfirmRequest` in `confirmations.ts`; copy and intent checks live here, not in callers.
- `LibraryFloatingIsland.vue` — island display: open/expanded, reference selection, add-to-canvas; props `{open/workspaceId/folderFacts/mutationPublisher/referenceKeys?/presentation}` plus emits; lazy-loaded via `loadLibraryFloatingIsland`.

## Invariants

- Server (workspace library, paginated `page_size + cursor`) is the folder/asset fact source; `selectedFolder` follows `?folder=` except in routeless cases with internal state.
- Every operation resolves its workspace id per call from `features/workspace` selection (store or `useWorkspaceSelection`); Library never duplicates the selection store; missing workspace fails closed.
- Writes must broadcast via the mutation publisher; surfaces react per `galleryEffect`; never full-reload on every change.
- Canvas consumption of Library assets must materialize; Search bridging passes only references (`libraryCoordinator`) and never reads/writes gallery directly.
- Folder deletion requires danger confirmation; deleting the selected folder falls back to `all`, never dangles.
- Deleting the selected folder falls back to `all`; `activeAsset` lightbox stays internal.
- MIME strings are a cross-surface contract; renames must sync all three ends.

## Non-goals

- No owned media assets; no provider-less assets.
- No numeric id or URL as identity; identity is `provider:sourceId`, durable ids are opaque (`libf_` / `liba_` / `libm_`).
- No reference as saved asset; no saved asset as reference.
- No canvas-direct provider URLs; no unmaterialized imports.
- No per-island folder fetch; no stale-facts render.
- No silent mutations; no untyped drag payloads or ambiguous drop effects.
- No plain-string folders; no `quick-saves` or folder asset counts (the backend has neither).
