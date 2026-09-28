/**
 * Library feature public entrypoint (issue #679).
 *
 * Canvas, Search, and any other feature consume Library exclusively through
 * this barrel. Deep relative imports into Library internals are not allowed
 * outside this feature.
 *
 * Layout: `domain/` owns pure models, validation, and drag-transfer codecs;
 * `application/` owns use-case orchestration and workspace state;
 * `infrastructure/` owns the workspace-library transport adapter;
 * `components/` owns the floating-island UI. This barrel re-exports the
 * public symbols so the surface stays stable while internals move.
 */
export {
  assertLibraryAssetId,
  assertLibraryFolderId,
  assertWorkspaceId,
  libraryAssetKey,
  mapWorkspaceLibraryProvider,
  mapWorkspaceLibraryResolveStatus,
  requireWorkspaceId,
  resolveLibraryAssetCardURL,
  toWorkspaceLibraryAsset,
  toWorkspaceLibraryAssetInput,
  toWorkspaceLibraryFolder,
  toWorkspaceLibraryResolveResult,
  type LibrarySearchReference,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryAssetInput,
  type WorkspaceLibraryAssetProvider,
  type WorkspaceLibraryFolder,
  type WorkspaceLibraryPage,
  type WorkspaceLibraryResolveResult,
  type WorkspaceLibraryResolveStatus,
} from './domain/validators'
export * from './infrastructure/resource'
export {
  listAllWorkspaceLibraryFolders,
  lookupWorkspaceSavedAssetKeys,
  materializeLibraryAssetForCanvas,
  removeWorkspaceSearchReference,
  saveWorkspaceSearchReference,
  type LibraryAssetMaterializer,
  type LibraryMaterializeOptions,
  type LibraryMaterializedMedia,
  type WorkspaceLibrarySavedKey,
} from './application/libraryUseCases'
export * from './domain/transfer'
export * from './application/folderFacts'
export * from './components/confirmations'
export * from './application/useLibraryWorkspace'

/** Lazily load the floating island shell to keep it out of the initial bundle. */
export const loadLibraryFloatingIsland = () => import('./components/LibraryFloatingIsland.vue')
