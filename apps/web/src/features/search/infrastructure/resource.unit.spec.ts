import { beforeEach, describe, expect, it, vi } from 'vitest'
import { isSearchQueryWithinLimit, normalizeSearchQuery } from '../domain/queryPolicy'
import { searchInspiration } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../shared/api/client', () => ({
  api: { request: transport.request },
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
    transport.request.mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )

    const first = searchInspiration({ query: 'warm shadows', cursor: 'cursor-a', limit: 24 })
    const second = searchInspiration({ query: 'warm shadows', cursor: 'cursor-a', limit: 24 })
    expect(transport.request).toHaveBeenCalledOnce()

    resolveRequest?.({
      data: {
        query: 'warm shadows',
        results: [],
        has_more: false,
      },
    })
    await expect(Promise.all([first, second])).resolves.toHaveLength(2)
  })

  it('surfaces the contract error message when the envelope carries one', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'search_failed', message: 'Search down' },
    })

    await expect(searchInspiration({ query: 'failing query' })).rejects.toThrow('Search down')
  })

  it('rejects an already-aborted request before touching the API', async () => {
    const controller = new AbortController()
    controller.abort()

    await expect(searchInspiration({ query: '', signal: controller.signal })).rejects.toMatchObject(
      {
        name: 'AbortError',
      },
    )
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('passes the normalized query through the feature adapter', async () => {
    transport.request.mockResolvedValue({
      data: { query: 'warm cafe', results: [], has_more: false },
    })

    await searchInspiration({
      query: 'warm cafe',
    })

    expect(transport.request).toHaveBeenCalledWith(
      expect.stringContaining('/search?'),
      expect.anything(),
    )
    const url = String(transport.request.mock.calls[0]?.[0] ?? '')
    expect(url).toContain('q=warm+cafe')
  })
})
