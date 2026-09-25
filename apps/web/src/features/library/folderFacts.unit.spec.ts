import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createLibraryFolderFacts, createLibraryMutationPublisher } from './folderFacts'
import type { WorkspaceLibraryFolder } from './workspace'

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
    fetchFolders.mockReturnValueOnce(staleRequest.promise).mockReturnValueOnce(currentRequest.promise)
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
