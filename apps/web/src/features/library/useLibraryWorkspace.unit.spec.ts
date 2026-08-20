import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useLibraryWorkspace } from './useLibraryWorkspace'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  replace: vi.fn(async () => undefined),
  getLibraryFolders: vi.fn(),
  getLibraryAssets: vi.fn(),
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
  createFolder: vi.fn(),
  getLibraryAssets: mocks.getLibraryAssets,
  getLibraryFolders: mocks.getLibraryFolders,
  moveAsset: vi.fn(),
  removeAsset: vi.fn(),
  removeFolder: vi.fn(),
  renameFolder: vi.fn(),
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
})
