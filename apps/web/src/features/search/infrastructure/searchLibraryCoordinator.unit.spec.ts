import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSearchLibraryCoordinator } from './searchLibraryCoordinator'

const mocks = vi.hoisted(() => ({
  getLibraryFolders: vi.fn(),
  saveSearchReference: vi.fn(),
  removeSearchReference: vi.fn(),
}))

vi.mock('../../library/libraryResource', () => ({
  getLibraryFolders: mocks.getLibraryFolders,
  saveSearchReference: mocks.saveSearchReference,
  removeSearchReference: mocks.removeSearchReference,
  libraryAssetKey: (provider: string, sourceID: string) => `${provider}:${sourceID}`,
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

describe('Search Library coordinator', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getLibraryFolders.mockResolvedValue([])
  })

  it('refreshes the shared Folder facts after save and remove mutations', async () => {
    mocks.saveSearchReference.mockResolvedValue({
      id: 1,
      provider: 'unsplash',
      source_id: 'image-1',
    })
    const coordinator = createSearchLibraryCoordinator()
    const outcomes: unknown[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome))

    await coordinator.saveReference(reference)
    await coordinator.removeReference('unsplash', 'image-1')

    expect(mocks.saveSearchReference).toHaveBeenCalledWith(reference, undefined, undefined)
    expect(mocks.removeSearchReference).toHaveBeenCalledWith('unsplash', 'image-1')
    expect(mocks.getLibraryFolders).toHaveBeenCalledTimes(2)
    expect(outcomes).toEqual([
      {
        kind: 'asset-saved',
        provider: 'unsplash',
        sourceID: 'image-1',
        assetKey: 'unsplash:image-1',
        affectedFolderKeys: ['all', 'quick-saves'],
        galleryEffect: 'reload-current',
      },
      {
        kind: 'asset-removed',
        provider: 'unsplash',
        sourceID: 'image-1',
        assetKey: 'unsplash:image-1',
        affectedFolderKeys: ['all', 'quick-saves'],
        galleryEffect: 'reload-current',
      },
    ])
  })

  it('publishes successful save and remove outcomes when the Folder refresh fails', async () => {
    mocks.getLibraryFolders.mockRejectedValue(new Error('Folder refresh unavailable'))
    mocks.saveSearchReference.mockResolvedValue({
      id: 1,
      provider: 'unsplash',
      source_id: 'image-1',
    })
    const coordinator = createSearchLibraryCoordinator()
    const outcomes: string[] = []
    coordinator.subscribe((outcome) => outcomes.push(outcome.kind))

    await coordinator.saveReference(reference)
    await coordinator.removeReference('unsplash', 'image-1')

    expect(outcomes).toEqual(['asset-saved', 'asset-removed'])
  })
})
