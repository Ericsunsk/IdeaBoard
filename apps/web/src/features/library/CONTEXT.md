# Library Context

Library owns the workspace asset collection: external-material (Unsplash/Pexels) saves, Folder management, and shared access plus bridging for Canvas/Search.
Library retains provider attribution and reference metadata; Media owns byte ingestion requested by materialization.
It does NOT own Canvas graph state, Search feed state, or gallery rendering in consuming surfaces.

## Domain Language

| Term               | Meaning                                                                                                                                                                                                                                                      |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Folder             | Workspace folder (`WorkspaceLibraryFolder`: opaque `lfolder_` id, `name`); the virtual `all` aggregate is synthesized client-side; there are no system/virtual folders, no `quick-saves`, and no asset counts (the backend returns none).                    |
| Library Asset      | A saved external image reference and provider attribution within a Workspace; distinct from a managed Media Asset.                                                                                                                                           |
| Asset Key          | The provider-qualified source identity of an external image, shared by Search and Library.                                                                                                                                                                   |
| Search Reference   | Same-origin reference state as Asset for passing between vision/search and Canvas; not a persisted asset.                                                                                                                                                    |
| Materialization    | Resolve a saved Library Asset to a managed Media Asset; saving the reference alone does not mean its image is ready.                                                                                                                                         |
| Folder Facts       | Workspace-scoped folder collection snapshot (`folders` / `loading` / `load` / `refresh` / `reset` / `dispose` plus revision dedup: `current` / `stale` / `disposed` / `error`); shared by multiple islands. The fetch closes over the caller's workspace id. |
| Mutation Publisher | Broadcasts `save` / `remove` to all subscribers (`asset-saved` / `removed`); drives each surface `galleryEffect` (`none` / `reload-current` / `selection-will-reload`).                                                                                      |
| Transfer           | Typed Pointer Events payloads for Search references and Library asset descriptors. Explicit receiving regions choose folder moves or Canvas imports without native drag MIME serialization.                                                                  |

## Public Seams

- `features/library` (`index.ts`) — sole public entry: `core/validators`, `session/resource`, `session/libraryUseCases`, `core/transfer`, `session/folderFacts`, `ui/confirmations`, `session/useLibraryWorkspace`, floating-island lazy loader; external features never deep-import internals.
- `session/resource.ts` — sole transport boundary: folders/assets pagination, folder create/rename/delete, asset save/move/remove, resolve lookup, poll-free Canvas media resolve, save/remove reference use-cases; talks to `/api/v1/workspaces/{id}/library` through the generated `public/workspace-library` hooks; snake_case maps to camelCase here; malformed ids throw before any network call.
- `createLibraryFolderFacts` plus `createLibraryMutationPublisher` — folder facts and mutation broadcast in `session/folderFacts.ts`; multiple surfaces share one instance; callers only see `galleryEffect` plus `affectedFolderKeys`. Facts require an explicit workspace-scoped fetch.
- `session/useLibraryWorkspace.ts` — asset-browsing use case: tabs/pagination/selection/lightbox/delete/edit/move/retry; in `{workspaceId/syncRoute/autoLoad/initialFolder/folderFacts/mutationPublisher}`, out `{workspaceId/folders/assets/selectedFolder/... + commands/resetWorkspace}`; normalizes `?folder=` (`all` / `<lfolder_ id>`).
- `toCanvasLibraryImportDescriptor` — the minimal asset descriptor for Canvas imports in `core/transfer.ts`.
- `LibraryDragPayload` / `LibraryDropTargets` — typed reference and asset payloads using the shared `core/interaction` Pointer Events controller and target registry. Registered folders take precedence over their panel; targets unregister with their UI lifetime.
- `deleteFolderConfirmation` / `removeAssetConfirmation` — danger confirmations to `UserConfirmRequest` in `ui/confirmations.ts`; copy and intent checks live here, not in callers.
- `LibraryFloatingIsland.vue` — island display: open/expanded, reference selection, add-to-canvas; props `{open/workspaceId/folderFacts/mutationPublisher/referenceKeys?/presentation/dropTargets?}` plus emits; lazy-loaded via `loadLibraryFloatingIsland`. Direct reference drops use `useLibraryIslandDnD.receiveReference` for saving, mutation publication, and folder selection. Pointer asset drops move existing Library assets between folders or import them into the Canvas. Dragging starts only from asset previews, hides the cursor, and cancels on Workspace changes, island closure or disposal.

Provider IDs and recognition share the pure `core/contracts/externalImageProviders` vocabulary with Admin; payload mapping and validation errors remain feature-owned.

## Invariants

- Server (workspace library, paginated `page_size + cursor`) is the folder/asset fact source; `selectedFolder` follows `?folder=` except in routeless cases with internal state.
- Every operation resolves its workspace id per call from `features/workspace` selection (store or `useWorkspaceSelection`); Library never duplicates the selection store; missing workspace fails closed.
- Writes must broadcast via the mutation publisher; surfaces react per `galleryEffect`; never full-reload on every change.
- Canvas consumption of Library assets must materialize; Search bridging passes only references (`libraryCoordinator`) and never reads/writes gallery directly.
- Browsing consumes inline `display` variants without materialization or per-card metadata requests. Cards, lightboxes, drag ghosts and Search reference previews share `core/media` tier selection; same-origin derivative bytes remain Workspace-authorized. Provider URLs retain source/import identity and attribution, never a display fallback or client-side thumbnail transform.
- Folder deletion requires danger confirmation; deleting the selected folder falls back to `all`, never dangles.
- The `activeAsset` lightbox stays internal.
- Pointer drag payloads use the shared typed contracts; native file drops remain a separate browser input path.

## Non-goals

- No owned media assets; no provider-less assets.
- No numeric id or URL as identity; identity is `provider:sourceId`, durable ids are opaque (`lfolder_` / `lasset_`).
- No reference as saved asset; no saved asset as reference.
- No canvas-direct provider URLs; no unmaterialized imports.
- No per-island folder fetch; no stale-facts render.
- No silent mutations; no untyped drag payloads or ambiguous drop effects.
- No plain-string folders; no `quick-saves` or folder asset counts (the backend has neither).
