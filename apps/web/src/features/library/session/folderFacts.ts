import { type ExternalImageProvider } from '../../../core/contracts/externalImageProviders'
import { ref, shallowRef, type Ref, type ShallowRef } from 'vue'
import { i18n } from '../../../core/i18n'
import { libraryAssetKey, type WorkspaceLibraryFolder } from '../core/validators'

export type LibraryFolderView = WorkspaceLibraryFolder

/** Virtual aggregate key for the unfiled-inclusive gallery view. */
export const LIBRARY_ALL_FOLDER_KEY = 'all'

export type LibraryFolderFacts = {
  folders: Readonly<ShallowRef<LibraryFolderView[]>>
  loading: Readonly<Ref<boolean>>
  load: () => Promise<LibraryFolderLoadResult>
  refresh: () => Promise<LibraryFolderLoadResult>
  reset: () => void
  dispose: () => void
}

export type LibraryFolderLoadResult =
  | { status: 'current'; folders: LibraryFolderView[] }
  | { status: 'stale' | 'disposed' }
  | { status: 'error'; error: Error }

export type LibraryGalleryEffect = 'none' | 'reload-current' | 'selection-will-reload'

export type LibraryMutationOutcome =
  | {
      kind: 'asset-saved'
      provider: ExternalImageProvider
      sourceID: string
      assetKey: string
      affectedFolderKeys: string[]
      galleryEffect: LibraryGalleryEffect
    }
  | {
      kind: 'asset-removed'
      provider: ExternalImageProvider
      sourceID: string
      assetKey: string
      affectedFolderKeys: string[]
      galleryEffect: LibraryGalleryEffect
    }
  | {
      kind: 'asset-moved'
      affectedFolderKeys: string[]
      galleryEffect: LibraryGalleryEffect
    }
  | {
      kind: 'folder-created' | 'folder-renamed' | 'folder-deleted'
      affectedFolderKeys: string[]
      galleryEffect: LibraryGalleryEffect
    }

export type LibraryMutationPublisher = {
  publishMutation: (outcome: LibraryMutationOutcome) => Promise<void>
  publishMutations: (outcomes: LibraryMutationOutcome[]) => Promise<void>
  subscribe: (listener: (outcome: LibraryMutationOutcome) => void) => () => void
}

// Committed writes own asset identities and affected folders.
// Surfaces choose galleryEffect: reload-current, selection-will-reload, or none.

export type LibraryAssetFact = {
  provider: ExternalImageProvider
  sourceId: string
  folderId?: string | null | undefined
}

/** `all` plus the concrete folder when the asset is filed. */
export function libraryAffectedFolderKeysForAsset(folderId?: string | null | undefined): string[] {
  return folderId ? [LIBRARY_ALL_FOLDER_KEY, folderId] : [LIBRARY_ALL_FOLDER_KEY]
}

/** Deduplicates folder keys while keeping first-seen order. */
export function uniqueLibraryFolderKeys(keys: Array<string | null | undefined>): string[] {
  const seen = new Set<string>()
  for (const key of keys) {
    if (!key || seen.has(key)) continue
    seen.add(key)
  }
  return [...seen]
}

export function buildLibraryAssetSavedOutcome(
  asset: LibraryAssetFact,
  galleryEffect: LibraryGalleryEffect,
): LibraryMutationOutcome {
  return {
    kind: 'asset-saved',
    provider: asset.provider,
    sourceID: asset.sourceId,
    assetKey: libraryAssetKey(asset.provider, asset.sourceId),
    affectedFolderKeys: libraryAffectedFolderKeysForAsset(asset.folderId),
    galleryEffect,
  }
}

export function buildLibraryAssetRemovedOutcome(
  asset: LibraryAssetFact,
  galleryEffect: LibraryGalleryEffect,
): LibraryMutationOutcome {
  return {
    kind: 'asset-removed',
    provider: asset.provider,
    sourceID: asset.sourceId,
    assetKey: libraryAssetKey(asset.provider, asset.sourceId),
    affectedFolderKeys: libraryAffectedFolderKeysForAsset(asset.folderId),
    galleryEffect,
  }
}

export function buildLibraryAssetMovedOutcome(
  input: { previousFolderId?: string | null | undefined; asset: LibraryAssetFact },
  galleryEffect: LibraryGalleryEffect,
): LibraryMutationOutcome {
  return {
    kind: 'asset-moved',
    affectedFolderKeys: uniqueLibraryFolderKeys([
      LIBRARY_ALL_FOLDER_KEY,
      input.previousFolderId,
      input.asset.folderId,
    ]),
    galleryEffect,
  }
}

export function buildLibraryFolderOutcome(
  kind: 'folder-created' | 'folder-renamed' | 'folder-deleted',
  folderId: string,
  galleryEffect: LibraryGalleryEffect,
): LibraryMutationOutcome {
  return { kind, affectedFolderKeys: [folderId], galleryEffect }
}

export function createLibraryMutationPublisher(
  folderFacts: LibraryFolderFacts,
): LibraryMutationPublisher {
  const listeners = new Set<(outcome: LibraryMutationOutcome) => void>()

  async function publishMutations(outcomes: LibraryMutationOutcome[]): Promise<void> {
    if (outcomes.length === 0) return

    // Resource mutations are already committed when they reach this boundary.
    // Refreshing Folder facts must never delay or hide their coordination outcome.
    for (const outcome of outcomes) {
      for (const listener of listeners) listener(outcome)
    }
    await folderFacts.refresh()
  }

  function publishMutation(outcome: LibraryMutationOutcome): Promise<void> {
    return publishMutations([outcome])
  }

  function subscribe(listener: (outcome: LibraryMutationOutcome) => void) {
    listeners.add(listener)
    return () => listeners.delete(listener)
  }

  return { publishMutation, publishMutations, subscribe }
}

/**
 * Owns one workspace-scoped Folder snapshot. Consumers share this boundary
 * so mutations cannot leave independently loaded Folder lists out of sync.
 * The fetch closes over the caller's workspace id (resolved per call from
 * `features/workspace`, never duplicated here).
 */
export function createLibraryFolderFacts(
  fetchFolders: () => Promise<LibraryFolderView[]>,
): LibraryFolderFacts {
  const folders = shallowRef<LibraryFolderView[]>([])
  const loading = ref(false)
  let loaded = false
  let disposed = false
  let revision = 0
  let inFlight: Promise<LibraryFolderLoadResult> | null = null
  let inFlightRevision = -1

  function invalidate() {
    loaded = false
    revision += 1
  }

  function reset() {
    invalidate()
    folders.value = []
    loading.value = false
    inFlight = null
    inFlightRevision = -1
  }

  function dispose() {
    disposed = true
    reset()
  }

  function load(): Promise<LibraryFolderLoadResult> {
    if (disposed) return Promise.resolve({ status: 'disposed' })
    if (loaded) return Promise.resolve({ status: 'current', folders: folders.value })
    if (inFlight && inFlightRevision === revision) return inFlight

    const requestRevision = revision
    loading.value = true
    const request = fetchFolders()
      .then((nextFolders): LibraryFolderLoadResult => {
        if (disposed) return { status: 'disposed' }
        if (requestRevision !== revision) return { status: 'stale' }
        folders.value = nextFolders
        loaded = true
        return { status: 'current', folders: nextFolders }
      })
      .catch((cause): LibraryFolderLoadResult => {
        if (disposed) return { status: 'disposed' }
        if (requestRevision !== revision) return { status: 'stale' }
        const nextError =
          cause instanceof Error
            ? cause
            : new Error(i18n.global.t('library.errors.loadFoldersFailed'))
        return { status: 'error', error: nextError }
      })
    inFlight = request
    inFlightRevision = requestRevision

    return request.finally(() => {
      if (inFlight === request) {
        inFlight = null
        inFlightRevision = -1
        loading.value = false
      }
    })
  }

  function refresh(): Promise<LibraryFolderLoadResult> {
    if (disposed) return Promise.resolve({ status: 'disposed' })
    invalidate()
    return load()
  }

  return { folders, loading, load, refresh, reset, dispose }
}
