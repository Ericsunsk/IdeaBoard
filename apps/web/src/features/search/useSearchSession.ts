import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { USER_SEARCH_PATH } from '../../shared/constants/routePaths'
import { useAppToast } from '../../shared/composables/useAppToast'
import {
  libraryAssetKey,
  saveSearchReference,
  type LibrarySearchReference,
} from '../library/libraryResource'
import {
  getLibraryIslandState,
  setLibraryIslandState,
} from './searchCache'
import {
  isSearchQueryWithinLimit,
  normalizeSearchQuery,
  type SearchResult,
} from './searchResource'
import { useSearchLightbox } from './useSearchLightbox'
import { useSearchResults } from './useSearchResults'
import { useVisionSearchFlow } from './useVisionSearchFlow'

export function useSearchSession(options: { cacheScope?: string } = {}) {
  const route = useRoute()
  const router = useRouter()
  const toast = useAppToast()
  const cacheScope = options.cacheScope ?? 'anonymous'
  const draftQuery = ref('')
  const submittedQuery = ref('')
  const queryError = ref('')
  const keepResultsOnNextLoad = ref(false)
  const feed = useSearchResults({
    submittedQuery,
    keepResultsOnNextLoad,
    cacheScope,
  })
  const vision = useVisionSearchFlow({
    draftQuery,
    submittedQuery,
    queryError,
    keepResultsOnNextLoad,
    loadCatalog: () => feed.load(true),
    cacheScope,
  })
  const lightbox = useSearchLightbox()
  const islandState = getLibraryIslandState(cacheScope)
  const libraryIslandOpen = ref(islandState.open)
  const libraryIslandExpanded = ref(islandState.expanded)
  const libraryRefreshKey = ref(0)
  const selectedBatchResults = ref<SearchResult[]>([])
  const heading = computed(() => {
    if (vision.references.value.length > 0) return 'Results for “Visual Concepts”'
    return submittedQuery.value ? `Results for “${submittedQuery.value}”` : 'Inspiration'
  })

  function queryValue(): string {
    const value = route.query.q
    return Array.isArray(value) ? String(value[0] || '') : String(value || '')
  }

  function clearSearch(): void {
    vision.clearAllReferences()
    draftQuery.value = ''
    submittedQuery.value = ''
    queryError.value = ''
    if (route.query.q) {
      void router.replace({ path: USER_SEARCH_PATH, query: {} })
      return
    }
    void feed.load(true)
  }

  function toLibraryReference(result: SearchResult): LibrarySearchReference {
    return {
      provider: result.provider,
      id: result.id,
      source_url: result.source_url,
      preview_url: result.preview_url,
      original_url: result.original_url,
      title: result.title,
      author: result.author,
      width: result.width,
      height: result.height,
    }
  }

  function isResultSelected(result: SearchResult): boolean {
    return selectedBatchResults.value.some(
      (selected) => selected.provider === result.provider && selected.id === result.id,
    )
  }

  function toggleBatchSelect(result: SearchResult): void {
    const index = selectedBatchResults.value.findIndex(
      (selected) => selected.provider === result.provider && selected.id === result.id,
    )
    if (index >= 0) {
      selectedBatchResults.value.splice(index, 1)
      return
    }
    if (selectedBatchResults.value.length >= 8) {
      toast.warning('Maximum 8 images for batch selection!')
      return
    }
    selectedBatchResults.value.push(result)
  }

  function clearSelection(): void {
    selectedBatchResults.value = []
  }

  async function applySelectedAsVisionReferences(): Promise<void> {
    const toAdd = selectedBatchResults.value.slice(0, 4)
    for (const result of toAdd) await vision.handleSearchResultReference(result)
    if (selectedBatchResults.value.length > 4) {
      toast.info('Added top 4 selected images as vision search references')
    }
    clearSelection()
  }

  async function saveReferenceToQuickSaves(reference: LibrarySearchReference): Promise<void> {
    try {
      await saveSearchReference(reference)
      feed.markSavedKey(libraryAssetKey(reference.provider, reference.id))
      libraryRefreshKey.value += 1
      toast.success(`Saved "${reference.title || 'Image'}" to Quick Saves!`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '保存素材失败')
    }
  }

  async function saveAllSelectedToLibrary(): Promise<void> {
    const items = [...selectedBatchResults.value]
    if (items.length === 0) return
    let successCount = 0
    let failureCount = 0
    for (const item of items) {
      try {
        await saveSearchReference(toLibraryReference(item))
        feed.markSavedKey(libraryAssetKey(item.provider, item.id))
        successCount += 1
      } catch {
        failureCount += 1
      }
    }
    libraryRefreshKey.value += 1
    if (failureCount === 0) {
      toast.success(`Saved ${successCount} assets to Library!`)
    } else if (successCount === 0) {
      toast.error('Failed to save selected assets')
    } else {
      toast.warning(`Saved ${successCount} assets; ${failureCount} failed`)
    }
    clearSelection()
  }

  function markResultSaved(result: SearchResult): void {
    feed.markSaved(result)
    libraryRefreshKey.value += 1
  }

  function markResultUnsaved(result: SearchResult): void {
    feed.markUnsaved(result)
    libraryRefreshKey.value += 1
  }

  function markAssetSaved(key: string): void {
    feed.markSavedKey(key)
  }

  function closeLibraryIsland(): void {
    libraryIslandOpen.value = false
    libraryIslandExpanded.value = false
  }

  function toggleLibraryIsland(): void {
    if (libraryIslandOpen.value) {
      closeLibraryIsland()
      return
    }
    libraryIslandOpen.value = true
  }

  async function submitQuery(): Promise<void> {
    const query = normalizeSearchQuery(draftQuery.value)
    if (!isSearchQueryWithinLimit(query)) {
      queryError.value = '搜索关键词不能超过 200 个字符'
      return
    }
    queryError.value = ''
    if (vision.references.value.length > 0) {
      await vision.runVisionSearch()
      return
    }
    vision.clearVisionExtraction()
    await router.replace({ path: USER_SEARCH_PATH, query: query ? { q: query } : {} })
  }

  watch(draftQuery, (value) => {
    if (value.trim() !== '' || vision.references.value.length > 0 || submittedQuery.value === '') return
    submittedQuery.value = ''
    if (route.query.q) {
      void router.replace({ path: USER_SEARCH_PATH, query: {} })
      return
    }
    void feed.load(true)
  })

  watch([libraryIslandOpen, libraryIslandExpanded], ([open, expanded]) => {
    setLibraryIslandState({ open, expanded }, cacheScope)
  })

  watch(feed.results, clearSelection)

  watch(vision.visionError, (message) => {
    if (message) toast.error(message)
  })

  watch(
    () => route.query.q,
    async () => {
      if (vision.references.value.length > 0) {
        const query = normalizeSearchQuery(queryValue())
        if (query) draftQuery.value = query
        if (vision.visionState.value === 'idle') await vision.runVisionSearch()
        return
      }
      const query = normalizeSearchQuery(queryValue())
      submittedQuery.value = query
      draftQuery.value = query
      queryError.value = ''
      await feed.load(true)
    },
    { immediate: true },
  )

  return {
    query: {
      draftQuery,
      submittedQuery,
      queryError,
      heading,
    },
    feed,
    vision,
    lightbox,
    library: {
      open: libraryIslandOpen,
      expanded: libraryIslandExpanded,
      refreshKey: libraryRefreshKey,
      close: closeLibraryIsland,
      toggle: toggleLibraryIsland,
      saveReferenceToQuickSaves,
      markResultSaved,
      markResultUnsaved,
      markAssetSaved,
    },
    batch: {
      selected: selectedBatchResults,
      isSelected: isResultSelected,
      toggle: toggleBatchSelect,
      clear: clearSelection,
      applyVisionReferences: applySelectedAsVisionReferences,
      saveAllToLibrary: saveAllSelectedToLibrary,
    },
    clearSearch,
    submitQuery,
    toLibraryReference,
  }
}
