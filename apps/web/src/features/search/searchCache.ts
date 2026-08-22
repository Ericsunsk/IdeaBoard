import type { SearchResult, VisionReference } from './searchResource'

const FEED_CACHE_KEY = 'ideaboard:search:feed_cache'
const FEED_SCROLL_KEY = 'ideaboard:search:feed_scroll'
const ISLAND_STATE_KEY = 'ideaboard:search:island_state'
const VISION_REF_CACHE_KEY = 'ideaboard:search:vision_refs'
const MAX_CACHE_AGE_MS = 30 * 60 * 1000 // 30 minutes

export interface SearchFeedCache {
  query: string
  results: SearchResult[]
  nextCursor: string
  hasMore: boolean
  scrollY: number
  timestamp: number
}

type PersistedSearchFeedCache = Omit<SearchFeedCache, 'scrollY'>

export interface CachedVisionReference {
  key: string
  previewUrl: string
  libraryAssetID?: number
}

export interface LibraryIslandState {
  open: boolean
  expanded: boolean
}

function safeGetStorage(): Storage | null {
  if (typeof window === 'undefined' || typeof window.sessionStorage === 'undefined') {
    return null
  }
  return window.sessionStorage
}

function scopedKey(key: string, scope: string): string {
  return `${key}:${scope || 'anonymous'}`
}

function scrollKey(scope: string, query: string): string {
  return `${scopedKey(FEED_SCROLL_KEY, scope)}:${encodeURIComponent(query)}`
}

export function isBrowserReload(): boolean {
  if (typeof window === 'undefined' || !window.performance) return false
  try {
    const navEntries = window.performance.getEntriesByType('navigation')
    if (navEntries.length > 0) {
      return (navEntries[0] as PerformanceNavigationTiming).type === 'reload'
    }
    return (window.performance as unknown as { navigation?: { type?: number } }).navigation?.type === 1
  } catch {
    return false
  }
}

export function getSearchFeedCache(
  query: string,
  allowReloadBypass = true,
  scope = 'anonymous',
): SearchFeedCache | null {
  const storage = safeGetStorage()
  if (!storage) return null
  try {
    // If empty query AND user explicitly refreshed the page in browser, bypass cache to fetch fresh inspirations
    if (query === '' && allowReloadBypass && isBrowserReload()) {
      clearSearchFeedCache(scope)
      return null
    }

    const raw = storage.getItem(scopedKey(FEED_CACHE_KEY, scope))
    if (!raw) return null
    const cache = JSON.parse(raw) as PersistedSearchFeedCache
    if (cache.query !== query) return null
    if (Date.now() - cache.timestamp > MAX_CACHE_AGE_MS) {
      storage.removeItem(scopedKey(FEED_CACHE_KEY, scope))
      return null
    }
    const scrollPosition = Number(storage.getItem(scrollKey(scope, query)))
    return {
      ...cache,
      scrollY: Number.isFinite(scrollPosition) && scrollPosition >= 0 ? scrollPosition : 0,
    }
  } catch {
    return null
  }
}

export function setSearchFeedCache(data: {
  query: string
  results: SearchResult[]
  nextCursor: string
  hasMore: boolean
  scope?: string
}): void {
  const storage = safeGetStorage()
  if (!storage) return
  try {
    const scope = data.scope ?? 'anonymous'
    const payload = {
      query: data.query,
      results: data.results,
      nextCursor: data.nextCursor,
      hasMore: data.hasMore,
      timestamp: Date.now(),
    }
    storage.setItem(scopedKey(FEED_CACHE_KEY, scope), JSON.stringify(payload))
  } catch {
    // quota exceeded or private mode
  }
}

export function updateFeedScrollPosition(scrollY: number, scope = 'anonymous', query = ''): void {
  const storage = safeGetStorage()
  if (!storage) return
  try {
    if (!Number.isFinite(scrollY) || scrollY < 0) return
    storage.setItem(scrollKey(scope, query), String(Math.round(scrollY)))
  } catch {
    // ignore
  }
}

export function clearSearchFeedCache(scope = 'anonymous'): void {
  const storage = safeGetStorage()
  if (!storage) return
  try {
    storage.removeItem(scopedKey(FEED_CACHE_KEY, scope))
    const scrollPrefix = `${scopedKey(FEED_SCROLL_KEY, scope)}:`
    for (let index = storage.length - 1; index >= 0; index -= 1) {
      const key = storage.key(index)
      if (key?.startsWith(scrollPrefix)) {
        storage.removeItem(key)
      }
    }
  } catch {
    // ignore
  }
}

export function getLibraryIslandState(scope = 'anonymous'): LibraryIslandState {
  const storage = safeGetStorage()
  if (!storage) return { open: false, expanded: false }
  try {
    const raw = storage.getItem(scopedKey(ISLAND_STATE_KEY, scope))
    if (!raw) return { open: false, expanded: false }
    return JSON.parse(raw) as LibraryIslandState
  } catch {
    return { open: false, expanded: false }
  }
}

export function setLibraryIslandState(state: LibraryIslandState, scope = 'anonymous'): void {
  const storage = safeGetStorage()
  if (!storage) return
  try {
    storage.setItem(scopedKey(ISLAND_STATE_KEY, scope), JSON.stringify(state))
  } catch {
    // ignore
  }
}

export function getCachedVisionReferences(scope = 'anonymous'): CachedVisionReference[] {
  const storage = safeGetStorage()
  if (!storage) return []
  try {
    const key = scopedKey(VISION_REF_CACHE_KEY, scope)
    const raw = storage.getItem(key)
    if (!raw) return []
    const references = JSON.parse(raw) as CachedVisionReference[]
    return references.filter((reference) => Boolean(reference.libraryAssetID))
  } catch {
    return []
  }
}

export function setCachedVisionReferences(references: VisionReference[], scope = 'anonymous'): void {
  const storage = safeGetStorage()
  if (!storage) return
  try {
    const serializable: CachedVisionReference[] = references
      // Only Library-backed references can be reconstructed after a reload.
      .filter((ref) => !ref.file && !ref.previewUrl.startsWith('blob:') && ref.libraryAssetID)
      .map((ref) => ({
        key: ref.key,
        previewUrl: ref.previewUrl,
        libraryAssetID: ref.libraryAssetID,
      }))
    if (serializable.length > 0) {
      storage.setItem(scopedKey(VISION_REF_CACHE_KEY, scope), JSON.stringify(serializable))
    } else {
      storage.removeItem(scopedKey(VISION_REF_CACHE_KEY, scope))
    }
  } catch {
    // ignore
  }
}

export function clearCachedVisionReferences(scope = 'anonymous'): void {
  const storage = safeGetStorage()
  if (!storage) return
  try {
    storage.removeItem(scopedKey(VISION_REF_CACHE_KEY, scope))
  } catch {
    // ignore
  }
}
