import { beforeEach, describe, expect, it, vi } from 'vitest'
import { libraryAssetKey, toLibraryAssetInput } from './libraryResource'

describe('libraryResource intent adapter', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps a normalized Search reference without exposing transport field assembly to callers', () => {
    expect(
      toLibraryAssetInput({
        provider: 'unsplash',
        id: 'photo-1',
        source_url: 'https://unsplash.com/photos/photo-1',
        preview_url: 'https://images.unsplash.com/photo-1?w=800',
        original_url: 'https://images.unsplash.com/photo-1',
        title: 'Warm light',
        author: 'A. Creator',
        width: 1200,
        height: 800,
      }),
    ).toEqual({
      provider: 'unsplash',
      source_id: 'photo-1',
      source_url: 'https://unsplash.com/photos/photo-1',
      preview_url: 'https://images.unsplash.com/photo-1?w=800',
      original_url: 'https://images.unsplash.com/photo-1',
      title: 'Warm light',
      author: 'A. Creator',
      width: 1200,
      height: 800,
    })
  })

  it('uses one saved identity key format for lookup and UI state', () => {
    expect(libraryAssetKey('pexels', '123')).toBe('pexels:123')
  })
})
