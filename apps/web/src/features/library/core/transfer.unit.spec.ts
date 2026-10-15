import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { describe, expect, it } from 'vitest'
import { toCanvasLibraryImportDescriptor, type LibraryAssetView } from './transfer'

const asset: LibraryAssetView = {
  id: 'lasset_asset0042',
  workspaceId: 'ws_test123',
  provider: 'pexels',
  sourceId: 'photo-42',
  sourceUrl: 'https://pexels.com/photo/photo-42',
  previewUrl: 'https://images.pexels.com/photo-42',
  originalUrl: 'https://images.pexels.com/photo-42',
  title: 'Blue shadow',
  author: 'B. Creator',
  width: 800,
  height: 800,
  folderId: 'lfolder_folder0001',
  createdAt: '2026-08-22T00:00:00Z',
  display: mediaDisplayFixture(),
  updatedAt: '2026-08-22T00:00:00Z',
}

describe('Canvas import descriptor', () => {
  it('keeps only the fields needed to import the Library asset', () => {
    expect(toCanvasLibraryImportDescriptor(asset)).toEqual({
      assetId: asset.id,
      title: asset.title,
      previewUrl: '/api/media/previews/fixture/preview',
      width: asset.width,
      height: asset.height,
    })
  })
})
