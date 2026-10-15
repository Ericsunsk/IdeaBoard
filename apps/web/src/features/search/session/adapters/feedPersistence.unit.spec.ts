import { mediaDisplayFixture } from '../../../../test/mediaDisplay'
import { beforeEach, describe, expect, it } from 'vitest'
import { createBrowserSearchFeedPersistence } from './feedPersistence'
import type { SearchResult } from '../../core/models'

const result: SearchResult = {
  provider: 'unsplash',
  id: 'photo-1',
  title: 'Neon city',
  author: 'IdeaBoard',
  author_url: 'https://example.com/author',
  thumbnail_url: 'https://images.example/thumb-400.jpg',
  preview_url: 'https://images.example/thumb.jpg',
  original_url: 'https://images.example/original.jpg',
  source_url: 'https://example.com/photo-1',
  width: 1200,
  height: 800,
  media_kind: 'image',
  display: mediaDisplayFixture(),
}

describe('Browser Search feed persistence', () => {
  beforeEach(() => sessionStorage.clear())

  it('stores an identity-scoped feed snapshot and its query scroll position', () => {
    const persistence = createBrowserSearchFeedPersistence()
    persistence.write(
      { query: 'cyberpunk', results: [result], nextCursor: 'page-2', hasMore: true },
      'user-a',
    )
    persistence.writeScroll(350, 'cyberpunk', 'user-a')

    expect(persistence.read('cyberpunk', 'user-a', false)).toEqual({
      query: 'cyberpunk',
      results: [result],
      nextCursor: 'page-2',
      hasMore: true,
      scrollY: 350,
    })
    expect(persistence.read('cyberpunk', 'user-b', false)).toBeNull()
  })
})

describe('Search display snapshot expiry', () => {
  it('rejects expired capabilities even when snapshot timestamp is fresh', () => {
    const persistence = createBrowserSearchFeedPersistence()
    persistence.write(
      {
        query: 'expired',
        results: [
          {
            ...result,
            display: {
              ...mediaDisplayFixture(),
              expires_at: new Date(Date.now() - 1).toISOString(),
            },
          },
        ],
        nextCursor: 'next',
        hasMore: true,
      },
      'user-a',
    )
    expect(persistence.read('expired', 'user-a', false)).toBeNull()
  })

  it('rejects malformed presentation descriptors instead of trusting cached provider fallbacks', () => {
    const persistence = createBrowserSearchFeedPersistence()
    const display = { ...mediaDisplayFixture(), expires_at: 'invalid' }
    persistence.write(
      { query: 'malformed', results: [{ ...result, display }], nextCursor: '', hasMore: false },
      'user-a',
    )
    expect(persistence.read('malformed', 'user-a', false)).toBeNull()
  })
})
