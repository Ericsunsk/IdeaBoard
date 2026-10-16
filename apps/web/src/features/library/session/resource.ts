import { type ExternalImageProvider } from '../../../core/contracts/externalImageProviders'
// Workspace-scoped library transport (issue #844; split from `../workspace`).
//
// Talks to the v1 endpoints under `/api/v1/workspaces/{id}/library`
// through the generated `public/workspace-library` hooks (`/api/v1/...`
// via `publicMutator`). Pages and components must consume this module via
// the `features/library` barrel, never generated code directly.
//
// Transport note: the generated hooks own routes/envelopes; this adapter
// keeps the opaque-id asserts, auto idempotency minting, resolve batch
// bound, cursor loops, and snake_case -> camelCase validators that have
// no generated equivalent.
//
// Backend notes callers must respect:
// - Folders are plain workspace folders: no system/virtual taxonomy, no
//   `quick-saves`, no asset counts. The island synthesizes the virtual
//   `all` aggregate client-side; saving without a folder leaves the asset
//   unfiled (visible under `all`). Deleting a folder unfiles its assets.
// - Asset save is idempotent on `(workspace, provider, source_id)` for
//   external rows and on `(workspace, media_asset_id)` for media rows.
// - Materialization runs internal-only on the backend (no public job
//   routes); Canvas resolves managed media through the shared workspace
//   media delivery boundary (see `./libraryUseCases`).

import {
  createWorkspaceLibraryFolder as createWorkspaceLibraryFolderTransport,
  deleteWorkspaceLibraryAsset as deleteWorkspaceLibraryAssetTransport,
  deleteWorkspaceLibraryFolder as deleteWorkspaceLibraryFolderTransport,
  listWorkspaceLibraryAssets as listWorkspaceLibraryAssetsTransport,
  listWorkspaceLibraryFolders as listWorkspaceLibraryFoldersTransport,
  moveWorkspaceLibraryAsset as moveWorkspaceLibraryAssetTransport,
  renameWorkspaceLibraryFolder as renameWorkspaceLibraryFolderTransport,
  resolveWorkspaceLibraryAssets as resolveWorkspaceLibraryAssetsTransport,
  saveWorkspaceLibraryAsset as saveWorkspaceLibraryAssetTransport,
} from '../../../api/generated-query/public/workspace-library/workspace-library'
import type { SaveLibraryAssetRequest } from '../../../api/generated-query/public/model/saveLibraryAssetRequest'
import { buildContractRequestHeaders, requireContractData } from '../../../api/contractKernel'
import type { AuthFailureMode } from '../../../api/client'
import { withSignal } from '../../../core/utils/request'
import { createUniqueId } from '../../../core/utils/ids'
import { i18n } from '../../../core/i18n'
import {
  assertLibraryAssetId,
  assertLibraryFolderId,
  assertWorkspaceId,
  toWorkspaceLibraryAsset,
  toWorkspaceLibraryFolder,
  toWorkspaceLibraryPage,
  toWorkspaceLibraryResolveResult,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryAssetInput,
  type WorkspaceLibraryFolder,
  type WorkspaceLibraryPage,
  type WorkspaceLibraryResolveResult,
} from '../core/validators'

export type WorkspaceLibraryRequestOptions = {
  requestId?: string | undefined
  idempotencyKey?: string | undefined
  signal?: AbortSignal | null | undefined
  // Per-request 401 policy, forwarded to the generated transport:
  // supplementary reads (saved-keys backfill) pass `silent` so a dead
  // session degrades locally instead of reloading the page. Writes keep
  // the default redirect.
  authFailure?: AuthFailureMode | undefined
}

function workspaceLibraryHeaders(options: WorkspaceLibraryRequestOptions): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: options.requestId,
    idempotencyKey: options.idempotencyKey,
  })
}

// Mutating endpoints require an `Idempotency-Key` (per-endpoint scope, see
// openapi). Default to a scoped unique key so callers stay replay-safe
// without threading keys through every island/popover call.
function withLibraryIdempotency(
  options: WorkspaceLibraryRequestOptions,
  scope: string,
): WorkspaceLibraryRequestOptions {
  if (options.idempotencyKey) return options
  return { ...options, idempotencyKey: `${scope}:${createUniqueId()}` }
}

function toGeneratedOptions(options: WorkspaceLibraryRequestOptions): {
  headers: Record<string, string>
  signal?: AbortSignal | undefined
  authFailure?: AuthFailureMode | undefined
} {
  return {
    headers: workspaceLibraryHeaders(options),
    ...withSignal(options.signal),
    ...(options.authFailure == null ? {} : { authFailure: options.authFailure }),
  }
}

export const WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS = 20

export const workspaceLibraryApi = {
  listFolders: async (
    workspaceId: string,
    options: WorkspaceLibraryRequestOptions & { cursor?: string; pageSize?: number } = {},
  ): Promise<WorkspaceLibraryPage<WorkspaceLibraryFolder>> => {
    const query: { cursor?: string; page_size?: number } = {}
    if (options.cursor) query.cursor = options.cursor
    if (options.pageSize != null) query.page_size = options.pageSize
    const page = await listWorkspaceLibraryFoldersTransport(
      assertWorkspaceId(workspaceId),
      query,
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryPage(
      page,
      toWorkspaceLibraryFolder,
      i18n.global.t('library.errors.loadFoldersFailed'),
    )
  },

  createFolder: async (
    workspaceId: string,
    name: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryFolder> => {
    const effective = withLibraryIdempotency(options, 'library:folder-create')
    const envelope = await createWorkspaceLibraryFolderTransport(
      assertWorkspaceId(workspaceId),
      { data: { name } },
      toGeneratedOptions(effective),
    )
    return toWorkspaceLibraryFolder(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('library.errors.createFolderFailed'),
      ),
    )
  },

  renameFolder: async (
    workspaceId: string,
    folderId: string,
    name: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryFolder> => {
    const envelope = await renameWorkspaceLibraryFolderTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryFolderId(folderId),
      { data: { name } },
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryFolder(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('library.errors.renameFolderFailed'),
      ),
    )
  },

  deleteFolder: async (
    workspaceId: string,
    folderId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<void> => {
    const effective = withLibraryIdempotency(options, 'library:folder-delete')
    await deleteWorkspaceLibraryFolderTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryFolderId(folderId),
      toGeneratedOptions(effective),
    )
  },

  listAssets: async (
    workspaceId: string,
    options: WorkspaceLibraryRequestOptions & {
      cursor?: string
      pageSize?: number
      folderId?: string
    } = {},
  ): Promise<WorkspaceLibraryPage<WorkspaceLibraryAsset>> => {
    const query: { cursor?: string; page_size?: number; folder_id?: string } = {}
    if (options.cursor) query.cursor = options.cursor
    if (options.pageSize != null) query.page_size = options.pageSize
    if (options.folderId) query.folder_id = assertLibraryFolderId(options.folderId)
    const page = await listWorkspaceLibraryAssetsTransport(
      assertWorkspaceId(workspaceId),
      query,
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryPage(
      page,
      toWorkspaceLibraryAsset,
      i18n.global.t('library.errors.loadAssetsFailed'),
    )
  },

  saveAsset: async (
    workspaceId: string,
    input: WorkspaceLibraryAssetInput,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryAsset> => {
    const effective = withLibraryIdempotency(options, 'library:asset-save')
    // The wire carries the `source` discriminator: external saves send the
    // provider/source identity plus URLs, media saves send only the uploaded
    // Media Asset identity (dimensions project server-side; no URL keys, no
    // task lineage, no operator identity, author display-only).
    const data: SaveLibraryAssetRequest['data'] =
      input.source === 'media'
        ? {
            source: 'media',
            media_asset_id: input.mediaAssetId,
            ...(input.title != null ? { title: input.title } : {}),
            ...(input.folderId != null ? { folder_id: assertLibraryFolderId(input.folderId) } : {}),
          }
        : {
            source: 'external',
            provider: input.provider,
            source_id: input.sourceId,
            ...(input.sourceUrl != null ? { source_url: input.sourceUrl } : {}),
            ...(input.previewUrl != null ? { preview_url: input.previewUrl } : {}),
            ...(input.originalUrl != null ? { original_url: input.originalUrl } : {}),
            ...(input.title != null ? { title: input.title } : {}),
            ...(input.author != null ? { author: input.author } : {}),
            ...(input.width != null ? { width: input.width } : {}),
            ...(input.height != null ? { height: input.height } : {}),
            ...(input.folderId != null ? { folder_id: assertLibraryFolderId(input.folderId) } : {}),
          }
    const envelope = await saveWorkspaceLibraryAssetTransport(
      assertWorkspaceId(workspaceId),
      { data },
      toGeneratedOptions(effective),
    )
    return toWorkspaceLibraryAsset(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('library.errors.saveAssetFailed'),
      ),
    )
  },

  moveAsset: async (
    workspaceId: string,
    assetId: string,
    folderId: string | null,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryAsset> => {
    const envelope = await moveWorkspaceLibraryAssetTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryAssetId(assetId),
      // Wire-identical to the retired transport: null rides as '' (unfiled).
      { data: { folder_id: folderId ?? '' } },
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryAsset(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('library.errors.moveAssetFailed'),
      ),
    )
  },

  deleteAsset: async (
    workspaceId: string,
    assetId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<void> => {
    const effective = withLibraryIdempotency(options, 'library:asset-delete')
    await deleteWorkspaceLibraryAssetTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryAssetId(assetId),
      toGeneratedOptions(effective),
    )
  },

  resolveAssets: async (
    workspaceId: string,
    items: Array<{ provider: string; sourceId?: string; sourceUrl?: string }>,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryResolveResult[]> => {
    if (items.length > WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS) {
      throw new Error(i18n.global.t('library.errors.lookupStatusFailed'))
    }
    const envelope = await resolveWorkspaceLibraryAssetsTransport(
      assertWorkspaceId(workspaceId),
      {
        data: {
          items: items.map((item) => {
            const entry: { provider: string } & Record<string, unknown> = {
              provider: item.provider,
            }
            if (item.sourceId != null) entry.source_id = item.sourceId
            if (item.sourceUrl != null) entry.source_url = item.sourceUrl
            return entry as {
              provider: ExternalImageProvider
              source_id?: string
              source_url?: string
            }
          }),
        },
      },
      toGeneratedOptions(options),
    )
    const result = requireContractData<{ items: unknown }>(
      envelope,
      i18n.global.t('library.errors.lookupStatusFailed'),
    )
    if (!Array.isArray(result.items))
      throw new Error(i18n.global.t('library.errors.lookupStatusFailed'))
    return result.items.map(toWorkspaceLibraryResolveResult)
  },
}
