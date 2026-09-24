import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSearchLibraryCoordinator } from './libraryCoordinator'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => ({
  listFolders: vi.fn(),
  saveAsset: vi.fn(),
  resolveAssets: vi.fn(),
  deleteAsset: vi.fn(),
}))

vi.mock('../../library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../library')>()
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

describe('Search Library coordinator', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listFolders.mockResolvedValue([])
  })

  it('refreshes the shared Folder facts after save and remove mutations', async () => {
    mocks.saveAsset.mockResolvedValue({
      id: 'liba_asset0001',
      provider: 'unsplash',
      sourceId: 'image-1',
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

  it('publishes successful save and remove outcomes when the Folder refresh fails', async () => {
    mocks.listFolders.mockRejectedValue(new Error('Folder refresh unavailable'))
    mocks.saveAsset.mockResolvedValue({
      id: 'liba_asset0001',
      provider: 'unsplash',
      sourceId: 'image-1',
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
