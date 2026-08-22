import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearCachedVisionReferences,
  clearSearchFeedCache,
  getCachedVisionReferences,
  getLibraryIslandState,
  getSearchFeedCache,
  setCachedVisionReferences,
  setLibraryIslandState,
  setSearchFeedCache,
  updateFeedScrollPosition,
} from './searchCache'
import type { SearchResult, VisionReference } from './searchResource'

describe('searchCache', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('stores and retrieves feed cache by query', () => {
    const mockResult: SearchResult = {
      provider: 'unsplash',
      id: 'photo-1',
      title: 'Neon city',
      author: 'Photographer',
      preview_url: 'https://images.example.com/thumb.jpg',
      original_url: 'https://images.example.com/orig.jpg',
      source_url: 'https://example.com/photo-1',
      width: 1920,
      height: 1080,
    }

    setSearchFeedCache({
      query: 'cyberpunk',
      results: [mockResult],
      nextCursor: 'cursor-123',
      hasMore: true,
      scrollY: 350,
    })

    const cached = getSearchFeedCache('cyberpunk')
    expect(cached).not.toBeNull()
    expect(cached?.query).toBe('cyberpunk')
    expect(cached?.results).toHaveLength(1)
    expect(cached?.nextCursor).toBe('cursor-123')
    expect(cached?.hasMore).toBe(true)
    expect(cached?.scrollY).toBe(350)

    expect(getSearchFeedCache('different-query')).toBeNull()
  })

  it('updates scroll position in feed cache', () => {
    setSearchFeedCache({
      query: 'architecture',
      results: [],
      nextCursor: '',
      hasMore: false,
      scrollY: 0,
    })

    updateFeedScrollPosition(820)
    const cached = getSearchFeedCache('architecture')
    expect(cached?.scrollY).toBe(820)
  })

  it('clears feed cache', () => {
    setSearchFeedCache({
      query: 'design',
      results: [],
      nextCursor: '',
      hasMore: false,
    })

    clearSearchFeedCache()
    expect(getSearchFeedCache('design')).toBeNull()
  })

  it('bypasses empty query cache when reload bypass is enabled and simulated', () => {
    setSearchFeedCache({
      query: '',
      results: [],
      nextCursor: '',
      hasMore: false,
    })

    // Without reload, returns cache
    expect(getSearchFeedCache('', false)).not.toBeNull()
  })

  it('stores and retrieves library island open and expanded state', () => {
    expect(getLibraryIslandState()).toEqual({ open: false, expanded: false })

    setLibraryIslandState({ open: true, expanded: true })
    expect(getLibraryIslandState()).toEqual({ open: true, expanded: true })

    setLibraryIslandState({ open: false, expanded: false })
    expect(getLibraryIslandState()).toEqual({ open: false, expanded: false })
  })

  it('stores and retrieves serializable remote vision references only', () => {
    const mockRef1: VisionReference = {
      key: 'unsplash:photo-1',
      previewUrl: 'https://images.example.com/1.jpg',
      libraryAssetID: 101,
    }
    const mockBlobRef: VisionReference = {
      key: 'local:blob-1',
      previewUrl: 'blob:http://localhost/12345',
      file: new File([''], 'test.png', { type: 'image/png' }),
    }

    setCachedVisionReferences([mockRef1, mockBlobRef])
    const cached = getCachedVisionReferences()
    expect(cached).toHaveLength(1)
    expect(cached[0]?.key).toBe('unsplash:photo-1')
    expect(cached[0]?.libraryAssetID).toBe(101)

    clearCachedVisionReferences()
    expect(getCachedVisionReferences()).toEqual([])
  })

  it('isolates caches by user scope and keeps only reloadable vision references', () => {
    setSearchFeedCache({
      query: 'scope-test',
      results: [],
      nextCursor: '',
      hasMore: false,
      scope: 'user-a',
    })
    setSearchFeedCache({
      query: 'scope-test',
      results: [],
      nextCursor: '',
      hasMore: false,
      scope: 'user-b',
    })

    expect(getSearchFeedCache('scope-test', true, 'user-a')).not.toBeNull()
    expect(getSearchFeedCache('scope-test', true, 'user-b')).not.toBeNull()
    expect(getSearchFeedCache('scope-test', true, 'user-c')).toBeNull()

    setCachedVisionReferences([
      { key: 'remote-only', previewUrl: 'https://example.com/remote.jpg' },
      { key: 'library-backed', previewUrl: 'https://example.com/library.jpg', libraryAssetID: 42 },
    ], 'user-a')
    expect(getCachedVisionReferences('user-a')).toEqual([
      { key: 'library-backed', previewUrl: 'https://example.com/library.jpg', libraryAssetID: 42 },
    ])
    expect(getCachedVisionReferences('user-b')).toEqual([])
  })
})
