import { nextTick, ref, type Ref } from 'vue'
import { lookupSavedAssetKeys, libraryAssetKey } from '../library/libraryResource'
import {
  generateRandomInspirationCursor,
  searchInspiration,
  type SearchResult,
} from './searchResource'
import { getSearchFeedCache, setSearchFeedCache, type SearchFeedCache } from './searchCache'

type InspirationFeedCache = Pick<
  SearchFeedCache,
  'results' | 'nextCursor' | 'hasMore' | 'scrollY'
>

export interface InspirationFeedDependencies {
  randomCursor: () => string
  search: typeof searchInspiration
  lookupSavedKeys: typeof lookupSavedAssetKeys
  getCache: (query: string, allowReloadBypass: boolean, scope: string) => InspirationFeedCache | null
  setCache: (data: {
    query: string
    results: SearchResult[]
    nextCursor: string
    hasMore: boolean
    scope: string
  }) => void
  restoreScroll: (scrollY: number) => void
}

const defaultDependencies: InspirationFeedDependencies = {
  randomCursor: generateRandomInspirationCursor,
  search: searchInspiration,
  lookupSavedKeys: lookupSavedAssetKeys,
  getCache: getSearchFeedCache,
  setCache: setSearchFeedCache,
  restoreScroll(scrollY) {
    if (typeof window === 'undefined') return
    window.scrollTo({ top: scrollY, behavior: 'instant' })
    const pageEl = document.querySelector('.app-scrollbar')
    if (pageEl) pageEl.scrollTop = scrollY
  },
}

export function createInspirationFeed(
  options: {
    submittedQuery: Ref<string>
    keepResultsOnNextLoad: Ref<boolean>
    cacheScope?: string
  },
  overrides: Partial<InspirationFeedDependencies> = {},
) {
  const dependencies = { ...defaultDependencies, ...overrides }
  const cacheScope = options.cacheScope ?? 'anonymous'
  const results = ref<SearchResult[]>([])
  const nextCursor = ref('')
  const hasMore = ref(false)
  const loading = ref(false)
  const loadingMore = ref(false)
  const hasError = ref(false)
  const savedKeys = ref(new Set<string>())

  let controller: AbortController | null = null
  let loadGeneration = 0

  async function refreshSavedKeys(items: SearchResult[], generation: number) {
    if (items.length === 0) return
    try {
      const saved = await dependencies.lookupSavedKeys(
        items.map((result) => ({ provider: result.provider, source_id: result.id })),
      )
      if (generation === loadGeneration) savedKeys.value = new Set([...savedKeys.value, ...saved])
    } catch (error) {
      if (import.meta.env.DEV) console.debug('library lookup unavailable', error)
    }
  }

  function hydrateFromCache(cache: InspirationFeedCache): void {
    results.value = cache.results
    nextCursor.value = cache.nextCursor
    hasMore.value = cache.hasMore
    loading.value = false
    if (cache.scrollY > 0) void nextTick(() => dependencies.restoreScroll(cache.scrollY))
  }

  async function load(reset: boolean): Promise<void> {
    const generation = reset ? ++loadGeneration : loadGeneration
    if (reset) {
      controller?.abort()
      controller = new AbortController()
      savedKeys.value = new Set()

      const cached = dependencies.getCache(options.submittedQuery.value, true, cacheScope)
      if (cached && cached.results.length > 0 && !options.keepResultsOnNextLoad.value) {
        hydrateFromCache(cached)
        void refreshSavedKeys(cached.results, generation)
      } else {
        if (!options.keepResultsOnNextLoad.value) results.value = []
        nextCursor.value = options.submittedQuery.value === '' ? dependencies.randomCursor() : ''
        hasMore.value = false
        loading.value = true
      }

      options.keepResultsOnNextLoad.value = false
      hasError.value = false
    } else {
      if (loadingMore.value || !hasMore.value) return
      loadingMore.value = true
    }

    try {
      const page = await dependencies.search({
        query: options.submittedQuery.value,
        cursor: nextCursor.value || undefined,
        limit: 24,
        signal: controller?.signal,
      })
      if (generation !== loadGeneration) return
      results.value = reset ? page.results : [...results.value, ...page.results]
      await refreshSavedKeys(page.results, generation)
      if (generation !== loadGeneration) return

      nextCursor.value = page.next_cursor || ''
      hasMore.value = page.has_more
      dependencies.setCache({
        query: options.submittedQuery.value,
        results: results.value,
        nextCursor: nextCursor.value,
        hasMore: hasMore.value,
        scope: cacheScope,
      })
    } catch (error) {
      if (generation === loadGeneration && !(error instanceof DOMException && error.name === 'AbortError')) {
        hasError.value = true
      }
    } finally {
      if (generation === loadGeneration) {
        loading.value = false
        loadingMore.value = false
      }
    }
  }

  async function reload(): Promise<void> {
    options.keepResultsOnNextLoad.value = results.value.length > 0
    await load(true)
  }

  function markSaved(result: SearchResult) {
    markSavedKey(libraryAssetKey(result.provider, result.id))
  }

  function markSavedKey(key: string) {
    savedKeys.value = new Set(savedKeys.value).add(key)
  }

  function markUnsaved(result: SearchResult) {
    markUnsavedKey(libraryAssetKey(result.provider, result.id))
  }

  function markUnsavedKey(key: string) {
    const next = new Set(savedKeys.value)
    next.delete(key)
    savedKeys.value = next
  }

  function clear(): void {
    loadGeneration += 1
    controller?.abort()
    controller = null
    results.value = []
    nextCursor.value = ''
    hasMore.value = false
    hasError.value = false
    savedKeys.value = new Set()
    loading.value = false
    loadingMore.value = false
  }

  function dispose(): void {
    controller?.abort()
    controller = null
  }

  return {
    results,
    hasMore,
    loading,
    loadingMore,
    hasError,
    savedKeys,
    load,
    reload,
    markSaved,
    markSavedKey,
    markUnsaved,
    markUnsavedKey,
    clear,
    dispose,
  }
}
