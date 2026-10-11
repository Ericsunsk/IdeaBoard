import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSearchLibraryCoordinator } from './libraryCoordinator'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => ({
  listFolders: vi.fn(),
  saveAsset: vi.fn(),
  resolveAssets: vi.fn(),
  deleteAsset: vi.fn(),
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
    saveWorkspaceSearchReference: mocks.saveAsset,
    removeWorkspaceSearchReference: mocks.deleteAsset,
    libraryAssetKey: (provider: string, sourceID: string) => `${provider}:${sourceID}`,
  }
})

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

function savedResult(overrides: Record<string, unknown> = {}) {
  const asset = {
    id: 'lasset_asset0001',
    workspaceId: WORKSPACE_ID,
    provider: 'unsplash',
    sourceId: 'image-1',
    sourceUrl: 'https://example.com/source',
    previewUrl: 'https://example.com/preview',
    originalUrl: 'https://example.com/original',
    title: 'Image',
    author: 'Author',
    width: 100,
    height: 100,
    createdAt: '2026-09-24T00:00:00Z',
    updatedAt: '2026-09-24T00:00:00Z',
    ...overrides,
  } as Record<string, unknown>
  const folderId = asset.folderId as string | undefined
  const sourceId = asset.sourceId as string
  return {
    asset,
    assetKey: `unsplash:${sourceId}`,
    affectedFolderKeys: folderId ? ['all', folderId] : ['all'],
  }
}

describe('Search Library coordinator', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listFolders.mockResolvedValue([])
  })

  it('refreshes the shared Folder facts after save and remove mutations', async () => {
    mocks.saveAsset.mockResolvedValue(savedResult())
    mocks.deleteAsset.mockResolvedValue({
      asset: {
        provider: 'unsplash',
        sourceId: 'image-1',
        folderId: undefined,
      },
      assetKey: 'unsplash:image-1',
      affectedFolderKeys: ['all'],
    })
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })
    const outcomes: unknown[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome))

    await coordinator.saveReference(reference)
    await coordinator.removeReference('unsplash', 'image-1')

    expect(mocks.saveAsset).toHaveBeenCalledWith(WORKSPACE_ID, reference, undefined, undefined)
    expect(mocks.deleteAsset).toHaveBeenCalledWith(WORKSPACE_ID, 'unsplash', 'image-1')
    expect(mocks.listFolders).toHaveBeenCalledTimes(2)
    expect(outcomes).toEqual([
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'image-1',
        assetKey: 'unsplash:image-1',
        affectedFolderKeys: ['all'],
        galleryEffect: 'reload-current',
      },
      {
        kind: 'asset-removed',
        provider: 'unsplash',
        sourceID: 'image-1',
        assetKey: 'unsplash:image-1',
        affectedFolderKeys: ['all'],
        galleryEffect: 'reload-current',
      },
    ])
  })

  it('carries the actually created Folder identity when saving into a new folder', async () => {
    mocks.saveAsset.mockResolvedValue({
      asset: {
        provider: 'unsplash',
        sourceId: 'image-1',
        folderId: 'lfolder_folder0009',
      },
      createdFolder: { id: 'lfolder_folder0009', name: 'fresh' },
      assetKey: 'unsplash:image-1',
      affectedFolderKeys: ['all', 'lfolder_folder0009'],
    })
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })
    const outcomes: unknown[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome))

    const saved = await coordinator.saveReference(reference, undefined, 'fresh')

    expect(mocks.saveAsset).toHaveBeenCalledWith(WORKSPACE_ID, reference, undefined, 'fresh')
    // The caller never rebuilds the target from its input: the created Folder
    // identity comes back with the committed result.
    expect(saved.createdFolder?.id).toBe('lfolder_folder0009')
    expect(outcomes).toEqual([
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'image-1',
        assetKey: 'unsplash:image-1',
        affectedFolderKeys: ['all', 'lfolder_folder0009'],
        galleryEffect: 'reload-current',
      },
    ])
  })

  it('reports the filed target from the stored asset for existing folders', async () => {
    mocks.saveAsset.mockResolvedValue(savedResult({ folderId: 'lfolder_folder0002' }))
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })
    const outcomes: unknown[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome))

    await coordinator.saveReference(reference, 'lfolder_folder0002')

    expect(outcomes).toEqual([
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'image-1',
        assetKey: 'unsplash:image-1',
        affectedFolderKeys: ['all', 'lfolder_folder0002'],
        galleryEffect: 'reload-current',
      },
    ])
  })

  it('keeps batch partial success and reports each stored target', async () => {
    mocks.saveAsset
      .mockResolvedValueOnce(savedResult({ sourceId: 'image-1' }))
      .mockRejectedValueOnce(new Error('save failed'))
      .mockResolvedValueOnce(savedResult({ sourceId: 'kept-2', folderId: undefined }))
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })
    const outcomes: unknown[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome))

    const second = { ...reference, id: 'kept-2' }
    const results = await coordinator.saveReferences([
      reference,
      { ...reference, id: 'boom' },
      second,
    ])

    expect(results.map((result) => result.status)).toEqual(['fulfilled', 'rejected', 'fulfilled'])
    expect(outcomes).toHaveLength(2)
    // Each outcome reports its actually stored asset — never the batch input.
    expect(outcomes).toEqual([
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'image-1',
        assetKey: 'unsplash:image-1',
        affectedFolderKeys: ['all'],
        galleryEffect: 'reload-current',
      },
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'kept-2',
        assetKey: 'unsplash:kept-2',
        affectedFolderKeys: ['all'],
        galleryEffect: 'reload-current',
      },
    ])
  })

  it('reports the deleted asset folders and still marks an unknown reference', async () => {
    mocks.deleteAsset
      .mockResolvedValueOnce({
        asset: { provider: 'unsplash', sourceId: 'filed', folderId: 'lfolder_folder0003' },
        assetKey: 'unsplash:filed',
        affectedFolderKeys: ['all', 'lfolder_folder0003'],
      })
      .mockResolvedValueOnce(null)
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })
    const outcomes: unknown[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome))

    await coordinator.removeReference('unsplash', 'filed')
    const missing = await coordinator.removeReference('unsplash', 'ghost')

    expect(missing.asset).toBeNull()
    expect(outcomes).toEqual([
      {
        kind: 'asset-removed',
        provider: 'unsplash',
        sourceID: 'filed',
        assetKey: 'unsplash:filed',
        affectedFolderKeys: ['all', 'lfolder_folder0003'],
        galleryEffect: 'reload-current',
      },
      {
        kind: 'asset-removed',
        provider: 'unsplash',
        sourceID: 'ghost',
        assetKey: 'unsplash:ghost',
        affectedFolderKeys: ['all'],
        galleryEffect: 'reload-current',
      },
    ])
  })

  it('publishes successful save and remove outcomes when the Folder refresh fails', async () => {
    mocks.listFolders.mockRejectedValue(new Error('Folder refresh unavailable'))
    mocks.saveAsset.mockResolvedValue(savedResult())
    mocks.deleteAsset.mockResolvedValue({
      asset: { provider: 'unsplash', sourceId: 'image-1', folderId: undefined },
      assetKey: 'unsplash:image-1',
      affectedFolderKeys: ['all'],
    })
    const coordinator = createSearchLibraryCoordinator({ workspaceId: WORKSPACE_ID })
    const outcomes: string[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome.kind))

    await coordinator.saveReference(reference)
    await coordinator.removeReference('unsplash', 'image-1')

    expect(outcomes).toEqual(['asset-saved', 'asset-removed'])
  })

  it('fails closed without a workspace', async () => {
    const coordinator = createSearchLibraryCoordinator({ workspaceId: null })
    await expect(coordinator.saveReference(reference)).rejects.toThrow()
    await expect(coordinator.removeReference('unsplash', 'image-1')).rejects.toThrow()
    expect(mocks.saveAsset).not.toHaveBeenCalled()
    expect(mocks.deleteAsset).not.toHaveBeenCalled()
  })
})
