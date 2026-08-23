import { beforeEach, describe, expect, it } from 'vitest'
import {
  allowAnonymousSearchQuery,
  clearPendingSearchAuthAction,
  consumePendingSearchAuthAction,
  MAX_ANONYMOUS_SEARCH_QUERIES,
  peekPendingSearchAuthAction,
  setPendingSearchAuthAction,
} from './searchAuthContinuation'

describe('search auth continuation', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
    clearPendingSearchAuthAction()
  })

  it('allows five unique normalized queries and gates the sixth', () => {
    for (let index = 0; index < MAX_ANONYMOUS_SEARCH_QUERIES; index += 1) {
      expect(allowAnonymousSearchQuery(`  query   ${index} `)).toBe(true)
    }

    expect(allowAnonymousSearchQuery('query 0')).toBe(true)
    expect(allowAnonymousSearchQuery('query 5')).toBe(false)
  })

  it('does not count an empty query', () => {
    expect(allowAnonymousSearchQuery('   ')).toBe(true)
    expect(window.sessionStorage.getItem('ideaboard:search:anonymous-query-gate')).toBeNull()
  })

  it('consumes a pending action exactly once', () => {
    const action = { kind: 'text-search' as const, query: 'warm cafe' }
    setPendingSearchAuthAction(action)

    expect(peekPendingSearchAuthAction()).toEqual(action)
    expect(consumePendingSearchAuthAction()).toEqual(action)
    expect(consumePendingSearchAuthAction()).toBeNull()
  })

  it('restores a serializable action after an OAuth page reload boundary', () => {
    const action = { kind: 'open-library' as const }
    window.sessionStorage.setItem(
      'ideaboard:search:pending-auth-action',
      JSON.stringify(action),
    )

    expect(peekPendingSearchAuthAction()).toEqual(action)
    expect(consumePendingSearchAuthAction()).toEqual(action)
    expect(window.sessionStorage.getItem('ideaboard:search:pending-auth-action')).toBeNull()
  })
})
