import { describe, expect, it } from 'vitest'
import {
  CANVAS_LIBRARY_IMPORT_MIME,
  getLibraryDropEffect,
  LIBRARY_FOLDER_MOVE_MIME,
  readCanvasLibraryImportTransfer,
  readLibraryFolderMoveTransfer,
  readSearchReferenceTransfer,
  SEARCH_REFERENCE_MIME,
  toCanvasLibraryImportDescriptor,
  writeCanvasLibraryImportTransfer,
  writeLibraryFolderMoveTransfer,
  writeSearchReferenceTransfer,
} from './transfer'
import type { LibrarySearchReference } from './validators'
import type { LibraryAssetView } from './transfer'

function transfer(): DataTransfer {
  const values = new Map<string, string>()
  const types: string[] = []
  return {
    types,
    effectAllowed: 'all',
    dropEffect: 'none',
    setData(type: string, value: string) {
      values.set(type, value)
      types.push(type)
    },
    getData(type: string) {
      return values.get(type) ?? ''
    },
  } as unknown as DataTransfer
}

const reference: LibrarySearchReference = {
  provider: 'unsplash',
  id: 'photo-1',
  source_url: 'https://unsplash.com/photos/photo-1',
  preview_url: 'https://images.unsplash.com/photo-1?w=800',
  original_url: 'https://images.unsplash.com/photo-1',
  title: 'Warm light',
  author: 'A. Creator',
  width: 1200,
  height: 800,
}

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
  updatedAt: '2026-08-22T00:00:00Z',
}

describe('transfer', () => {
  it('round-trips Search references with copy semantics', () => {
    const dataTransfer = transfer()
    writeSearchReferenceTransfer(dataTransfer, reference)

    expect(dataTransfer.effectAllowed).toBe('copy')
    expect(dataTransfer.types).toContain(SEARCH_REFERENCE_MIME)
    expect(readSearchReferenceTransfer(dataTransfer)).toEqual(reference)
    expect(readLibraryFolderMoveTransfer(dataTransfer)).toBeNull()
    expect(readCanvasLibraryImportTransfer(dataTransfer)).toBeNull()
    expect(getLibraryDropEffect(dataTransfer)).toBe('copy')
  })

  it('uses a minimal descriptor for Canvas imports', () => {
    const dataTransfer = transfer()
    const descriptor = toCanvasLibraryImportDescriptor(asset)
    writeCanvasLibraryImportTransfer(dataTransfer, descriptor)

    expect(dataTransfer.effectAllowed).toBe('copyMove')
    expect(dataTransfer.types).toContain(CANVAS_LIBRARY_IMPORT_MIME)
    expect(readCanvasLibraryImportTransfer(dataTransfer)).toEqual(descriptor)
    expect(readSearchReferenceTransfer(dataTransfer)).toBeNull()
    expect(JSON.parse(dataTransfer.getData(CANVAS_LIBRARY_IMPORT_MIME))).toEqual({
      type: 'canvas-library-import',
      asset: {
        assetId: asset.id,
        title: asset.title,
        previewUrl: asset.previewUrl,
        width: asset.width,
        height: asset.height,
      },
    })
  })

  it('uses only an asset ID for Library folder moves', () => {
    const dataTransfer = transfer()
    writeLibraryFolderMoveTransfer(dataTransfer, asset.id)

    expect(dataTransfer.effectAllowed).toBe('copyMove')
    expect(dataTransfer.types).toContain(LIBRARY_FOLDER_MOVE_MIME)
    expect(readLibraryFolderMoveTransfer(dataTransfer)).toBe(asset.id)
    expect(getLibraryDropEffect(dataTransfer)).toBe('move')
  })

  it('rejects malformed and unrelated payloads', () => {
    const dataTransfer = transfer()
    dataTransfer.setData(SEARCH_REFERENCE_MIME, '{"type":"search-reference","reference":{}}')
    expect(readSearchReferenceTransfer(dataTransfer)).toBeNull()

    const unrelated = transfer()
    unrelated.setData('text/plain', 'hello')
    expect(readSearchReferenceTransfer(unrelated)).toBeNull()
    expect(readLibraryFolderMoveTransfer(unrelated)).toBeNull()
    expect(readCanvasLibraryImportTransfer(unrelated)).toBeNull()
    expect(getLibraryDropEffect(unrelated)).toBe('none')

    const invalidFolderMove = transfer()
    invalidFolderMove.setData(
      LIBRARY_FOLDER_MOVE_MIME,
      JSON.stringify({ type: 'library-folder-move', assetId: '7' }),
    )
    expect(readLibraryFolderMoveTransfer(invalidFolderMove)).toBeNull()

    const legacyNumericMove = transfer()
    legacyNumericMove.setData(
      LIBRARY_FOLDER_MOVE_MIME,
      JSON.stringify({ type: 'library-folder-move', assetId: 7 }),
    )
    expect(readLibraryFolderMoveTransfer(legacyNumericMove)).toBeNull()

    const malformedCanvasImport = transfer()
    malformedCanvasImport.setData(
      CANVAS_LIBRARY_IMPORT_MIME,
      JSON.stringify({
        type: 'canvas-library-import',
        asset: { assetId: asset.id, title: asset.title, width: asset.width, height: asset.height },
      }),
    )
    expect(readCanvasLibraryImportTransfer(malformedCanvasImport)).toBeNull()
  })
})
