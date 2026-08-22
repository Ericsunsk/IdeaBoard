import { nextTick, onBeforeUnmount, onMounted, ref, type Ref } from 'vue'
import { lookupSavedAssetKeys, libraryAssetKey } from '../library/libraryResource'
import {
  generateRandomInspirationCursor,
  reportSearchImageFailure,
  searchInspiration,
  type SearchResult,
} from './searchResource'
import {
  getSearchFeedCache,
  setSearchFeedCache,
  updateFeedScrollPosition,
} from './searchCache'

export function useSearchResults(options: {
  submittedQuery: Ref<string>
  keepResultsOnNextLoad: Ref<boolean>
  cacheScope?: string
}) {
  const results = ref<SearchResult[]>([])
  const nextCursor = ref('')
  const hasMore = ref(false)
  const loading = ref(false)
  const loadingMore = ref(false)
  const hasError = ref(false)
  const sentinel = ref<HTMLElement>()
  const failedImages = ref(new Set<string>())
  const imageRetryCounts = ref(new Map<string, number>())
  const savedKeys = ref(new Set<string>())

  let controller: AbortController | null = null
  let observer: IntersectionObserver | null = null
  let loadGeneration = 0
  const cacheScope = options.cacheScope ?? 'anonymous'

  async function refreshSavedKeys(items: SearchResult[], generation: number) {
    if (items.length === 0) return
    try {
      const saved = await lookupSavedAssetKeys(
        items.map((result) => ({ provider: result.provider, source_id: result.id })),
      )
      if (generation === loadGeneration) savedKeys.value = new Set([...savedKeys.value, ...saved])
    } catch (error) {
      if (import.meta.env.DEV) console.debug('library lookup unavailable', error)
    }
  }

  async function load(reset: boolean) {
    const generation = reset ? ++loadGeneration : loadGeneration
    if (reset) {
      controller?.abort()
      controller = new AbortController()
      savedKeys.value = new Set()

      // Instant hydration from session cache if available
      const cached = getSearchFeedCache(options.submittedQuery.value, true, cacheScope)
      if (cached && cached.results.length > 0 && !options.keepResultsOnNextLoad.value) {
        results.value = cached.results
        nextCursor.value = cached.nextCursor
        hasMore.value = cached.hasMore
        loading.value = false
        void refreshSavedKeys(cached.results, generation)
        if (cached.scrollY > 0) {
          void nextTick(() => {
            if (typeof window !== 'undefined') {
              window.scrollTo({ top: cached.scrollY, behavior: 'instant' })
              const pageEl = document.querySelector('.app-scrollbar')
              if (pageEl) pageEl.scrollTop = cached.scrollY
            }
          })
        }
      } else {
        if (!options.keepResultsOnNextLoad.value) results.value = []
        nextCursor.value =
          options.submittedQuery.value === '' ? generateRandomInspirationCursor() : ''
        hasMore.value = false
        loading.value = true
      }

      options.keepResultsOnNextLoad.value = false
      hasError.value = false
      failedImages.value = new Set()
      imageRetryCounts.value = new Map()
    } else {
      if (loadingMore.value || !hasMore.value) return
      loadingMore.value = true
    }
    try {
      const page = await searchInspiration({
        query: options.submittedQuery.value,
        cursor: nextCursor.value || undefined,
        limit: 24,
        signal: controller?.signal,
      })
      if (generation !== loadGeneration) return
      const incomingResults = page.results
      results.value = reset ? incomingResults : [...results.value, ...page.results]
      await refreshSavedKeys(incomingResults, generation)
      if (generation === loadGeneration) {
        nextCursor.value = page.next_cursor || ''
        hasMore.value = page.has_more
        setSearchFeedCache({
          query: options.submittedQuery.value,
          results: results.value,
          nextCursor: nextCursor.value,
          hasMore: hasMore.value,
          scope: cacheScope,
        })
      }
    } catch (error) {
      if (
        generation === loadGeneration &&
        !(error instanceof DOMException && error.name === 'AbortError')
      ) {
        hasError.value = true
      }
    } finally {
      if (generation === loadGeneration) {
        loading.value = false
        loadingMore.value = false
      }
    }
  }

  async function reload() {
    options.keepResultsOnNextLoad.value = results.value.length > 0
    await load(true)
  }

  function markImageError(id: string) {
    if (failedImages.value.has(id)) return
    failedImages.value = new Set(failedImages.value).add(id)
    const result = results.value.find((item) => item.id === id)
    if (result) void reportSearchImageFailure(result.provider)
  }

  function retryImage(id: string) {
    const next = new Map(imageRetryCounts.value)
    next.set(id, (next.get(id) ?? 0) + 1)
    imageRetryCounts.value = next
    const failed = new Set(failedImages.value)
    failed.delete(id)
    failedImages.value = failed
  }

  function imageRetryCount(id: string) {
    return imageRetryCounts.value.get(id) ?? 0
  }

  function markSaved(result: SearchResult) {
    savedKeys.value = new Set(savedKeys.value).add(libraryAssetKey(result.provider, result.id))
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

  function clearResults() {
    loadGeneration += 1
    controller?.abort()
    controller = null
    results.value = []
    nextCursor.value = ''
    hasMore.value = false
    hasError.value = false
    failedImages.value = new Set()
    imageRetryCounts.value = new Map()
    savedKeys.value = new Set()
    loading.value = false
    loadingMore.value = false
  }

  let scrollTimeout: ReturnType<typeof setTimeout> | null = null
  const handleScroll = (e?: Event) => {
    if (scrollTimeout) return
    scrollTimeout = setTimeout(() => {
      scrollTimeout = null
      if (typeof window !== 'undefined') {
        const target = e?.target as HTMLElement | null
        const top =
          target && target.scrollTop !== undefined
            ? target.scrollTop
            : (window.scrollY || document.documentElement.scrollTop || 0)
        updateFeedScrollPosition(top, cacheScope)
      }
    }, 150)
  }

  onMounted(async () => {
    await nextTick()
    if (typeof window !== 'undefined') {
      window.addEventListener('scroll', handleScroll, { capture: true, passive: true })
    }
    observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void load(false)
      },
      { rootMargin: '640px' },
    )
    if (sentinel.value) observer.observe(sentinel.value)
  })

  onBeforeUnmount(() => {
    controller?.abort()
    observer?.disconnect()
    if (typeof window !== 'undefined') {
      window.removeEventListener('scroll', handleScroll, { capture: true } as EventListenerOptions)
    }
    if (scrollTimeout) clearTimeout(scrollTimeout)
  })

  return {
    results,
    hasMore,
    loading,
    loadingMore,
    hasError,
    sentinel,
    failedImages,
    savedKeys,
    load,
    reload,
    clearResults,
    markImageError,
    retryImage,
    imageRetryCount,
    markSaved,
    markSavedKey,
    markUnsaved,
    markUnsavedKey,
  }
}
