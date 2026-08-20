import { nextTick, onBeforeUnmount, onMounted, ref, type ComputedRef, type Ref } from 'vue'
import { lookupSavedAssetKeys, libraryAssetKey } from '../library/libraryResource'
import { MOCK_SEARCH_RESULTS } from './mockSearchData'
import {
  reportSearchImageFailure,
  searchInspiration,
  type SearchResult,
} from './searchResource'

export type SearchRefinementState = {
  palette: string[]
  atmosphereTags: string[]
}

export function useSearchResults(options: {
  submittedQuery: Ref<string>
  refinements: ComputedRef<SearchRefinementState>
  keepResultsOnNextLoad: Ref<boolean>
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

  async function load(reset: boolean) {
    const generation = reset ? ++loadGeneration : loadGeneration
    if (reset) {
      controller?.abort()
      controller = new AbortController()
      if (!options.keepResultsOnNextLoad.value) results.value = []
      options.keepResultsOnNextLoad.value = false
      nextCursor.value = ''
      hasMore.value = false
      hasError.value = false
      failedImages.value = new Set()
      imageRetryCounts.value = new Map()
      savedKeys.value = new Set()
      loading.value = true
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
        palette: options.refinements.value.palette,
        atmosphereTags: options.refinements.value.atmosphereTags,
      })
      if (generation !== loadGeneration) return
      const incomingResults =
        page.results.length > 0 ? page.results : !options.submittedQuery.value ? MOCK_SEARCH_RESULTS : []
      results.value = reset ? incomingResults : [...results.value, ...page.results]
      try {
        const saved = await lookupSavedAssetKeys(
          incomingResults.map((result) => ({ provider: result.provider, source_id: result.id })),
        )
        if (generation === loadGeneration) savedKeys.value = new Set([...savedKeys.value, ...saved])
      } catch (error) {
        if (import.meta.env.DEV) console.debug('library lookup unavailable', error)
      }
      if (generation === loadGeneration) {
        nextCursor.value = page.next_cursor || ''
        hasMore.value = page.has_more
      }
    } catch (error) {
      if (
        generation === loadGeneration &&
        !(error instanceof DOMException && error.name === 'AbortError')
      ) {
        if (import.meta.env.DEV && results.value.length === 0 && !options.submittedQuery.value) {
          results.value = MOCK_SEARCH_RESULTS
          hasError.value = false
        } else {
          hasError.value = true
        }
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

  onMounted(async () => {
    await nextTick()
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
    markImageError,
    retryImage,
    imageRetryCount,
    markSaved,
    markSavedKey,
  }
}
