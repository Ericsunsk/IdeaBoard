import { nextTick, ref, type Ref } from 'vue'
import { libraryAssetKey, lookupSavedAssetKeys } from '../library/libraryResource'
import {
  generateRandomInspirationCursor,
  reportSearchImageFailure,
  searchInspiration,
  type SearchResult,
} from './searchResource'
import {
  createBrowserSearchFeedPersistence,
  type SearchFeedPersistence,
  type SearchFeedSnapshot,
} from './searchFeedPersistence'

export type SearchFeedDependencies = {
  randomCursor: () => string
  search: typeof searchInspiration
  lookupSavedKeys: typeof lookupSavedAssetKeys
  persistence: SearchFeedPersistence
  restoreScroll: (scrollY: number) => void
  reportImageFailure: typeof reportSearchImageFailure
}

const defaults: SearchFeedDependencies = {
  randomCursor: generateRandomInspirationCursor,
  search: searchInspiration,
  lookupSavedKeys: lookupSavedAssetKeys,
  persistence: createBrowserSearchFeedPersistence(),
  restoreScroll(scrollY) {
    window.scrollTo({ top: scrollY, behavior: 'instant' })
  },
  reportImageFailure: reportSearchImageFailure,
}

type SearchFeedOptions = {
  submittedQuery: Ref<string>
  keepResultsOnNextLoad: Ref<boolean>
  cacheScope?: string | undefined
  scrollContainer?: Ref<HTMLElement | null> | undefined
  isAuthenticated?: (() => boolean) | undefined
}

/**
 * The one fact owner for a Search result feed. It owns page data, saved
 * projection, cache lifecycle, request ordering and cancellation; rendering
 * surfaces only report scroll/pagination observations back to it.
 */
export function createSearchFeed(
  options: SearchFeedOptions,
  overrides: Partial<SearchFeedDependencies> = {},
) {
  const dependencies = { ...defaults, ...overrides }
  const cacheScope = options.cacheScope ?? 'anonymous'
  const isAuthenticated = options.isAuthenticated ?? (() => true)
  const results = ref<SearchResult[]>([])
  const nextCursor = ref('')
  const hasMore = ref(false)
  const loading = ref(false)
  const loadingMore = ref(false)
  const hasError = ref(false)
  const savedKeys = ref(new Set<string>())
  const failedImages = ref(new Set<string>())
  const imageRetryCounts = ref(new Map<string, number>())

  let controller: AbortController | null = null
  let loadGeneration = 0
  let cacheWriteVersion = 0
  let cancelScheduledCacheWrite = () => {}
  let scrollWriteTimer: ReturnType<typeof setTimeout> | null = null
  let pendingScrollY = 0
  let pendingScrollTarget: { query: string; scope: string } | null = null
  let disposed = false
  let savedMutationRevision = 0
  const savedKeyMutationRevisions = new Map<string, number>()

  function cancelCacheWrite(): void {
    cacheWriteVersion += 1
    cancelScheduledCacheWrite()
    cancelScheduledCacheWrite = () => {}
  }

  function cancelScrollWrite(): void {
    if (scrollWriteTimer) clearTimeout(scrollWriteTimer)
    scrollWriteTimer = null
    pendingScrollTarget = null
  }

  function scheduleCacheWrite(
    snapshot: Omit<SearchFeedSnapshot, 'scrollY'>,
    generation: number,
  ): void {
    cancelCacheWrite()
    const version = cacheWriteVersion
    // Results are replaced on a refresh and appended by page. Persist one
    // immutable snapshot so cursor and cards can never drift apart.
    const completeSnapshot = { ...snapshot, results: [...snapshot.results] }
    cancelScheduledCacheWrite = dependencies.persistence.scheduleWrite(() => {
      if (
        disposed ||
        generation !== loadGeneration ||
        version !== cacheWriteVersion ||
        completeSnapshot.query !== options.submittedQuery.value
      )
        return
      dependencies.persistence.write(completeSnapshot, cacheScope)
    })
  }

  async function refreshSavedKeys(items: SearchResult[], generation: number): Promise<void> {
    if (items.length === 0 || !isAuthenticated()) return
    const lookupStartedAtRevision = savedMutationRevision
    try {
      const saved = await dependencies.lookupSavedKeys(
        items.map((result) => ({ provider: result.provider, source_id: result.id })),
      )
      if (generation !== loadGeneration) return
      const nextKeys = new Set(savedKeys.value)
      for (const key of saved) {
        if ((savedKeyMutationRevisions.get(key) ?? 0) <= lookupStartedAtRevision) nextKeys.add(key)
      }
      savedKeys.value = nextKeys
    } catch (error) {
      if (import.meta.env.DEV) console.debug('library lookup unavailable', error)
    }
  }

  function hydrate(snapshot: SearchFeedSnapshot): void {
    results.value = snapshot.results
    nextCursor.value = snapshot.nextCursor
    hasMore.value = snapshot.hasMore
    loading.value = false
    if (snapshot.scrollY <= 0) return
    void nextTick(() => {
      const root = options.scrollContainer?.value
      if (root) {
        root.scrollTo({ top: snapshot.scrollY, behavior: 'instant' })
        return
      }
      dependencies.restoreScroll(snapshot.scrollY)
    })
  }

  async function load(reset: boolean): Promise<void> {
    const generation = reset ? ++loadGeneration : loadGeneration
    let cachedFallback: SearchFeedSnapshot | null = null
    if (reset) {
      controller?.abort()
      cancelCacheWrite()
      cancelScrollWrite()
      controller = new AbortController()
      savedKeys.value = new Set()
      savedMutationRevision = 0
      savedKeyMutationRevisions.clear()
      failedImages.value = new Set()
      imageRetryCounts.value = new Map()

      const cached = dependencies.persistence.read(options.submittedQuery.value, cacheScope, true)
      if (cached && cached.results.length > 0 && !options.keepResultsOnNextLoad.value) {
        cachedFallback = cached
        hydrate(cached)
        nextCursor.value = options.submittedQuery.value === '' ? dependencies.randomCursor() : ''
        loading.value = true
      } else {
        if (!options.keepResultsOnNextLoad.value) results.value = []
        nextCursor.value = options.submittedQuery.value === '' ? dependencies.randomCursor() : ''
        hasMore.value = false
        loading.value = true
      }
      options.keepResultsOnNextLoad.value = false
      hasError.value = false
    } else {
      if (loading.value || loadingMore.value || !hasMore.value) return
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
      const nextResults = reset ? page.results : [...results.value, ...page.results]
      results.value = nextResults
      nextCursor.value = page.next_cursor || ''
      hasMore.value = page.has_more
      scheduleCacheWrite(
        {
          query: options.submittedQuery.value,
          results: nextResults,
          nextCursor: nextCursor.value,
          hasMore: hasMore.value,
        },
        generation,
      )
      // Fresh results replace cached cards, so resolve saved state only once
      // against the rendered page instead of merging stale cache projections.
      void refreshSavedKeys(page.results, generation)
    } catch (error) {
      if (
        generation === loadGeneration &&
        !(error instanceof DOMException && error.name === 'AbortError')
      ) {
        hasError.value = true
        if (cachedFallback) {
          // A failed SWR refresh must leave one coherent cached page behind.
          // Otherwise cards are cached but pagination restarts at page one.
          nextCursor.value = cachedFallback.nextCursor
          hasMore.value = cachedFallback.hasMore
          void refreshSavedKeys(cachedFallback.results, generation)
        }
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

  function recordScroll(scrollY: number): void {
    const target = { query: options.submittedQuery.value, scope: cacheScope }
    if (
      scrollWriteTimer &&
      (pendingScrollTarget?.query !== target.query || pendingScrollTarget.scope !== target.scope)
    ) {
      cancelScrollWrite()
    }
    pendingScrollY = scrollY
    if (scrollWriteTimer) return
    pendingScrollTarget = target
    scrollWriteTimer = setTimeout(() => {
      scrollWriteTimer = null
      pendingScrollTarget = null
      dependencies.persistence.writeScroll(pendingScrollY, target.query, target.scope)
    }, 150)
  }

  function markSavedKey(key: string): void {
    savedMutationRevision += 1
    savedKeyMutationRevisions.set(key, savedMutationRevision)
    savedKeys.value = new Set(savedKeys.value).add(key)
  }

  function markUnsavedKey(key: string): void {
    savedMutationRevision += 1
    savedKeyMutationRevisions.set(key, savedMutationRevision)
    const next = new Set(savedKeys.value)
    next.delete(key)
    savedKeys.value = next
  }

  function markSaved(result: SearchResult): void {
    markSavedKey(libraryAssetKey(result.provider, result.id))
  }

  function markUnsaved(result: SearchResult): void {
    markUnsavedKey(libraryAssetKey(result.provider, result.id))
  }

  function markImageError(id: string): void {
    if (failedImages.value.has(id)) return
    failedImages.value = new Set(failedImages.value).add(id)
    const result = results.value.find((item) => item.id === id)
    if (result && isAuthenticated()) void dependencies.reportImageFailure(result.provider)
  }

  function retryImage(id: string): void {
    const nextRetries = new Map(imageRetryCounts.value)
    nextRetries.set(id, (nextRetries.get(id) ?? 0) + 1)
    imageRetryCounts.value = nextRetries
    const nextFailed = new Set(failedImages.value)
    nextFailed.delete(id)
    failedImages.value = nextFailed
  }

  function imageRetryCount(id: string): number {
    return imageRetryCounts.value.get(id) ?? 0
  }

  function clear(): void {
    loadGeneration += 1
    controller?.abort()
    cancelCacheWrite()
    cancelScrollWrite()
    controller = null
    results.value = []
    nextCursor.value = ''
    hasMore.value = false
    hasError.value = false
    savedKeys.value = new Set()
    failedImages.value = new Set()
    imageRetryCounts.value = new Map()
    loading.value = false
    loadingMore.value = false
  }

  function dispose(): void {
    disposed = true
    clear()
  }

  return {
    results,
    hasMore,
    loading,
    loadingMore,
    hasError,
    savedKeys,
    failedImages,
    load,
    reload,
    recordScroll,
    markSaved,
    markSavedKey,
    markUnsaved,
    markUnsavedKey,
    markImageError,
    retryImage,
    imageRetryCount,
    clear,
    dispose,
  }
}
