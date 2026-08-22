import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createInspirationFeed, type InspirationFeedDependencies } from './inspirationFeed'
import type { SearchResult } from './searchResource'

const result = (id: string): SearchResult => ({
  provider: 'unsplash',
  id,
  preview_url: `https://images.example/${id}.jpg`,
  source_url: `https://example.com/${id}`,
  original_url: `https://images.example/${id}-original.jpg`,
  title: id,
  author: 'IdeaBoard',
  width: 100,
  height: 100,
  media_kind: 'image',
})

function dependencies(overrides: Partial<InspirationFeedDependencies> = {}): InspirationFeedDependencies {
  return {
    randomCursor: () => 'random-cursor',
    search: vi.fn(),
    lookupSavedKeys: vi.fn(async () => []),
    getCache: vi.fn(() => null),
    setCache: vi.fn(),
    restoreScroll: vi.fn(),
    ...overrides,
  }
}

function createFeed(deps: InspirationFeedDependencies, query = 'cafe') {
  return createInspirationFeed(
    {
      submittedQuery: ref(query),
      keepResultsOnNextLoad: ref(false),
      cacheScope: 'user-1',
    },
    deps,
  )
}

describe('createInspirationFeed', () => {
  it('hydrates a matching cache before refreshing it from the provider', async () => {
    const cached = result('cached')
    const fresh = result('fresh')
    const deps = dependencies({
      getCache: vi.fn(() => ({ results: [cached], nextCursor: 'next', hasMore: true, scrollY: 420 })),
      search: vi.fn(async () => ({ results: [fresh], next_cursor: '', has_more: false })),
    })
    const feed = createFeed(deps)

    await feed.load(true)

    expect(deps.restoreScroll).toHaveBeenCalledWith(420)
    expect(feed.results.value).toEqual([fresh])
    expect(deps.setCache).toHaveBeenCalledWith({
      query: 'cafe',
      results: [fresh],
      nextCursor: '',
      hasMore: false,
      scope: 'user-1',
    })
  })

  it('ignores stale responses from a superseded reset', async () => {
    let resolveFirst!: (value: { results: SearchResult[]; next_cursor: string; has_more: boolean }) => void
    const search = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve
          }),
      )
      .mockResolvedValueOnce({ results: [result('current')], next_cursor: '', has_more: false })
    const feed = createFeed(dependencies({ search }))

    const first = feed.load(true)
    await feed.load(true)
    resolveFirst({ results: [result('stale')], next_cursor: '', has_more: false })
    await first

    expect(feed.results.value).toEqual([result('current')])
  })

  it('uses the next cursor only while another page is available', async () => {
    const search = vi
      .fn()
      .mockResolvedValueOnce({ results: [result('first')], next_cursor: 'page-2', has_more: true })
      .mockResolvedValueOnce({ results: [result('second')], next_cursor: '', has_more: false })
    const feed = createFeed(dependencies({ search }))

    await feed.load(true)
    await feed.load(false)
    await feed.load(false)

    expect(search).toHaveBeenCalledTimes(2)
    expect(search.mock.calls[1]?.[0]).toMatchObject({ cursor: 'page-2' })
    expect(feed.results.value.map((item) => item.id)).toEqual(['first', 'second'])
  })
})
