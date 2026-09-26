// Workspace-scoped library use cases (issue #844; split from `../workspace`).
//
// Save/lookup/materialize orchestration over the transport in
// `../infrastructure/resource`. Pure helpers keep their behavior; transport
// closes over an explicit workspace id resolved from `features/workspace`
// (never duplicated here). Display media for materialized assets resolves
// through the shared workspace media delivery boundary with the caller's
// explicit workspace.

import { getWorkspaceMediaAssetMetadata } from '../../../shared/api/mediaAssetDelivery'
import { resolveAssetUrl } from '../../../shared/config/runtimeUrls'
import { i18n } from '../../../shared/i18n'
import { createPollDelay } from '../../../shared/utils/polling'
import {
  libraryAssetKey,
  toWorkspaceLibraryAssetInput,
  type LibrarySearchReference,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryFolder,
  type WorkspaceLibraryMaterialization,
} from '../domain/validators'
import {
  WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS,
  workspaceLibraryApi,
  type WorkspaceLibraryRequestOptions,
} from '../infrastructure/resource'

const LIBRARY_FOLDER_PAGE_SIZE = 100
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

/** Saves a Search reference; creates the folder first when a new name is given. */
export async function saveWorkspaceSearchReference(
  workspaceId: string,
  reference: LibrarySearchReference,
  folderId?: string,
  newFolderName?: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<WorkspaceLibraryAsset> {
  let targetFolderId = folderId
  const trimmedName = newFolderName?.trim()
  if (trimmedName) {
    const folder = await workspaceLibraryApi.createFolder(workspaceId, trimmedName, options)
    targetFolderId = folder.id
  }
  return workspaceLibraryApi.saveAsset(
    workspaceId,
    toWorkspaceLibraryAssetInput(reference, targetFolderId),
    options,
  )
}

/** Removes a saved Search reference (resolve, then delete the owned asset). */
export async function removeWorkspaceSearchReference(
  workspaceId: string,
  provider: 'unsplash' | 'pexels',
  sourceId: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<void> {
  const results = await workspaceLibraryApi.resolveAssets(
    workspaceId,
    [{ provider, sourceId }],
    options,
  )
  const asset = results.find((result) => result.status === 'resolved' && result.asset)?.asset
  if (!asset) return
  await workspaceLibraryApi.deleteAsset(workspaceId, asset.id, options)
}

export type LibraryMaterializedMedia = {
  mediaAssetId: string
  url: string
  width: number
  height: number
}

export type LibraryAssetMaterializer = (assetId: string) => Promise<LibraryMaterializedMedia>

export type LibraryMaterializeOptions = WorkspaceLibraryRequestOptions & {
  pollAttempts?: number | undefined
  pollDelayMs?: number | undefined
}

function materializeDelay(ms: number, signal?: AbortSignal | null): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException('The request was aborted.', 'AbortError'))
      return
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    function onAbort() {
      clearTimeout(timer)
      reject(new DOMException('The request was aborted.', 'AbortError'))
    }
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

/**
 * Turns an owned workspace Library asset into a managed Media asset for
 * Canvas: creates the materialization job, polls it to a terminal state,
 * then resolves display media through the shared delivery boundary.
 */
export async function materializeLibraryAssetForCanvas(
  workspaceId: string,
  assetId: string,
  options: LibraryMaterializeOptions = {},
): Promise<LibraryMaterializedMedia> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  const job = await workspaceLibraryApi.createMaterialization(workspaceId, assetId, options)
  const snapshot = await pollLibraryMaterialization(workspaceId, job.id, options)
  if (snapshot.status === 'failed') {
    throw new Error(snapshot.errorMessage?.trim() || fallback)
  }
  if (snapshot.status !== 'completed') throw new Error(fallback)
  return resolveMaterializedMedia(workspaceId, snapshot.mediaAssetId, options)
}

async function pollLibraryMaterialization(
  workspaceId: string,
  materializationId: string,
  options: LibraryMaterializeOptions,
): Promise<WorkspaceLibraryMaterialization> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  const attempts = options.pollAttempts ?? 20
  const delayMs = options.pollDelayMs ?? 1000
  // Fixed-cadence polling rides the shared delay primitive; the
  // terminal-state predicate stays local to this module.
  const retryDelay = createPollDelay({ initialDelayMs: delayMs, maxDelayMs: delayMs })
  let snapshot = await workspaceLibraryApi.getMaterialization(
    workspaceId,
    materializationId,
    options,
  )
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    if (snapshot.status === 'completed' || snapshot.status === 'failed') return snapshot
    await materializeDelay(retryDelay.next(), options.signal)
    snapshot = await workspaceLibraryApi.getMaterialization(workspaceId, materializationId, options)
  }
  if (snapshot.status === 'completed' || snapshot.status === 'failed') return snapshot
  throw new Error(fallback)
}

async function resolveMaterializedMedia(
  workspaceId: string,
  mediaAssetId: string | undefined,
  options: LibraryMaterializeOptions,
): Promise<LibraryMaterializedMedia> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  if (!mediaAssetId) throw new Error(fallback)
  const metadata = await getWorkspaceMediaAssetMetadata(workspaceId, mediaAssetId, {
    ...(options.signal == null ? {} : { signal: options.signal }),
  })
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
    mediaAssetId,
    url: resolveAssetUrl(deliveryUrl),
    width: metadata.width,
    height: metadata.height,
  }
}
