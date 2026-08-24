import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createSearchFeed, type SearchFeedDependencies } from './searchFeed'
import type { SearchFeedPersistence } from './searchFeedPersistence'
import type { SearchPage, SearchResult } from './searchResource'

const result = (id: string): SearchResult => ({
  provider: 'unsplash',
  id,
  preview_url: `https://images.example/${id}.jpg`,
  source_url: `https://example.com/${id}`,
  original_url: `https://images.example/${id}-original.jpg`,
  title: id,
  author: 'IdeaBoard',
  author_url: 'https://example.com/author',
  thumbnail_url: `https://images.example/${id}-thumb.jpg`,
  width: 100,
  height: 100,
  media_kind: 'image',
})

function persistence(overrides: Partial<SearchFeedPersistence> = {}): SearchFeedPersistence {
  return {
    read: vi.fn(() => null),
    write: vi.fn(),
    writeScroll: vi.fn(),
    scheduleWrite: (write) => {
      write()
      return () => {}
    },
    ...overrides,
  }
}

function dependencies(overrides: Partial<SearchFeedDependencies> = {}): SearchFeedDependencies {
  return {
    randomCursor: () => 'random-cursor',
    search: vi.fn(),
    lookupSavedKeys: vi.fn(async () => new Set<string>()),
    persistence: persistence(),
    restoreScroll: vi.fn(),
    reportImageFailure: vi.fn(),
    ...overrides,
  }
}

function page(results: SearchResult[], next_cursor = '', has_more = false): SearchPage {
  return { query: 'cafe', results, next_cursor, has_more }
}

function createFeed(deps: SearchFeedDependencies, query = 'cafe') {
  return createSearchFeed(
    {
      submittedQuery: ref(query),
      keepResultsOnNextLoad: ref(false),
      cacheScope: 'user-1',
    },
    deps,
  )
}

describe('createSearchFeed interface', () => {
  it('hydrates cached cards, then replaces them with one fresh saved-state lookup', async () => {
    const cached = result('cached')
    const fresh = result('fresh')
    const storage = persistence({
      read: vi.fn(() => ({
        results: [cached],
        nextCursor: 'next',
        hasMore: true,
        scrollY: 420,
        query: 'cafe',
      })),
    })
    const lookupSavedKeys = vi.fn(async () => new Set(['unsplash:fresh']))
    const deps = dependencies({
      persistence: storage,
      search: vi.fn(async () => page([fresh])),
      lookupSavedKeys,
    })
    const feed = createFeed(deps)

    await feed.load(true)
    await Promise.resolve()

    expect(deps.restoreScroll).toHaveBeenCalledWith(420)
    expect(feed.results.value).toEqual([fresh])
    expect(lookupSavedKeys).toHaveBeenCalledOnce()
    expect(lookupSavedKeys).toHaveBeenCalledWith([{ provider: 'unsplash', source_id: 'fresh' }])
    expect(feed.savedKeys.value).toEqual(new Set(['unsplash:fresh']))
    expect(storage.write).toHaveBeenCalledWith(
      {
        query: 'cafe',
        results: [fresh],
        nextCursor: '',
        hasMore: false,
      },
      'user-1',
    )
  })

  it('keeps one coherent cached page when fresh refresh fails, then appends from its cached cursor', async () => {
    const cached = result('cached')
    const lookupSavedKeys = vi.fn(async () => new Set(['unsplash:cached']))
    const search = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(page([result('cached-page-2')]))
    const feed = createFeed(
      dependencies({
        persistence: persistence({
          read: vi.fn(() => ({
            results: [cached],
            nextCursor: 'next',
            hasMore: true,
            scrollY: 0,
            query: 'cafe',
          })),
        }),
        search,
        lookupSavedKeys,
      }),
    )

    await feed.load(true)

    expect(feed.results.value).toEqual([cached])
    expect(feed.hasError.value).toBe(true)
    expect(feed.hasMore.value).toBe(true)
    expect(lookupSavedKeys).toHaveBeenCalledWith([{ provider: 'unsplash', source_id: 'cached' }])

    await feed.load(false)

    expect(search.mock.calls[1]?.[0]).toMatchObject({ cursor: 'next' })
    expect(feed.results.value.map((item) => item.id)).toEqual(['cached', 'cached-page-2'])
  })

  it('keeps an explicit unsave newer than an in-flight saved lookup', async () => {
    let resolveLookup!: (keys: Set<string>) => void
    const feed = createFeed(
      dependencies({
        search: vi.fn(async () => page([result('first')])),
        lookupSavedKeys: vi.fn(
          () =>
            new Promise<Set<string>>((resolve) => {
              resolveLookup = resolve
            }),
        ),
      }),
    )

    await feed.load(true)
    feed.markUnsavedKey('unsplash:first')
    resolveLookup(new Set(['unsplash:first']))
    await Promise.resolve()

    expect(feed.savedKeys.value.has('unsplash:first')).toBe(false)
  })

  it('drops a stale reset response after cancellation and only persists the current feed', async () => {
    let resolveFirst!: (page: SearchPage) => void
    const storage = persistence()
    const search = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve
          }),
      )
      .mockResolvedValueOnce(page([result('current')]))
    const feed = createFeed(dependencies({ search, persistence: storage }))

    const first = feed.load(true)
    await feed.load(true)
    resolveFirst(page([result('stale')]))
    await first

    expect(feed.results.value).toEqual([result('current')])
    expect(storage.write).toHaveBeenLastCalledWith(
      expect.objectContaining({ results: [result('current')] }),
      'user-1',
    )
  })

  it('appends exactly one cursor page and persists the complete snapshot', async () => {
    const storage = persistence()
    const search = vi
      .fn()
      .mockResolvedValueOnce(page([result('first')], 'page-2', true))
      .mockResolvedValueOnce(page([result('second')]))
    const feed = createFeed(dependencies({ search, persistence: storage }))

    await feed.load(true)
    await feed.load(false)
    await feed.load(false)

    expect(search).toHaveBeenCalledTimes(2)
    expect(search.mock.calls[1]?.[0]).toMatchObject({ cursor: 'page-2' })
    expect(feed.results.value.map((item) => item.id)).toEqual(['first', 'second'])
    expect(storage.write).toHaveBeenLastCalledWith(
      {
        query: 'cafe',
        results: [result('first'), result('second')],
        nextCursor: '',
        hasMore: false,
      },
      'user-1',
    )
  })

  it('cancels a deferred persistence snapshot when the feed is disposed', async () => {
    let scheduledWrite: (() => void) | undefined
    const storage = persistence({
      scheduleWrite: (write) => {
        scheduledWrite = write
        return () => {}
      },
    })
    const feed = createFeed(
      dependencies({
        persistence: storage,
        search: vi.fn(async () => page([result('first')])),
      }),
    )

    await feed.load(true)
    feed.dispose()
    scheduledWrite?.()

    expect(storage.write).not.toHaveBeenCalled()
  })

  it('persists scroll to the query that produced it, even if the route query changes before debounce', () => {
    vi.useFakeTimers()
    try {
      const submittedQuery = ref('old-query')
      const storage = persistence()
      const feed = createSearchFeed(
        {
          submittedQuery,
          keepResultsOnNextLoad: ref(false),
          cacheScope: 'user-1',
        },
        dependencies({ persistence: storage }),
      )

      feed.recordScroll(360)
      submittedQuery.value = 'new-query'
      vi.advanceTimersByTime(150)

      expect(storage.writeScroll).toHaveBeenCalledWith(360, 'old-query', 'user-1')
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not request protected saved facts for anonymous Search', async () => {
    const lookupSavedKeys = vi.fn(async () => new Set<string>())
    const feed = createSearchFeed(
      {
        submittedQuery: ref('cafe'),
        keepResultsOnNextLoad: ref(false),
        cacheScope: 'anonymous',
        isAuthenticated: () => false,
      },
      dependencies({
        lookupSavedKeys,
        search: vi.fn(async () => page([result('public')])),
      }),
    )

    await feed.load(true)

    expect(lookupSavedKeys).not.toHaveBeenCalled()
  })
})
