# Library Context

Library owns Workspace-scoped saved references, Folder management, and shared access for Canvas/Search.
A Library Asset has an explicit `external` or `media` Source. Library retains attribution and reference metadata; Media owns managed identity, bytes, ingestion, and image dimensions.
It does NOT own Canvas graph state, Search feed state, or gallery rendering in consuming surfaces.

## Domain Language

| Term               | Meaning                                                                                                                                                                                                                                                      |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Folder             | A persisted Workspace collection with an opaque `lfolder_` id and a name; `all` is a client-side aggregate, not a Folder identity.                                                                                                                           |
| Library Asset      | A saved Workspace reference with Source `external` (provider reference) or `media` (Media Asset identity); distinct from the managed Media Asset it may reference.                                                                                           |
| Asset Key          | The Source-qualified save identity within a Workspace: `provider:sourceId` for external images or `media:mediaAssetId` for managed images. Task lineage and Folder placement do not change it.                                                               |
| Search Reference   | Same-origin reference state as Asset for passing between vision/search and Canvas; not a persisted asset.                                                                                                                                                    |
| Materialization    | Ingest an external Library Asset into Media; a saved external reference does not imply image readiness. A managed Library Asset already carries its Media identity.                                                                                          |
| Folder Facts       | Workspace-scoped folder collection snapshot (`folders` / `loading` / `load` / `refresh` / `reset` / `dispose` plus revision dedup: `current` / `stale` / `disposed` / `error`); shared by multiple islands. The fetch closes over the caller's workspace id. |
| Mutation Publisher | Coordinates committed mutation facts, Folder Facts refresh, and surface notifications. A batch publishes its committed outcomes together; observer failures never change a committed save result.                                                            |
| Save Operation     | A pinned Workspace and target with per-item preparation, upload, save, and retry state. Existing Media identity is reused; successful uploads and save idempotency keys survive retries.                                                                     |
| Transfer           | Typed Pointer Events payloads for Search references and Library asset descriptors. Explicit receiving regions choose folder moves or Canvas imports without native drag MIME serialization.                                                                  |

## Public Seams

- `features/library` (`index.ts`) — sole public entry: `core/validators`, `session/resource`, `session/libraryUseCases`, `core/transfer`, `session/folderFacts`, `session/librarySaveModule`, `ui/confirmations`, `session/useLibraryWorkspace`, floating-island lazy loader; external features never deep-import internals.
- `session/resource.ts` — sole transport boundary: folders/assets pagination, folder create/rename/delete, asset save/move/remove, resolve lookup, poll-free Canvas media resolve, save/remove reference use-cases; talks to `/api/v1/workspaces/{id}/library` through the generated `public/workspace-library` hooks; snake_case maps to camelCase here; malformed ids throw before any network call.
- `createLibraryFolderFacts` plus `createLibraryMutationPublisher` — Workspace-scoped Folder snapshot and committed mutation synchronization in `session/folderFacts.ts`. Batch publication refreshes Folder Facts once and lets each surface decide its `galleryEffect` from the affected Folder keys; observer failures are isolated.
- `createLibrarySaveModule` — shared save orchestration for Search references, Canvas images, and native files. Owns preparation, upload-once retry, per-item progress, idempotent saves, batch publication, and notifications; adapters supply inputs and capture the target before asynchronous preparation.
- `session/useLibraryWorkspace.ts` — asset-browsing use case: tabs/pagination/selection/lightbox/delete/edit/move/retry; in `{workspaceId/syncRoute/autoLoad/initialFolder/folderFacts/mutationPublisher}`, out `{workspaceId/folders/assets/selectedFolder/... + commands/resetWorkspace}`; normalizes `?folder=` (`all` / `<lfolder_ id>`).
- `toCanvasLibraryImportDescriptor` — the minimal asset descriptor for Canvas imports in `core/transfer.ts`.
- `LibraryDragPayload` / `LibraryDropTargets` — typed reference and asset payloads using the shared `core/interaction` Pointer Events controller and target registry. Registered folders take precedence over their panel; targets unregister with their UI lifetime.
- `deleteFolderConfirmation` / `removeAssetConfirmation` — danger confirmations to `UserConfirmRequest` in `ui/confirmations.ts`; copy and intent checks live here, not in callers.
- `LibraryFloatingIsland.vue` — island display: open/expanded, reference selection, add-to-canvas; props `{open/workspaceId/folderFacts/mutationPublisher/saveModule/referenceKeys?/presentation/dropTargets?}` plus emits; lazy-loaded via `loadLibraryFloatingIsland`. Reference and native-file drops submit through the shared Save Operation; the island renders progress and retry controls. `useLibraryIslandDnD` owns drop intent and Folder selection. Pointer asset drops move existing Library assets between folders or import them into the Canvas. Dragging starts only from asset previews, hides the cursor, and cancels on Workspace changes, island closure or disposal.

Provider IDs and recognition share the pure `core/contracts/externalImageProviders` vocabulary with Admin; payload mapping and validation errors remain feature-owned.

## Invariants

- Server (workspace library, paginated `page_size + cursor`) is the folder/asset fact source; `selectedFolder` follows `?folder=` except in routeless cases with internal state.
- Save submission captures the Workspace and target before preparation begins. Workspace changes, cancellation, clear, and disposal invalidate the operation; stale completions cannot publish or restore cleared state. Library never duplicates the Workspace selection store; missing Workspace fails closed.
- Committed writes publish through the mutation publisher. A save batch retains per-item results but synchronizes only newly committed outcomes together; retries skip saved items. Surfaces apply at most one gallery reload per publication according to `galleryEffect`; notification failures never turn committed items into failed saves.
- External Canvas imports materialize through Media; managed imports reuse the stored Media Asset ID. Saving an existing managed Canvas image also reuses that identity, never a display rendition. Search bridging passes references through `libraryCoordinator` and never reads/writes the gallery directly.
- Source-specific save identity is shared by manual and Task auto-save paths. Task lineage is provenance only. Media dimensions are projected from Media; unknown dimensions remain a paired `null`, never copied into Library or invented by consumers.
- Browsing consumes inline `display` variants without materialization or per-card metadata requests. Cards, lightboxes, drag ghosts and Search reference previews share `core/media` tier selection; same-origin derivative bytes remain Workspace-authorized. Provider URLs retain source/import identity and attribution, never a display fallback or client-side thumbnail transform.
- Folder deletion requires danger confirmation; deleting the selected folder falls back to `all`, never dangles.
- The `activeAsset` lightbox stays internal.
- Pointer drag payloads use the shared typed contracts; native file drops remain a separate browser input path.

## Non-goals

- No Library-owned Media bytes, ingestion, or dimension facts; managed Library Assets carry Media identity rather than provider URLs.
- No numeric id, URL, Folder, or Task lineage as save identity; durable ids remain opaque (`lfolder_` / `lasset_` / `asset_`).
- No reference as saved asset; no saved asset as reference.
- No canvas-direct provider URLs; no unmaterialized imports.
- No per-island folder fetch; no stale-facts render.
- No silent mutations; no untyped drag payloads or ambiguous drop effects.
- No Folder names or virtual aggregate keys as persisted Folder identity; no client-invented asset counts.
