import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useLibraryWorkspace } from './useLibraryWorkspace'
import { createLibraryFolderFacts } from './libraryFolderFacts'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  replace: vi.fn(async () => undefined),
  getLibraryFolders: vi.fn(),
  getLibraryAssets: vi.fn(),
  createFolder: vi.fn(),
  renameFolder: vi.fn(),
  removeFolder: vi.fn(),
  removeAsset: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: mocks.toastSuccess, error: mocks.toastError }),
}))

vi.mock('./libraryResource', () => ({
  createFolder: mocks.createFolder,
  getLibraryAssets: mocks.getLibraryAssets,
  getLibraryFolders: mocks.getLibraryFolders,
  libraryAssetKey: (provider: string, sourceID: string) => `${provider}:${sourceID}`,
  moveAsset: vi.fn(),
  removeAsset: mocks.removeAsset,
  removeFolder: mocks.removeFolder,
  renameFolder: mocks.renameFolder,
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve
  })
  return { promise, resolve }
}

describe('useLibraryWorkspace', () => {
  beforeEach(() => {
    mocks.route.query = {}
    vi.clearAllMocks()
    mocks.getLibraryFolders.mockResolvedValue([])
  })

  it('ignores an older Folder response after the user changes selection', async () => {
    const allPage = deferred<{
      items: Array<{ id: number }>
      nextCursor: string
      hasMore: boolean
    }>()
    const quickSavesPage = deferred<{
      items: Array<{ id: number }>
      nextCursor: string
      hasMore: boolean
    }>()
    mocks.getLibraryAssets.mockImplementation(
      ({ folder_id }: { folder_id: number | 'all' | 'quick-saves' }) =>
        folder_id === 'quick-saves' ? quickSavesPage.promise : allPage.promise,
    )

    const workspace = useLibraryWorkspace()
    await vi.waitFor(() => expect(mocks.getLibraryAssets).toHaveBeenCalledOnce())
    workspace.selectFolder('quick-saves')
    await vi.waitFor(() => expect(mocks.getLibraryAssets).toHaveBeenCalledTimes(2))

    quickSavesPage.resolve({ items: [{ id: 2 }], nextCursor: '', hasMore: false })
    await vi.waitFor(() => expect(workspace.assets.value).toEqual([{ id: 2 }]))
    allPage.resolve({ items: [{ id: 1 }], nextCursor: '', hasMore: false })
    await allPage.promise

    expect(workspace.selectedFolder.value).toBe('quick-saves')
    expect(workspace.assets.value).toEqual([{ id: 2 }])
  })

  it('publishes Folder create, rename, and delete through one shared snapshot', async () => {
    const original = {
      id: 1,
      query_key: '1',
      name: 'Original',
      is_system: false,
      is_default: false,
      is_virtual: false,
      asset_count: 0,
    }
    const renamed = { ...original, name: 'Renamed' }
    const facts = createLibraryFolderFacts()
    mocks.getLibraryFolders
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([original])
      .mockResolvedValueOnce([renamed])
      .mockResolvedValueOnce([])
    mocks.createFolder.mockResolvedValue(original)
    mocks.renameFolder.mockResolvedValue(renamed)
    mocks.removeFolder.mockResolvedValue(undefined)
    mocks.getLibraryAssets.mockResolvedValue({ items: [], nextCursor: '', hasMore: false })
    const workspace = useLibraryWorkspace({ autoLoad: false, folderFacts: facts })

    await workspace.loadFolders()
    expect(facts.folders.value).toEqual([])

    await workspace.createNewFolder('Original')
    expect(facts.folders.value).toEqual([original])
    expect(mocks.getLibraryAssets).toHaveBeenCalledOnce()

    await workspace.renameFolder('1', 'Renamed')
    expect(facts.folders.value).toEqual([renamed])
    expect(mocks.getLibraryAssets).toHaveBeenCalledOnce()

    await workspace.deleteFolder('1')
    expect(facts.folders.value).toEqual([])
    expect(mocks.getLibraryAssets).toHaveBeenCalledTimes(2)
  })

  it('publishes the removed asset identity for the Search feed coordination', async () => {
    const outcomes: unknown[] = []
    const publisher = {
      publishMutation: vi.fn(async (outcome) => {
        outcomes.push(outcome)
      }),
      publishMutations: vi.fn(async () => {}),
      subscribe: vi.fn(() => () => {}),
    }
    mocks.removeAsset.mockResolvedValue(undefined)
    const workspace = useLibraryWorkspace({
      autoLoad: false,
      mutationPublisher: publisher,
    })
    const asset = {
      id: 9,
      folder_id: 3,
      provider: 'pexels' as const,
      source_id: 'deleted-image',
      source_url: 'https://example.com/source',
      preview_url: 'https://example.com/preview',
      original_url: 'https://example.com/original',
      title: 'Deleted image',
      author: 'IdeaBoard',
      width: 100,
      height: 100,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    }

    await workspace.requestRemove(asset)

    expect(outcomes).toEqual([
      {
        kind: 'asset-removed',
        provider: 'pexels',
        sourceID: 'deleted-image',
        assetKey: 'pexels:deleted-image',
        affectedFolderKeys: ['all', '3'],
        galleryEffect: 'none',
      },
    ])
  })
})
