import type { SearchFeedSnapshot, SearchFeedPersistence } from '../ports/persistencePort'
import { searchScopedStorageKey } from './browserPersistence'
import {
  readSessionStorageItem,
  removeSessionStorageItem,
  writeSessionStorageItem,
} from '../../../core/utils/storage'

const FEED_CACHE_KEY = 'ideaboard:search:feed_cache'
const FEED_SCROLL_KEY = 'ideaboard:search:feed_scroll'
const MAX_CACHE_AGE_MS = 30 * 60 * 1000

type PersistedSnapshot = Omit<SearchFeedSnapshot, 'scrollY'> & { timestamp: number }

function scrollKey(scope: string, query: string): string {
  return `${searchScopedStorageKey(FEED_SCROLL_KEY, scope)}:${encodeURIComponent(query)}`
}

function browserWasReloaded(): boolean {
  if (typeof window === 'undefined' || !window.performance) return false
  try {
    const entries = window.performance.getEntriesByType('navigation')
    if (entries.length > 0) return (entries[0] as PerformanceNavigationTiming).type === 'reload'
    return (
      (window.performance as unknown as { navigation?: { type?: number } }).navigation?.type === 1
    )
  } catch {
    return false
  }
}

/** Browser-only adapter. Feed state never reaches sessionStorage except through this seam. */
export function createBrowserSearchFeedPersistence(): SearchFeedPersistence {
  return {
    read(query, scope, allowReloadBypass) {
      const cacheKey = searchScopedStorageKey(FEED_CACHE_KEY, scope)
      if (query === '' && allowReloadBypass && browserWasReloaded()) {
        removeSessionStorageItem(cacheKey)
        return null
      }
      const raw = readSessionStorageItem(cacheKey)
      if (!raw) return null
      try {
        const snapshot = JSON.parse(raw) as PersistedSnapshot
        if (snapshot.query !== query || Date.now() - snapshot.timestamp > MAX_CACHE_AGE_MS) {
          if (Date.now() - snapshot.timestamp > MAX_CACHE_AGE_MS) removeSessionStorageItem(cacheKey)
          return null
        }
        const scrollY = Number(readSessionStorageItem(scrollKey(scope, query)))
        return {
          query: snapshot.query,
          results: snapshot.results,
          nextCursor: snapshot.nextCursor,
          hasMore: snapshot.hasMore,
          scrollY: Number.isFinite(scrollY) && scrollY >= 0 ? scrollY : 0,
        }
      } catch {
        return null
      }
    },
    write(snapshot, scope) {
      try {
        writeSessionStorageItem(
          searchScopedStorageKey(FEED_CACHE_KEY, scope),
          JSON.stringify({ ...snapshot, timestamp: Date.now() }),
        )
      } catch {
        // Storage can be unavailable in private browsing or over quota.
      }
    },
    writeScroll(scrollY, query, scope) {
      if (!Number.isFinite(scrollY) || scrollY < 0) return
      try {
        writeSessionStorageItem(scrollKey(scope, query), String(Math.round(scrollY)))
      } catch {
        // Scroll restoration is opportunistic.
      }
    },
    scheduleWrite(write) {
      if (typeof window === 'undefined') return () => {}
      let cancelled = false
      const run = () => {
        if (!cancelled) write()
      }
      const idleWindow = window as Window & {
        cancelIdleCallback?: (handle: number) => void
        requestIdleCallback?: (
          callback: IdleRequestCallback,
          options?: IdleRequestOptions,
        ) => number
      }
      if (typeof idleWindow.requestIdleCallback === 'function') {
        const handle = idleWindow.requestIdleCallback(run, { timeout: 1_000 })
        return () => {
          cancelled = true
          idleWindow.cancelIdleCallback?.(handle)
        }
      }
      const handle = window.setTimeout(run, 0)
      return () => {
        cancelled = true
        window.clearTimeout(handle)
      }
    },
  }
}
