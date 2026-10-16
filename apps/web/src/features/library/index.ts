/**
 * Library feature public entrypoint (issue #679).
 *
 * Canvas, Search, and any other feature consume Library exclusively through
 * this barrel. Deep relative imports into Library internals are not allowed
 * outside this feature.
 *
 * Layout: `core/` owns pure models, validation, and drag payloads;
 * `session/` owns use-case orchestration, workspace state, and the
 * workspace-library transport adapter; `ui/` owns the floating-island UI.
 * This barrel re-exports the public symbols so the surface stays stable
 * while internals move.
 */
export {
  assertLibraryAssetId,
  assertLibraryFolderId,
  assertWorkspaceId,
  libraryAssetKeyFor,
  mapWorkspaceLibraryProvider,
  mapWorkspaceLibraryResolveStatus,
  requireWorkspaceId,
  toWorkspaceLibraryAsset,
  toWorkspaceLibraryAssetInput,
  toWorkspaceLibraryMediaAssetInput,
  toWorkspaceLibraryFolder,
  toWorkspaceLibraryResolveResult,
  type LibraryAssetIdentity,
  type LibrarySearchReference,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryAssetInput,
  type WorkspaceLibraryAssetProvider,
  type WorkspaceLibraryExternalAsset,
  type WorkspaceLibraryExternalAssetInput,
  type WorkspaceLibraryFolder,
  type WorkspaceLibraryMediaAsset,
  type WorkspaceLibraryMediaAssetInput,
  type WorkspaceLibraryPage,
  type WorkspaceLibraryResolveResult,
  type WorkspaceLibraryResolveStatus,
} from './core/validators'
export * from './session/resource'
export {
  listAllWorkspaceLibraryFolders,
  lookupWorkspaceSavedAssetKeys,
  materializeLibraryAssetForCanvas,
  removeWorkspaceSearchReference,
  type LibraryAssetMaterializer,
  type LibraryMaterializeOptions,
  type LibraryMaterializedMedia,
  type WorkspaceLibraryRemoveResult,
  type WorkspaceLibrarySavedKey,
} from './session/libraryUseCases'
export * from './core/transfer'
export * from './session/folderFacts'
export * from './session/librarySaveModule'
export * from './ui/confirmations'
export * from './ui/libraryDropHitTest'
export * from './session/useLibraryWorkspace'
export type { LibraryDragPayload, LibraryDropTargets } from './core/drag'

/** Lazily load the floating island shell to keep it out of the initial bundle. */
export const loadLibraryFloatingIsland = () => import('./ui/LibraryFloatingIsland.vue')
