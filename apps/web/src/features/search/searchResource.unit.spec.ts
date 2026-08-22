import { beforeEach, describe, expect, it, vi } from 'vitest'
import { isSearchQueryWithinLimit, normalizeSearchQuery, searchInspiration } from './searchResource'

const generated = vi.hoisted(() => ({
  search: vi.fn(),
}))

vi.mock('../../shared/api/generated-query/user/user', () => ({
  searchUserInspiration: generated.search,
}))

describe('searchResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('normalizes empty and whitespace-heavy queries without rewriting intent', () => {
    expect(normalizeSearchQuery('  warm\t shadows\n')).toBe('warm shadows')
    expect(normalizeSearchQuery('   ')).toBe('')
    expect(isSearchQueryWithinLimit('你'.repeat(200))).toBe(true)
    expect(isSearchQueryWithinLimit('你'.repeat(201))).toBe(false)
  })

  it('dedupes identical in-flight query pages', async () => {
    let resolveRequest: ((value: unknown) => void) | undefined
    generated.search.mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )

    const first = searchInspiration({ query: 'warm shadows', cursor: 'cursor-a', limit: 24 })
    const second = searchInspiration({ query: 'warm shadows', cursor: 'cursor-a', limit: 24 })
    expect(generated.search).toHaveBeenCalledOnce()

    resolveRequest?.({
      code: 0,
      data: {
        query: 'warm shadows',
        results: [],
        has_more: false,
      },
    })
    await expect(Promise.all([first, second])).resolves.toHaveLength(2)
  })

  it('rejects an already-aborted request before touching the API', async () => {
    const controller = new AbortController()
    controller.abort()

    await expect(searchInspiration({ query: '', signal: controller.signal })).rejects.toMatchObject(
      {
        name: 'AbortError',
      },
    )
    expect(generated.search).not.toHaveBeenCalled()
  })

  it('passes the normalized query through the feature adapter', async () => {
    generated.search.mockResolvedValue({
      code: 0,
      data: { query: 'warm cafe', results: [], has_more: false },
    })

    await searchInspiration({
      query: 'warm cafe',
    })

    expect(generated.search).toHaveBeenCalledWith(
      expect.objectContaining({
        q: 'warm cafe',
      }),
      expect.anything(),
    )
  })
})
