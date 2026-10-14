import { type ExternalImageProvider } from '../../../core/contracts/externalImageProviders'
// Workspace-scoped Library use cases and managed media reads.
// Canvas owns pending-node readiness waits and explicit retries after failure.

import { getWorkspaceMediaAssetMetadata } from '../../../api/mediaAssetDelivery'
import { withSignal } from '../../../core/utils/request'
import { PAGE_SIZES } from '../../../core/config/pagination'
import { resolveAssetUrl } from '../../../core/config/runtimeUrls'
import { i18n } from '../../../core/i18n'
import {
  assertLibraryAssetId,
  libraryAssetKey,
  toWorkspaceLibraryAssetInput,
  type LibrarySearchReference,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryFolder,
} from '../core/validators'
import { LIBRARY_ALL_FOLDER_KEY, uniqueLibraryFolderKeys } from './folderFacts'
import {
  WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS,
  workspaceLibraryApi,
  type WorkspaceLibraryRequestOptions,
} from './resource'

const LIBRARY_FOLDER_PAGE_SIZE = PAGE_SIZES.dense
const LIBRARY_FOLDER_MAX_PAGES = 10

/** Loads every workspace folder (cursor loop, bounded) for folder-facts snapshots. */
export async function listAllWorkspaceLibraryFolders(
  workspaceId: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<WorkspaceLibraryFolder[]> {
  const folders: WorkspaceLibraryFolder[] = []
  let cursor: string | undefined
  for (let page = 0; page < LIBRARY_FOLDER_MAX_PAGES; page += 1) {
    const result = await workspaceLibraryApi.listFolders(workspaceId, {
      ...options,
      ...(cursor == null ? {} : { cursor }),
      pageSize: LIBRARY_FOLDER_PAGE_SIZE,
    })
    folders.push(...result.items)
    if (!result.hasMore || !result.nextCursor) return folders
    cursor = result.nextCursor
  }
  return folders
}

export type WorkspaceLibrarySavedKey = {
  provider: string
  sourceId: string
}

/**
 * Saved-state lookup for the Search feed: resolves references against saved
 * workspace assets only (nothing is fetched) and returns `provider:source_id`
 * keys for the resolved ones. Chunked to the backend batch bound.
 */
export async function lookupWorkspaceSavedAssetKeys(
  workspaceId: string,
  keys: WorkspaceLibrarySavedKey[],
  options: WorkspaceLibraryRequestOptions = {},
): Promise<Set<string>> {
  const saved = new Set<string>()
  for (let index = 0; index < keys.length; index += WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS) {
    const chunk = keys.slice(index, index + WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS)
    if (chunk.length === 0) continue
    const results = await workspaceLibraryApi.resolveAssets(
      workspaceId,
      chunk.map((key) => ({ provider: key.provider, sourceId: key.sourceId })),
      options,
    )
    for (const result of results) {
      if (result.status !== 'resolved' || !result.asset) continue
      saved.add(libraryAssetKey(result.asset.provider, result.asset.sourceId))
    }
  }
  return saved
}

/** Committed save result: the stored asset plus where it actually landed. */
export type WorkspaceLibrarySaveResult = {
  asset: WorkspaceLibraryAsset
  /** Present only when this call created the folder. */
  createdFolder?: WorkspaceLibraryFolder | undefined
  assetKey: string
  affectedFolderKeys: string[]
}

/**
 * Saves a Search reference; creates the folder first when a new name is given.
 * The returned `affectedFolderKeys`/`createdFolder` are the committed write
 * facts — callers must not rebuild the target from their input folder id.
 */
export async function saveWorkspaceSearchReference(
  workspaceId: string,
  reference: LibrarySearchReference,
  folderId?: string,
  newFolderName?: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<WorkspaceLibrarySaveResult> {
  let targetFolderId = folderId
  let createdFolder: WorkspaceLibraryFolder | undefined
  const trimmedName = newFolderName?.trim()
  if (trimmedName) {
    createdFolder = await workspaceLibraryApi.createFolder(workspaceId, trimmedName, options)
    targetFolderId = createdFolder.id
  }
  const asset = await workspaceLibraryApi.saveAsset(
    workspaceId,
    toWorkspaceLibraryAssetInput(reference, targetFolderId),
    options,
  )
  return {
    asset,
    ...(createdFolder == null ? {} : { createdFolder }),
    assetKey: libraryAssetKey(asset.provider, asset.sourceId),
    affectedFolderKeys: uniqueLibraryFolderKeys([
      LIBRARY_ALL_FOLDER_KEY,
      asset.folderId,
      createdFolder?.id,
    ]),
  }
}

/** Committed remove result, or `null` when the reference was never saved. */
export type WorkspaceLibraryRemoveResult = {
  asset: WorkspaceLibraryAsset
  assetKey: string
  affectedFolderKeys: string[]
} | null

/**
 * Removes a saved Search reference (resolve, then delete the owned asset).
 * Returns the deleted asset facts so callers publish the actual affected
 * folders; `null` means the resolve found nothing and nothing was deleted.
 */
export async function removeWorkspaceSearchReference(
  workspaceId: string,
  provider: ExternalImageProvider,
  sourceId: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<WorkspaceLibraryRemoveResult> {
  const results = await workspaceLibraryApi.resolveAssets(
    workspaceId,
    [{ provider, sourceId }],
    options,
  )
  const asset = results.find((result) => result.status === 'resolved' && result.asset)?.asset
  if (!asset) return null
  await workspaceLibraryApi.deleteAsset(workspaceId, asset.id, options)
  return {
    asset,
    assetKey: libraryAssetKey(asset.provider, asset.sourceId),
    affectedFolderKeys: uniqueLibraryFolderKeys([LIBRARY_ALL_FOLDER_KEY, asset.folderId]),
  }
}

export type LibraryMaterializedMedia = {
  mediaAssetId: string
  url: string
  width: number
  height: number
}

export type LibraryAssetMaterializer = (assetId: string) => Promise<LibraryMaterializedMedia>

export type LibraryMaterializeOptions = WorkspaceLibraryRequestOptions

/** Read one owned Library asset through the workspace media delivery boundary. */
export async function materializeLibraryAssetForCanvas(
  workspaceId: string,
  assetId: string,
  options: LibraryMaterializeOptions = {},
): Promise<LibraryMaterializedMedia> {
  return resolveMaterializedMedia(workspaceId, assertLibraryAssetId(assetId), options)
}

async function resolveMaterializedMedia(
  workspaceId: string,
  assetId: string | undefined,
  options: LibraryMaterializeOptions,
): Promise<LibraryMaterializedMedia> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  if (!assetId) throw new Error(fallback)
  const metadata = await getWorkspaceMediaAssetMetadata(
    workspaceId,
    assetId,
    withSignal(options.signal),
  )
  const originalUrl = metadata.original?.url?.trim() ? metadata.original.url.trim() : ''
  const previewUrl = metadata.derivatives['large-preview']?.url?.trim()
    ? metadata.derivatives['large-preview'].url.trim()
    : ''
  const deliveryUrl = originalUrl || previewUrl
  if (!deliveryUrl) throw new Error(fallback)
  if (!Number.isFinite(metadata.width) || !Number.isFinite(metadata.height)) {
    throw new Error(fallback)
  }
  return {
    mediaAssetId: assetId,
    url: resolveAssetUrl(deliveryUrl),
    width: metadata.width,
    height: metadata.height,
  }
}
