import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useLibraryWorkspace } from './useLibraryWorkspace'
import { createLibraryFolderFacts } from './folderFacts'
import type { WorkspaceLibraryAsset, WorkspaceLibraryFolder } from './workspace'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  replace: vi.fn(async () => undefined),
  listFolders: vi.fn(),
  listAssets: vi.fn(),
  createFolder: vi.fn(),
  renameFolder: vi.fn(),
  removeFolder: vi.fn(),
  removeAsset: vi.fn(),
  moveAsset: vi.fn(),
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

vi.mock('./workspace', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./workspace')>()
  return {
    ...actual,
    listAllWorkspaceLibraryFolders: mocks.listFolders,
    workspaceLibraryApi: {
      listFolders: vi.fn(),
      createFolder: mocks.createFolder,
      renameFolder: mocks.renameFolder,
      deleteFolder: mocks.removeFolder,
      listAssets: mocks.listAssets,
      saveAsset: vi.fn(),
      moveAsset: mocks.moveAsset,
      deleteAsset: mocks.removeAsset,
      resolveAssets: vi.fn(),
    },
  }
})

function folder(id: string, name: string): WorkspaceLibraryFolder {
  return {
    id,
    workspaceId: WORKSPACE_ID,
    name,
    createdAt: '2026-09-24T00:00:00Z',
    updatedAt: '2026-09-24T00:00:00Z',
  }
}

function asset(overrides: Partial<WorkspaceLibraryAsset> = {}): WorkspaceLibraryAsset {
  return {
    id: 'lasset_asset0009',
    workspaceId: WORKSPACE_ID,
    folderId: 'lfolder_folder0003',
    provider: 'pexels',
    sourceId: 'deleted-image',
    sourceUrl: 'https://example.com/source',
    previewUrl: 'https://example.com/preview',
    originalUrl: 'https://example.com/original',
    title: 'Deleted image',
    author: 'IdeaBoard',
    width: 100,
    height: 100,
    createdAt: '2026-09-24T00:00:00Z',
    updatedAt: '2026-09-24T00:00:00Z',
    ...overrides,
  }
}

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
    mocks.listFolders.mockResolvedValue([])
  })

  it('ignores an older Folder response after the user changes selection', async () => {
    const allPage = deferred<{
      items: Array<{ id: string }>
      nextCursor: string
      hasMore: boolean
    }>()
    const folderPage = deferred<{
      items: Array<{ id: string }>
      nextCursor: string
      hasMore: boolean
    }>()
    mocks.listAssets.mockImplementation((workspaceId: string, options: { folderId?: string }) =>
      options.folderId ? folderPage.promise : allPage.promise,
    )

    const workspace = useLibraryWorkspace({ workspaceId: WORKSPACE_ID })
    await vi.waitFor(() => expect(mocks.listAssets).toHaveBeenCalledOnce())
    workspace.selectFolder('lfolder_folder0002')
    await vi.waitFor(() => expect(mocks.listAssets).toHaveBeenCalledTimes(2))

    folderPage.resolve({ items: [{ id: 'lasset_asset0002' }], nextCursor: '', hasMore: false })
    await vi.waitFor(() =>
      expect(workspace.assets.value).toEqual([{ id: 'lasset_asset0002' }]),
    )
    allPage.resolve({ items: [{ id: 'lasset_asset0001' }], nextCursor: '', hasMore: false })
    await allPage.promise

    expect(workspace.selectedFolder.value).toBe('lfolder_folder0002')
    expect(workspace.assets.value).toEqual([{ id: 'lasset_asset0002' }])
  })

  it('publishes Folder create, rename, and delete through one shared snapshot', async () => {
    const original = folder('lfolder_folder0001', 'Original')
    const renamed = { ...original, name: 'Renamed' }
    const fetchFolders = vi
      .fn<() => Promise<WorkspaceLibraryFolder[]>>()
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([original])
      .mockResolvedValueOnce([renamed])
      .mockResolvedValueOnce([])
    const facts = createLibraryFolderFacts(fetchFolders)
    mocks.createFolder.mockResolvedValue(original)
    mocks.renameFolder.mockResolvedValue(renamed)
    mocks.removeFolder.mockResolvedValue(undefined)
    mocks.listAssets.mockResolvedValue({ items: [], nextCursor: '', hasMore: false })
    const workspace = useLibraryWorkspace({
      workspaceId: WORKSPACE_ID,
      autoLoad: false,
      folderFacts: facts,
    })

    await workspace.loadFolders()
    expect(facts.folders.value).toEqual([])

    await workspace.createNewFolder('Original')
    expect(facts.folders.value).toEqual([original])
    expect(mocks.listAssets).toHaveBeenCalledOnce()

    await workspace.renameFolder('lfolder_folder0001', 'Renamed')
    expect(facts.folders.value).toEqual([renamed])
    expect(mocks.listAssets).toHaveBeenCalledOnce()

    await workspace.deleteFolder('lfolder_folder0001')
    expect(facts.folders.value).toEqual([])
    expect(mocks.listAssets).toHaveBeenCalledTimes(2)
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
      workspaceId: WORKSPACE_ID,
      autoLoad: false,
      mutationPublisher: publisher,
    })

    await workspace.requestRemove(asset())

    expect(outcomes).toEqual([
      {
        kind: 'asset-removed',
        provider: 'pexels',
        sourceID: 'deleted-image',
        assetKey: 'pexels:deleted-image',
        affectedFolderKeys: ['all', 'lfolder_folder0003'],
        galleryEffect: 'none',
      },
    ])
  })
})
