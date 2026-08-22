import { describe, expect, it } from 'vitest'
import {
  getLibraryDropEffect,
  LIBRARY_ASSET_MIME,
  readLibraryAssetTransfer,
  readSearchReferenceTransfer,
  SEARCH_REFERENCE_MIME,
  writeLibraryAssetTransfer,
  writeSearchReferenceTransfer,
} from './libraryTransfer'
import type { LibraryAssetView, LibrarySearchReference } from './libraryResource'

function transfer(): DataTransfer {
  const values = new Map<string, string>()
  return {
    types: [],
    effectAllowed: 'all',
    dropEffect: 'none',
    setData(type: string, value: string) {
      values.set(type, value)
      ;(this.types as string[]).push(type)
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
  id: 42,
  provider: 'pexels',
  source_id: 'photo-42',
  source_url: 'https://pexels.com/photo/photo-42',
  preview_url: 'https://images.pexels.com/photo-42',
  original_url: 'https://images.pexels.com/photo-42',
  title: 'Blue shadow',
  author: 'B. Creator',
  width: 800,
  height: 800,
  folder_id: null,
  created_at: '2026-08-22T00:00:00Z',
}

describe('libraryTransfer', () => {
  it('round-trips Search references with copy semantics', () => {
    const dataTransfer = transfer()
    writeSearchReferenceTransfer(dataTransfer, reference)

    expect(dataTransfer.effectAllowed).toBe('copy')
    expect(dataTransfer.types).toContain(SEARCH_REFERENCE_MIME)
    expect(readSearchReferenceTransfer(dataTransfer)).toEqual(reference)
    expect(readLibraryAssetTransfer(dataTransfer)).toBeNull()
    expect(getLibraryDropEffect(dataTransfer)).toBe('copy')
  })

  it('round-trips Library assets with move semantics', () => {
    const dataTransfer = transfer()
    writeLibraryAssetTransfer(dataTransfer, asset)

    expect(dataTransfer.effectAllowed).toBe('move')
    expect(dataTransfer.types).toContain(LIBRARY_ASSET_MIME)
    expect(readLibraryAssetTransfer(dataTransfer)).toEqual(asset)
    expect(readSearchReferenceTransfer(dataTransfer)).toBeNull()
    expect(getLibraryDropEffect(dataTransfer)).toBe('move')
  })

  it('rejects malformed and unrelated payloads', () => {
    const dataTransfer = transfer()
    dataTransfer.setData(SEARCH_REFERENCE_MIME, '{"type":"search-reference","reference":{}}')
    expect(readSearchReferenceTransfer(dataTransfer)).toBeNull()

    const unrelated = transfer()
    unrelated.setData('text/plain', 'hello')
    expect(readSearchReferenceTransfer(unrelated)).toBeNull()
    expect(readLibraryAssetTransfer(unrelated)).toBeNull()
    expect(getLibraryDropEffect(unrelated)).toBe('none')
  })
})
