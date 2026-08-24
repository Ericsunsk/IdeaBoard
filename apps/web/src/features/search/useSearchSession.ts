import { computed, ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { USER_SEARCH_PATH } from '../../shared/constants/routePaths'
import { useAppToast } from '../../shared/composables/useAppToast'
import { libraryAssetKey, type LibrarySearchReference } from '../library/libraryResource'
import { getLibraryIslandState, setLibraryIslandState } from './libraryIslandPersistence'
import {
  isSearchQueryWithinLimit,
  normalizeSearchQuery,
  type SearchResult,
  type VisionReference,
} from './searchResource'
import { useSearchLightbox } from './useSearchLightbox'
import { createSearchFeed } from './searchFeed'
import { useVisionQueryLifecycle } from './useVisionQueryLifecycle'
import {
  allowAnonymousSearchQuery,
  consumePendingSearchAuthAction,
  peekPendingSearchAuthAction,
  type SearchAuthAction,
} from './searchAuthContinuation'
import { type SearchLibraryCoordinator } from './searchLibraryCoordinator'

export function useSearchSession(options: {
  libraryCoordinator: SearchLibraryCoordinator
  cacheScope?: string
  scrollContainer?: Ref<HTMLElement | null>
  isAuthenticated?: () => boolean
  requireLogin?: (action: SearchAuthAction) => void
  initialVisionReferences?: VisionReference[]
  route?: ReturnType<typeof useRoute>
  router?: ReturnType<typeof useRouter>
  toast?: ReturnType<typeof useAppToast>
}) {
  const route = options.route ?? useRoute()
  const router = options.router ?? useRouter()
  const toast = options.toast ?? useAppToast()
  const cacheScope = options.cacheScope ?? 'anonymous'
  const isAuthenticated = options.isAuthenticated ?? (() => true)
  const libraryCoordinator = options.libraryCoordinator
  const draftQuery = ref('')
  const submittedQuery = ref('')
  const queryError = ref('')
  const keepResultsOnNextLoad = ref(false)
  const feed = createSearchFeed({
    submittedQuery,
    keepResultsOnNextLoad,
    cacheScope,
    scrollContainer: options.scrollContainer,
    isAuthenticated,
  })
  const vision = useVisionQueryLifecycle({
    draftQuery,
    submittedQuery,
    queryError,
    keepResultsOnNextLoad,
    loadResults: () => feed.load(true),
    isAuthenticated,
    requireLogin: (action) => options.requireLogin?.(action),
    cacheScope,
    initialReferences: options.initialVisionReferences,
    router,
    toast,
  })
  const lightbox = useSearchLightbox()
  const islandState = getLibraryIslandState(cacheScope)
  // A persisted open state must never auto-mount the protected Library island
  // for an anonymous Search session. It would immediately issue 401 requests.
  const libraryIslandOpen = ref(isAuthenticated() && islandState.open)
  const libraryIslandExpanded = ref(isAuthenticated() && islandState.expanded)
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
    vision.clearReferences()
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
    await vision.addSearchResults(toAdd)
    if (selectedBatchResults.value.length > 4) {
      toast.info('Added top 4 selected images as vision search references')
    }
    clearSelection()
  }

  async function saveReferenceToQuickSaves(reference: LibrarySearchReference): Promise<boolean> {
    try {
      await libraryCoordinator.saveReference(reference)
      feed.markSavedKey(libraryAssetKey(reference.provider, reference.id))
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
    const outcomes = await libraryCoordinator.saveReferences(items.map(toLibraryReference))
    let successCount = 0
    for (const [index, outcome] of outcomes.entries()) {
      if (outcome.status !== 'fulfilled') continue
      const item = items[index]
      if (!item) continue
      feed.markSavedKey(libraryAssetKey(item.provider, item.id))
      successCount += 1
    }
    const failureCount = outcomes.length - successCount
    if (failureCount === 0) {
      toast.success(`Saved ${successCount} assets to Library!`)
    } else if (successCount === 0) {
      toast.error('Failed to save selected assets')
    } else {
      toast.warning(`Saved ${successCount} assets; ${failureCount} failed`)
    }
    clearSelection()
  }

  function markAssetSaved(key: string): void {
    feed.markSavedKey(key)
  }

  function markAssetUnsaved(key: string): void {
    feed.markUnsavedKey(key)
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
      await vision.submit()
      return
    }
    if (!isAuthenticated() && !allowAnonymousSearchQuery(query)) {
      options.requireLogin?.({ kind: 'text-search', query })
      return
    }
    vision.resetExtraction()
    await router.replace({ path: USER_SEARCH_PATH, query: query ? { q: query } : {} })
  }

  watch(draftQuery, (value) => {
    if (value.trim() !== '' || vision.references.value.length > 0 || submittedQuery.value === '')
      return
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

  watch(vision.error, (message) => {
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
          pending?.kind === 'vision-search' &&
          vision.state.value === 'idle'
        ) {
          await vision.resumeAfterAuthentication()
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
      if (vision.state.value === 'idle') await vision.resumeAfterAuthentication()
      if (vision.state.value === 'ready') consumePendingSearchAuthAction()
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

  function activate() {
    lightbox.activate()
  }

  function dispose() {
    vision.dispose()
    lightbox.dispose()
    feed.dispose()
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
      close: closeLibraryIsland,
      toggle: toggleLibraryIsland,
      saveReferenceToQuickSaves,
      markAssetSaved,
      markAssetUnsaved,
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
    activate,
    dispose,
  }
}
