import { describe, expect, it } from 'vitest'
import { CHUNK_RELOAD_GUARD_KEY, isChunkLoadFailure, resolveScrollPosition } from './routeRecovery'

describe('resolveScrollPosition', () => {
  it('restores the saved position on back/forward navigation', () => {
    expect(resolveScrollPosition({ path: '/a' }, { path: '/b' }, { left: 0, top: 240 })).toEqual({
      left: 0,
      top: 240,
    })
  })

  it('starts at the top when the path changes', () => {
    expect(resolveScrollPosition({ path: '/b' }, { path: '/a' }, null)).toEqual({ top: 0 })
  })

  it('keeps the scroll position for same-path navigations such as search queries', () => {
    expect(resolveScrollPosition({ path: '/search' }, { path: '/search' }, null)).toBe(false)
  })
})

describe('isChunkLoadFailure', () => {
  it.each([
    'Failed to fetch dynamically imported module: https://app/assets/Page-abc.js',
    'Importing a module script failed.',
    'Loading chunk 42 failed.',
  ])('recognizes %s as a stale-manifest chunk failure', (message) => {
    expect(isChunkLoadFailure(new Error(message))).toBe(true)
  })

  it.each([
    new Error('Network Error'),
    new Error('Request failed with status code 500'),
    'plain string failure',
    '',
    null,
    undefined,
  ])('does not treat %s as a chunk failure', (message) => {
    expect(isChunkLoadFailure(message)).toBe(false)
  })

  it('exposes a stable session guard key for the single-reload recovery', () => {
    expect(CHUNK_RELOAD_GUARD_KEY).toBe('ideaboard:chunk-reload')
  })
})
