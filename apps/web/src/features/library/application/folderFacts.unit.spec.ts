import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildLibraryAssetMovedOutcome,
  buildLibraryAssetRemovedOutcome,
  buildLibraryAssetSavedOutcome,
  buildLibraryFolderOutcome,
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
  LIBRARY_ALL_FOLDER_KEY,
  libraryAffectedFolderKeysForAsset,
  uniqueLibraryFolderKeys,
} from './folderFacts'
import type { WorkspaceLibraryFolder } from '../domain/validators'

const fetchFolders = vi.fn<() => Promise<WorkspaceLibraryFolder[]>>()

function folder(id: string, name: string): WorkspaceLibraryFolder {
  return {
    id,
    workspaceId: 'ws_test123',
    name,
    createdAt: '2026-09-24T00:00:00Z',
    updatedAt: '2026-09-24T00:00:00Z',
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve
  })
  return { promise, resolve }
}

describe('Library Folder facts', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shares one load while multiple Search surfaces need folders', async () => {
    const request = deferred<WorkspaceLibraryFolder[]>()
    fetchFolders.mockReturnValue(request.promise)
    const facts = createLibraryFolderFacts(fetchFolders)

    const popover = facts.load()
    const island = facts.load()

    expect(fetchFolders).toHaveBeenCalledOnce()
    request.resolve([folder('lfolder_folder0001', 'References')])
    await expect(Promise.all([popover, island])).resolves.toEqual([
      { status: 'current', folders: [folder('lfolder_folder0001', 'References')] },
      { status: 'current', folders: [folder('lfolder_folder0001', 'References')] },
    ])
    expect(facts.folders.value).toEqual([folder('lfolder_folder0001', 'References')])
  })

  it('does not allow a pre-mutation response to overwrite refreshed facts', async () => {
    const staleRequest = deferred<WorkspaceLibraryFolder[]>()
    const currentRequest = deferred<WorkspaceLibraryFolder[]>()
    fetchFolders
      .mockReturnValueOnce(staleRequest.promise)
      .mockReturnValueOnce(currentRequest.promise)
    const facts = createLibraryFolderFacts(fetchFolders)

    const stale = facts.load()
    const current = facts.refresh()
    staleRequest.resolve([folder('lfolder_folder0001', 'Before save')])
    await stale

    expect(facts.folders.value).toEqual([])
    currentRequest.resolve([folder('lfolder_folder0002', 'After save')])
    await current

    expect(facts.folders.value).toEqual([folder('lfolder_folder0002', 'After save')])
  })

  it('drops identity-scoped facts and in-flight work on reset and dispose', async () => {
    const previousRequest = deferred<WorkspaceLibraryFolder[]>()
    const currentRequest = deferred<WorkspaceLibraryFolder[]>()
    fetchFolders
      .mockReturnValueOnce(previousRequest.promise)
      .mockReturnValueOnce(currentRequest.promise)
    const facts = createLibraryFolderFacts(fetchFolders)

    const previous = facts.load()
    facts.reset()
    const current = facts.load()
    previousRequest.resolve([folder('lfolder_folder0001', 'Previous user')])
    await expect(previous).resolves.toEqual({ status: 'stale' })
    expect(facts.folders.value).toEqual([])

    currentRequest.resolve([folder('lfolder_folder0002', 'Current user')])
    await expect(current).resolves.toEqual({
      status: 'current',
      folders: [folder('lfolder_folder0002', 'Current user')],
    })
    facts.dispose()

    expect(facts.folders.value).toEqual([])
    await expect(facts.load()).resolves.toEqual({ status: 'disposed' })
  })

  it('publishes committed mutations before one best-effort Folder refresh', async () => {
    const refresh = deferred<WorkspaceLibraryFolder[]>()
    fetchFolders.mockReturnValue(refresh.promise)
    const facts = createLibraryFolderFacts(fetchFolders)
    const publisher = createLibraryMutationPublisher(facts)
    const outcomes: unknown[] = []
    publisher.subscribe((outcome) => outcomes.push(outcome))

    const publication = publisher.publishMutations([
      { kind: 'folder-renamed', affectedFolderKeys: ['lfolder_folder0007'], galleryEffect: 'none' },
      { kind: 'folder-deleted', affectedFolderKeys: ['lfolder_folder0008'], galleryEffect: 'none' },
    ])

    expect(outcomes).toEqual([
      { kind: 'folder-renamed', affectedFolderKeys: ['lfolder_folder0007'], galleryEffect: 'none' },
      { kind: 'folder-deleted', affectedFolderKeys: ['lfolder_folder0008'], galleryEffect: 'none' },
    ])
    expect(fetchFolders).toHaveBeenCalledOnce()

    refresh.resolve([])
    await expect(publication).resolves.toBeUndefined()
  })

  it('keeps a committed mutation outcome when the Folder refresh fails', async () => {
    const cause = new Error('Folder refresh unavailable')
    fetchFolders.mockRejectedValue(cause)
    const facts = createLibraryFolderFacts(fetchFolders)
    const publisher = createLibraryMutationPublisher(facts)
    const outcomes: unknown[] = []
    publisher.subscribe((outcome) => outcomes.push(outcome))

    await publisher.publishMutation({
      kind: 'folder-renamed',
      affectedFolderKeys: ['lfolder_folder0007'],
      galleryEffect: 'none',
    })

    expect(outcomes).toEqual([
      { kind: 'folder-renamed', affectedFolderKeys: ['lfolder_folder0007'], galleryEffect: 'none' },
    ])
  })
})

describe('Library mutation-fact builders', () => {
  it('derives saved/removed folders from the stored asset with the caller policy intact', () => {
    expect(
      buildLibraryAssetSavedOutcome(
        { provider: 'unsplash', sourceId: 'u1', folderId: 'lfolder_folder0001' },
        'reload-current',
      ),
    ).toEqual({
      kind: 'asset-saved',
      provider: 'unsplash',
      sourceID: 'u1',
      assetKey: 'unsplash:u1',
      affectedFolderKeys: ['all', 'lfolder_folder0001'],
      galleryEffect: 'reload-current',
    })
    expect(
      buildLibraryAssetRemovedOutcome(
        { provider: 'pexels', sourceId: 'p1', folderId: undefined },
        'reload-current',
      ),
    ).toEqual({
      kind: 'asset-removed',
      provider: 'pexels',
      sourceID: 'p1',
      assetKey: 'pexels:p1',
      affectedFolderKeys: ['all'],
      galleryEffect: 'reload-current',
    })
    // `selection-will-reload` stays a distinct caller-driven policy.
    expect(
      buildLibraryAssetSavedOutcome(
        { provider: 'unsplash', sourceId: 'u1', folderId: undefined },
        'selection-will-reload',
      ).galleryEffect,
    ).toBe('selection-will-reload')
  })

  it('covers both Folder ends for a move without duplicates', () => {
    expect(
      buildLibraryAssetMovedOutcome(
        {
          previousFolderId: 'lfolder_folder0003',
          asset: { provider: 'unsplash', sourceId: 'u1', folderId: 'lfolder_folder0007' },
        },
        'none',
      ),
    ).toEqual({
      kind: 'asset-moved',
      affectedFolderKeys: ['all', 'lfolder_folder0003', 'lfolder_folder0007'],
      galleryEffect: 'none',
    })
    expect(
      buildLibraryAssetMovedOutcome(
        {
          previousFolderId: 'lfolder_folder0007',
          asset: { provider: 'unsplash', sourceId: 'u1', folderId: 'lfolder_folder0007' },
        },
        'none',
      ).affectedFolderKeys,
    ).toEqual(['all', 'lfolder_folder0007'])
  })

  it('builds Folder outcomes from the committed Folder id', () => {
    expect(
      buildLibraryFolderOutcome('folder-created', 'lfolder_folder0009', 'selection-will-reload'),
    ).toEqual({
      kind: 'folder-created',
      affectedFolderKeys: ['lfolder_folder0009'],
      galleryEffect: 'selection-will-reload',
    })
  })

  it('always includes the virtual all aggregate for filed assets', () => {
    expect(LIBRARY_ALL_FOLDER_KEY).toBe('all')
    expect(libraryAffectedFolderKeysForAsset('lfolder_folder0001')).toEqual([
      'all',
      'lfolder_folder0001',
    ])
    expect(libraryAffectedFolderKeysForAsset(undefined)).toEqual(['all'])
    expect(uniqueLibraryFolderKeys(['all', 'all', 'lfolder_folder0001', undefined])).toEqual([
      'all',
      'lfolder_folder0001',
    ])
  })
})
