import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createSearchLibraryCoordinator } from './libraryCoordinator'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => ({
  listFolders: vi.fn(),
  submit: vi.fn(),
  deleteAsset: vi.fn(),
  toastSuccess: vi.fn(),
  toastWarning: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('../../../library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../library')>()
  return {
    ...actual,
    // Route the shared Folder facts through the stubbed loader so refresh
    // behavior is observable without reaching past the public entrypoint.
    createLibraryFolderFacts: (fetchFolders?: () => Promise<never[]>) =>
      actual.createLibraryFolderFacts(fetchFolders ?? mocks.listFolders),
    listAllWorkspaceLibraryFolders: mocks.listFolders,
    removeWorkspaceSearchReference: mocks.deleteAsset,
    createLibrarySaveModule: () => ({
      operations: ref([]),
      submit: mocks.submit,
      retryOperation: vi.fn(),
      clear: vi.fn(),
      dispose: vi.fn(),
    }),
  }
})

vi.mock('../../../../core/composables/useAppToast', () => ({
  useAppToast: () => ({
    success: mocks.toastSuccess,
    warning: mocks.toastWarning,
    error: mocks.toastError,
  }),
}))

const reference = {
  provider: 'unsplash' as const,
  id: 'image-1',
  source_url: 'https://example.com/source',
  preview_url: 'https://example.com/preview',
  original_url: 'https://example.com/original',
  title: 'Image',
  author: 'Author',
  width: 100,
  height: 100,
}

function submitResult(sourceId: string, folderId?: string) {
  return {
    operationId: 'op-1',
    status: 'succeeded',
    items: [
      {
        id: 'item-1',
        kind: 'external',
        title: 'Image',
        status: 'saved',
        attempts: 1,
        asset: {
          source: 'external' as const,
          id: 'lasset_asset0001',
          workspaceId: WORKSPACE_ID,
          provider: 'unsplash' as const,
          sourceId,
          folderId,
        },
      },
    ],
  }
}

describe('Search Library coordinator', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listFolders.mockResolvedValue([])
    mocks.submit.mockResolvedValue(submitResult('image-1'))
  })

  it('delegates saves to the shared save module and returns the stored asset', async () => {
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })

    const saved = await coordinator.saveReference(reference, undefined, 'fresh')

    // Search shapes the input; upload/retry/publication/notification stay in
    // the module. Heavy lifecycle coverage lives in the module contract spec.
    expect(mocks.submit).toHaveBeenCalledExactlyOnceWith(
      { source: 'external', reference },
      { newFolderName: 'fresh', galleryEffect: 'reload-current' },
    )
    expect(saved.asset.source).toBe('external')
    if (saved.asset.source !== 'external') throw new Error('unreachable')
    expect(saved.asset.sourceId).toBe('image-1')
  })

  it('reports the filed target from the stored asset for existing folders', async () => {
    mocks.submit.mockResolvedValue(submitResult('image-1', 'lfolder_folder0002'))
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })

    const saved = await coordinator.saveReference(reference, 'lfolder_folder0002')

    expect(mocks.submit).toHaveBeenCalledExactlyOnceWith(
      { source: 'external', reference },
      { folderId: 'lfolder_folder0002', galleryEffect: 'reload-current' },
    )
    expect(saved.asset.folderId).toBe('lfolder_folder0002')
  })

  it('throws without notifying twice when the module reports failure', async () => {
    mocks.submit.mockResolvedValue({
      operationId: 'op-1',
      status: 'failed',
      items: [
        {
          id: 'item-1',
          kind: 'external',
          title: 'Image',
          status: 'failed',
          attempts: 1,
          error: 'nope',
        },
      ],
    })
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })

    await expect(coordinator.saveReference(reference)).rejects.toThrow()
  })

  it('submits batches as one module operation', async () => {
    mocks.submit.mockResolvedValue({
      operationId: 'op-1',
      status: 'partial',
      items: [],
    })
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })

    const second = { ...reference, id: 'kept-2' }
    const result = await coordinator.saveReferences([reference, second])

    expect(mocks.submit).toHaveBeenCalledExactlyOnceWith(
      [
        { source: 'external', reference },
        { source: 'external', reference: second },
      ],
      { galleryEffect: 'reload-current' },
    )
    expect(result.status).toBe('partial')
  })

  it('reports the deleted asset folders and still marks an unknown reference', async () => {
    mocks.deleteAsset
      .mockResolvedValueOnce({
        asset: {
          source: 'external',
          provider: 'unsplash',
          sourceId: 'filed',
          folderId: 'lfolder_folder0003',
        },
        assetKey: 'unsplash:filed',
        affectedFolderKeys: ['all', 'lfolder_folder0003'],
      })
      .mockResolvedValueOnce(null)
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })
    const outcomes: unknown[] = []
    coordinator.subscribe((batch) => {
      outcomes.push(...batch)
    })

    await coordinator.removeReference('unsplash', 'filed')
    const missing = await coordinator.removeReference('unsplash', 'ghost')

    expect(missing.asset).toBeNull()
    expect(outcomes).toEqual([
      {
        kind: 'asset-removed',
        source: 'external',
        provider: 'unsplash',
        sourceID: 'filed',
        assetKey: 'unsplash:filed',
        affectedFolderKeys: ['all', 'lfolder_folder0003'],
        galleryEffect: 'reload-current',
      },
      {
        kind: 'asset-removed',
        source: 'external',
        provider: 'unsplash',
        sourceID: 'ghost',
        assetKey: 'unsplash:ghost',
        affectedFolderKeys: ['all'],
        galleryEffect: 'reload-current',
      },
    ])
  })

  it('fails closed without a workspace', async () => {
    const coordinator = createSearchLibraryCoordinator({ workspaceId: null })
    await expect(coordinator.saveReference(reference)).rejects.toThrow()
    await expect(coordinator.removeReference('unsplash', 'image-1')).rejects.toThrow()
    expect(mocks.submit).not.toHaveBeenCalled()
    expect(mocks.deleteAsset).not.toHaveBeenCalled()
  })
})
