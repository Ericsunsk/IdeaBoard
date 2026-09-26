// Workspace-scoped library resource (issue #806; split in issue #844).
//
// Talks to the v1 endpoints under `/api/v1/workspaces/{id}/library`
// through the generated `public/workspace-library` hooks (`/api/v1/...`
// via `publicMutator`). Pages and components must consume this module via
// the `features/library` barrel, never generated code directly.
//
// This module is the SOLE library transport surface: the legacy user-scoped
// `resource.ts` (integer ids, `/user/library/*`) is deleted. Ids are
// opaque (`ws_` + canonical `lfolder_` / `lasset_` / `lmat_`);
// malformed ids throw before any network call, matching the backend 404
// posture. Payloads map snake_case to camelCase at this boundary;
// callers only see camelCase.
//
// Layout (issue #844): pure id asserts + mappers live in
// `domain/validators` (no i18n/network), transport in
// `infrastructure/resource`, save/lookup/materialize orchestration +
// polling in `application/libraryUseCases`. This file only re-exports those
// modules so the `features/library` barrel names — and every `./workspace`
// deep import — stay stable with zero call-site churn.

export {
  assertLibraryAssetId,
  assertLibraryFolderId,
  assertLibraryMaterializationId,
  assertWorkspaceId,
  libraryAssetKey,
  mapWorkspaceLibraryMaterializationStatus,
  mapWorkspaceLibraryProvider,
  mapWorkspaceLibraryResolveStatus,
  requireWorkspaceId,
  resolveLibraryAssetCardURL,
  toWorkspaceLibraryAsset,
  toWorkspaceLibraryAssetInput,
  toWorkspaceLibraryFolder,
  toWorkspaceLibraryMaterialization,
  toWorkspaceLibraryResolveResult,
  type LibrarySearchReference,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryAssetInput,
  type WorkspaceLibraryAssetProvider,
  type WorkspaceLibraryFolder,
  type WorkspaceLibraryMaterialization,
  type WorkspaceLibraryMaterializationStatus,
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
