import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { mockAppToast } from '../../../test/toast'
import { useLibraryWorkspace } from './useLibraryWorkspace'
import { createLibraryFolderFacts } from './folderFacts'
import type { WorkspaceLibraryAsset, WorkspaceLibraryFolder } from '../core/validators'

const WORKSPACE_ID = 'ws_test123'
const WORKSPACE_A_ID = 'ws_test0001'
const WORKSPACE_B_ID = 'ws_test0002'

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
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('../../../core/composables/useAppToast')

vi.mock('./libraryUseCases', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./libraryUseCases')>()
  return {
    ...actual,
    listAllWorkspaceLibraryFolders: mocks.listFolders,
  }
})

vi.mock('./resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./resource')>()
  return {
    ...actual,
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

function asset(
  overrides: Partial<Extract<WorkspaceLibraryAsset, { source: 'external' }>> = {},
): WorkspaceLibraryAsset {
  return {
    source: 'external',
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
    display: mediaDisplayFixture(),
    updatedAt: '2026-09-24T00:00:00Z',
    ...overrides,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve
    reject = nextReject
  })
  return { promise, resolve, reject }
}

type AssetPage = {
  items: Array<{ id: string }>
  nextCursor: string
  hasMore: boolean
}

describe('useLibraryWorkspace', () => {
  beforeEach(() => {
    mocks.route.query = {}
    vi.clearAllMocks()
    mockAppToast()
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
    await vi.waitFor(() => expect(workspace.assets.value).toEqual([{ id: 'lasset_asset0002' }]))
    allPage.resolve({ items: [{ id: 'lasset_asset0001' }], nextCursor: '', hasMore: false })
    await allPage.promise

    expect(workspace.selectedFolder.value).toBe('lfolder_folder0002')
    expect(workspace.assets.value).toEqual([{ id: 'lasset_asset0002' }])
  })

  it('discards a late workspace A response after switching to workspace B with no reload', async () => {
    const pageA = deferred<AssetPage>()
    mocks.listAssets.mockReturnValue(pageA.promise)
    const activeWorkspace = ref<string | null>(WORKSPACE_A_ID)
    const workspace = useLibraryWorkspace({
      workspaceId: activeWorkspace,
      syncRoute: false,
      autoLoad: false,
    })

    const pending = workspace.loadAssets(true)
    await vi.waitFor(() => expect(mocks.listAssets).toHaveBeenCalledOnce())
    expect(mocks.listAssets.mock.calls[0]?.[0]).toBe(WORKSPACE_A_ID)

    // Island closed: the workspace switches with no reload/invalidation,
    // so the request id and folder guards still match the late response.
    activeWorkspace.value = WORKSPACE_B_ID

    pageA.resolve({ items: [{ id: 'lasset_asset0001' }], nextCursor: '', hasMore: false })
    await pending

    expect(workspace.assets.value).toEqual([])
    expect(workspace.loading.value).toBe(false)
  })

  it('discards a late workspace A failure after switching to workspace B', async () => {
    const pageA = deferred<AssetPage>()
    mocks.listAssets.mockReturnValue(pageA.promise)
    const activeWorkspace = ref<string | null>(WORKSPACE_A_ID)
    const workspace = useLibraryWorkspace({
      workspaceId: activeWorkspace,
      syncRoute: false,
      autoLoad: false,
    })

    const pending = workspace.loadAssets(true)
    await vi.waitFor(() => expect(mocks.listAssets).toHaveBeenCalledOnce())

    activeWorkspace.value = WORKSPACE_B_ID

    pageA.reject(new Error('workspace A exploded'))
    await pending

    expect(workspace.errorMessage.value).toBe('')
    expect(workspace.assets.value).toEqual([])
  })

  it('keeps all server-paginated assets available to load-more', async () => {
    const firstHalf = Array.from({ length: 300 }, (_, index) => ({
      id: `lasset_asset${String(index).padStart(4, '0')}`,
    }))
    const secondHalf = Array.from({ length: 300 }, (_, index) => ({
      id: `lasset_asset${String(index + 300).padStart(4, '0')}`,
    }))
    mocks.listAssets
      .mockResolvedValueOnce({ items: firstHalf, nextCursor: 'cursor-1', hasMore: true })
      .mockResolvedValueOnce({ items: secondHalf, nextCursor: '', hasMore: false })
    const workspace = useLibraryWorkspace({
      workspaceId: WORKSPACE_ID,
      syncRoute: false,
      autoLoad: false,
    })

    await workspace.loadAssets(true)
    expect(workspace.assets.value).toHaveLength(300)
    expect(workspace.hasMore.value).toBe(true)

    await workspace.loadAssets(false)
    expect(workspace.assets.value).toHaveLength(600)
    expect(workspace.hasMore.value).toBe(false)
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
        source: 'external',
        provider: 'pexels',
        sourceID: 'deleted-image',
        assetKey: 'pexels:deleted-image',
        affectedFolderKeys: ['all', 'lfolder_folder0003'],
        galleryEffect: 'none',
      },
    ])
  })

  it('publishes both Folder ends for a move from the committed asset', async () => {
    const source = folder('lfolder_folder0003', 'Source')
    const destination = folder('lfolder_folder0007', 'Destination')
    const facts = createLibraryFolderFacts(vi.fn(async () => [source, destination]))
    const outcomes: unknown[] = []
    const publisher = {
      publishMutation: vi.fn(async (outcome: unknown) => {
        outcomes.push(outcome)
      }),
      publishMutations: vi.fn(async () => {}),
      subscribe: vi.fn(() => () => {}),
    }
    mocks.listAssets.mockResolvedValue({ items: [], nextCursor: '', hasMore: false })
    const workspace = useLibraryWorkspace({
      workspaceId: WORKSPACE_ID,
      autoLoad: false,
      syncRoute: false,
      folderFacts: facts,
      mutationPublisher: publisher,
    })
    await workspace.loadFolders()
    workspace.assets.value = [asset({ id: 'lasset_asset0009', folderId: source.id })]
    const moved = asset({ id: 'lasset_asset0009', folderId: destination.id })
    mocks.moveAsset.mockResolvedValue(moved)

    const ok = await workspace.moveSelectedAsset('lasset_asset0009', destination.id)

    expect(ok).toBe(true)
    // Both ends come from the pre-move fact and the committed move result.
    expect(outcomes).toEqual([
      {
        kind: 'asset-moved',
        affectedFolderKeys: ['all', source.id, destination.id],
        galleryEffect: 'none',
      },
    ])
    expect(workspace.assets.value).toEqual([moved])
  })
})
