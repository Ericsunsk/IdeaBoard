import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createLibraryFolderFacts, createLibraryMutationPublisher } from './libraryFolderFacts'

const mocks = vi.hoisted(() => ({ getLibraryFolders: vi.fn() }))

vi.mock('./libraryResource', () => ({ getLibraryFolders: mocks.getLibraryFolders }))

function folder(id: number, name: string) {
  return {
    id,
    query_key: String(id),
    name,
    is_system: false,
    is_default: false,
    is_virtual: false,
    asset_count: 0,
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
    const request = deferred<ReturnType<typeof folder>[]>()
    mocks.getLibraryFolders.mockReturnValue(request.promise)
    const facts = createLibraryFolderFacts()

    const popover = facts.load()
    const island = facts.load()

    expect(mocks.getLibraryFolders).toHaveBeenCalledOnce()
    request.resolve([folder(1, 'References')])
    await expect(Promise.all([popover, island])).resolves.toEqual([
      { status: 'current', folders: [folder(1, 'References')] },
      { status: 'current', folders: [folder(1, 'References')] },
    ])
    expect(facts.folders.value).toEqual([folder(1, 'References')])
  })

  it('does not allow a pre-mutation response to overwrite refreshed facts', async () => {
    const staleRequest = deferred<ReturnType<typeof folder>[]>()
    const currentRequest = deferred<ReturnType<typeof folder>[]>()
    mocks.getLibraryFolders
      .mockReturnValueOnce(staleRequest.promise)
      .mockReturnValueOnce(currentRequest.promise)
    const facts = createLibraryFolderFacts()

    const stale = facts.load()
    const current = facts.refresh()
    staleRequest.resolve([folder(1, 'Before save')])
    await stale

    expect(facts.folders.value).toEqual([])
    currentRequest.resolve([folder(2, 'After save')])
    await current

    expect(facts.folders.value).toEqual([folder(2, 'After save')])
  })

  it('drops identity-scoped facts and in-flight work on reset and dispose', async () => {
    const previousRequest = deferred<ReturnType<typeof folder>[]>()
    const currentRequest = deferred<ReturnType<typeof folder>[]>()
    mocks.getLibraryFolders
      .mockReturnValueOnce(previousRequest.promise)
      .mockReturnValueOnce(currentRequest.promise)
    const facts = createLibraryFolderFacts()

    const previous = facts.load()
    facts.reset()
    const current = facts.load()
    previousRequest.resolve([folder(1, 'Previous user')])
    await expect(previous).resolves.toEqual({ status: 'stale' })
    expect(facts.folders.value).toEqual([])

    currentRequest.resolve([folder(2, 'Current user')])
    await expect(current).resolves.toEqual({
      status: 'current',
      folders: [folder(2, 'Current user')],
    })
    facts.dispose()

    expect(facts.folders.value).toEqual([])
    await expect(facts.load()).resolves.toEqual({ status: 'disposed' })
  })

  it('publishes committed mutations before one best-effort Folder refresh', async () => {
    const refresh = deferred<ReturnType<typeof folder>[]>()
    mocks.getLibraryFolders.mockReturnValue(refresh.promise)
    const facts = createLibraryFolderFacts()
    const publisher = createLibraryMutationPublisher(facts)
    const outcomes: unknown[] = []
    publisher.subscribe((outcome) => outcomes.push(outcome))

    const publication = publisher.publishMutations([
      { kind: 'folder-renamed', affectedFolderKeys: ['7'], galleryEffect: 'none' },
      { kind: 'folder-deleted', affectedFolderKeys: ['8'], galleryEffect: 'none' },
    ])

    expect(outcomes).toEqual([
      { kind: 'folder-renamed', affectedFolderKeys: ['7'], galleryEffect: 'none' },
      { kind: 'folder-deleted', affectedFolderKeys: ['8'], galleryEffect: 'none' },
    ])
    expect(mocks.getLibraryFolders).toHaveBeenCalledOnce()

    refresh.resolve([])
    await expect(publication).resolves.toBeUndefined()
  })

  it('keeps a committed mutation outcome when the Folder refresh fails', async () => {
    const cause = new Error('Folder refresh unavailable')
    mocks.getLibraryFolders.mockRejectedValue(cause)
    const facts = createLibraryFolderFacts()
    const publisher = createLibraryMutationPublisher(facts)
    const outcomes: unknown[] = []
    publisher.subscribe((outcome) => outcomes.push(outcome))

    await publisher.publishMutation({
      kind: 'folder-renamed',
      affectedFolderKeys: ['7'],
      galleryEffect: 'none',
    })

    expect(outcomes).toEqual([
      { kind: 'folder-renamed', affectedFolderKeys: ['7'], galleryEffect: 'none' },
    ])
  })
})
