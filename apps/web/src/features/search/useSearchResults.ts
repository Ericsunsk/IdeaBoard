import { nextTick, onBeforeUnmount, onMounted, ref, type Ref } from 'vue'
import { reportSearchImageFailure } from './searchResource'
import { updateFeedScrollPosition } from './searchCache'
import { createInspirationFeed } from './inspirationFeed'

export function useSearchResults(options: {
  submittedQuery: Ref<string>
  keepResultsOnNextLoad: Ref<boolean>
  cacheScope?: string
}) {
  const feed = createInspirationFeed(options)
  const sentinel = ref<HTMLElement>()
  const failedImages = ref(new Set<string>())
  const imageRetryCounts = ref(new Map<string, number>())
  let observer: IntersectionObserver | null = null
  const cacheScope = options.cacheScope ?? 'anonymous'

  async function load(reset: boolean) {
    if (reset) {
      failedImages.value = new Set()
      imageRetryCounts.value = new Map()
    }
    await feed.load(reset)
  }

  function markImageError(id: string) {
    if (failedImages.value.has(id)) return
    failedImages.value = new Set(failedImages.value).add(id)
    const result = feed.results.value.find((item) => item.id === id)
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

  function clearResults() {
    feed.clear()
    failedImages.value = new Set()
    imageRetryCounts.value = new Map()
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
        if (entries.some((entry) => entry.isIntersecting)) void feed.load(false)
      },
      { rootMargin: '640px' },
    )
    if (sentinel.value) observer.observe(sentinel.value)
  })

  onBeforeUnmount(() => {
    feed.dispose()
    observer?.disconnect()
    if (typeof window !== 'undefined') {
      window.removeEventListener('scroll', handleScroll, { capture: true } as EventListenerOptions)
    }
    if (scrollTimeout) clearTimeout(scrollTimeout)
  })

  return {
    results: feed.results,
    hasMore: feed.hasMore,
    loading: feed.loading,
    loadingMore: feed.loadingMore,
    hasError: feed.hasError,
    sentinel,
    failedImages,
    savedKeys: feed.savedKeys,
    load,
    reload: feed.reload,
    clearResults,
    markImageError,
    retryImage,
    imageRetryCount,
    markSaved: feed.markSaved,
    markSavedKey: feed.markSavedKey,
    markUnsaved: feed.markUnsaved,
    markUnsavedKey: feed.markUnsavedKey,
  }
}
