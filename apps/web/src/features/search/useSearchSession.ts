import { computed, ref, watch, type Ref } from 'vue'
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
import {
  allowAnonymousSearchQuery,
  consumePendingSearchAuthAction,
  peekPendingSearchAuthAction,
  type SearchAuthAction,
} from './searchAuthContinuation'

export function useSearchSession(
  options: {
    cacheScope?: string
    scrollContainer?: Ref<HTMLElement | null>
    isAuthenticated?: () => boolean
    requireLogin?: (action: SearchAuthAction) => void
  } = {},
) {
  const route = useRoute()
  const router = useRouter()
  const toast = useAppToast()
  const cacheScope = options.cacheScope ?? 'anonymous'
  const isAuthenticated = options.isAuthenticated ?? (() => true)
  const draftQuery = ref('')
  const submittedQuery = ref('')
  const queryError = ref('')
  const keepResultsOnNextLoad = ref(false)
  const feed = useSearchResults({
    submittedQuery,
    keepResultsOnNextLoad,
    cacheScope,
    scrollContainer: options.scrollContainer,
    isAuthenticated,
  })
  const pendingAction = peekPendingSearchAuthAction()
  const vision = useVisionSearchFlow({
    draftQuery,
    submittedQuery,
    queryError,
    keepResultsOnNextLoad,
    loadCatalog: () => feed.load(true),
    cacheScope,
    initialReferences: pendingAction?.kind === 'vision-search' ? pendingAction.references : undefined,
  })
  const lightbox = useSearchLightbox()
  const islandState = getLibraryIslandState(cacheScope)
  // A persisted open state must never auto-mount the protected Library island
  // for an anonymous Search session. It would immediately issue 401 requests.
  const libraryIslandOpen = ref(isAuthenticated() && islandState.open)
  const libraryIslandExpanded = ref(isAuthenticated() && islandState.expanded)
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

  async function saveReferenceToQuickSaves(reference: LibrarySearchReference): Promise<boolean> {
    try {
      await saveSearchReference(reference)
      feed.markSavedKey(libraryAssetKey(reference.provider, reference.id))
      libraryRefreshKey.value += 1
      toast.success(`Saved "${reference.title || 'Image'}" to Quick Saves!`)
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '保存素材失败')
      return false
    }
  }

  async function saveAllSelectedToLibrary(): Promise<void> {
    const items = [...selectedBatchResults.value]
    if (items.length === 0) return
    if (!isAuthenticated()) {
      options.requireLogin?.({
        kind: 'save-batch',
        references: items.map(toLibraryReference),
      })
      return
    }
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
      if (!isAuthenticated()) {
        options.requireLogin?.({
          kind: 'vision-search',
          references: vision.references.value.map((reference) => ({
            ...reference,
            previewUrl: reference.file
              ? URL.createObjectURL(reference.file)
              : reference.previewUrl,
          })),
          query,
        })
        return
      }
      await vision.runVisionSearch()
      return
    }
    if (!isAuthenticated() && !allowAnonymousSearchQuery(query)) {
      options.requireLogin?.({ kind: 'text-search', query })
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
        const pending = peekPendingSearchAuthAction()
        if (
          isAuthenticated() &&
          pending?.kind !== 'vision-search' &&
          vision.visionState.value === 'idle'
        ) {
          await vision.runVisionSearch()
        }
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

  async function resumePendingAction(): Promise<void> {
    if (!isAuthenticated()) return
    const action = peekPendingSearchAuthAction()
    if (!action) return

    if (action.kind === 'text-search') {
      await router.replace({
        path: USER_SEARCH_PATH,
        query: action.query ? { q: action.query } : {},
      })
      consumePendingSearchAuthAction()
      return
    }

    if (action.kind === 'vision-search') {
      if (vision.visionState.value === 'idle') await vision.runVisionSearch()
      if (vision.visionState.value === 'ready') consumePendingSearchAuthAction()
      return
    }

    if (action.kind === 'open-library') {
      libraryIslandOpen.value = true
      consumePendingSearchAuthAction()
      return
    }

    const references = action.kind === 'save-reference' ? [action.reference] : action.references
    let allSaved = true
    for (const reference of references) {
      if (!(await saveReferenceToQuickSaves(reference))) allSaved = false
    }
    if (allSaved) consumePendingSearchAuthAction()
  }

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
    resumePendingAction,
    toLibraryReference,
  }
}
