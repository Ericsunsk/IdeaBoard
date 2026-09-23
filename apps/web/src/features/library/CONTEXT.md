# Library Context
Library owns the user asset collection: external-material (Unsplash/Pexels) saves, Folder management, and shared access plus bridging for Canvas/Search.
It keeps provider attribution and never copies objects into Media Storage.
It does NOT own Canvas graph state, Search feed state, or gallery rendering in consuming surfaces.

## Domain Language

| Term | Meaning |
| ---- | ------- |
| Folder | `{ query_key, is_system, is_virtual, isMutable }`; `all` is a virtual aggregate, `quick-saves` is the system default, numeric ids are user folders; `isMutable = !system && !virtual`. |
| Asset | `LibraryAsset` (`provider: unsplash\|pexels`, `source_id`, `source` / `preview` / `original_url`, `title` / `author` / `w` / `h`, `folder_id`); URLs are display-only. |
| Asset Key | `provider:source_id`; identity across Feed, floating island, and bridging. |
| Search Reference | Same-origin reference state as Asset for passing between vision/search and Canvas; not a persisted asset. |
| Materialize | Converts a URL asset into a managed Media asset (yields `mediaAssetId`) for Canvas; the sole materialization path from Library to Canvas. |
| Folder Facts | Identity-level folder collection snapshot (`folders` / `loading` / `load` / `refresh` / `reset` / `dispose` plus revision dedup: `current` / `stale` / `disposed` / `error`); shared by multiple islands. |
| Mutation Publisher | Broadcasts `save` / `remove` to all subscribers (`asset-saved` / `removed`); drives each surface `galleryEffect` (`none` / `reload-current` / `selection-will-reload`). |
| Transfer | Drag MIME (`application/x-ideaboard-{search-reference,library-folder-move,canvas-library-import}`) with `copy` / `move` / `none` semantics. |

## Public Seams

- `features/library` (`index.ts`) — sole public entry: `resource`, `transfer`, `folderFacts`, `confirmations`, `useLibraryWorkspace`, floating-island lazy loader; external features never deep-import internals.
- `resource.ts` — sole transport boundary: folders/assets pagination, folder create/rename/delete, asset save/move/remove, materialize, key lookup; calls generated `user-library/*`; assembles provider card URLs (`w480`).
- `createLibraryFolderFacts` plus `createLibraryMutationPublisher` — folder facts and mutation broadcast in `folderFacts.ts`; multiple surfaces share one instance; callers only see `galleryEffect` plus `affectedFolderKeys`.
- `useLibraryWorkspace.ts` — asset-browsing use case: tabs/pagination/selection/lightbox/delete/edit/move/retry; in `{syncRoute/autoLoad/initialFolder/folderFacts/mutationPublisher}`, out `{folders/assets/selectedFolder/... + commands}`; normalizes `?folder=` (`all` / `quick-saves` / `<id>`).
- `write` / `read` / `getLibraryDropEffect` / `toCanvasLibraryImportDescriptor` — DnD read/write and effect resolution plus Canvas import descriptors in `transfer.ts`.
- `deleteFolderConfirmation` / `removeAssetConfirmation` — danger confirmations to `UserConfirmRequest` in `confirmations.ts`; copy and intent checks live here, not in callers.
- `LibraryFloatingIsland.vue` — island display: open/expanded, reference selection, add-to-canvas; props `{open/folderFacts/mutationPublisher/referenceKeys?/presentation}` plus emits; lazy-loaded via `loadLibraryFloatingIsland`.

## Invariants

- Server (`user-library`, paginated `limit 24 + cursor`) is the folder/asset fact source; `selectedFolder` follows `?folder=` except in routeless cases with internal state.
- Writes must broadcast via the mutation publisher; surfaces react per `galleryEffect`; never full-reload on every change.
- Canvas consumption of Library assets must materialize; Search bridging passes only references (`libraryCoordinator`) and never reads/writes gallery directly.
- Folder deletion requires danger confirmation; deleting the selected folder falls back to `quick-saves`, never dangles.
- Deleting the selected folder falls back to `quick-saves`; `activeAsset` lightbox stays internal.
- MIME strings are a cross-surface contract; renames must sync all three ends.

## Non-goals

- No owned media assets; no provider-less assets.
- No numeric id or URL as identity; identity is `provider:source_id`.
- No reference as saved asset; no saved asset as reference.
- No canvas-direct provider URLs; no unmaterialized imports.
- No per-island folder fetch; no stale-facts render.
- No silent mutations; no untyped drag payloads or ambiguous drop effects.
- No plain-string folders; no mutable system folders.
