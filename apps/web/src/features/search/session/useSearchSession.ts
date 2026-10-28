import { isMediaDisplayExpired } from '../../../core/media/mediaDisplay'
import { computed, ref, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { USER_SEARCH_PATH } from '../../workspace/routePaths'
import { stringParam } from '../../../core/router/queryParams'
import { useAppToast } from '../../../core/composables/useAppToast'
import { useFailureToast } from '../../../core/composables/useFailureToast'
import { i18n } from '../../../core/i18n'
import { searchReferenceKey } from './adapters/libraryBridge'
import type { LibrarySearchReference } from '../core/authAction'
import { getLibraryIslandState, setLibraryIslandState } from './adapters/libraryIslandPersistence'
import { type SearchResult, type VisionReference } from '../core/models'
import {
  allowAnonymousSearchQuery,
  consumePendingSearchAuthAction,
  peekPendingSearchAuthAction,
} from './adapters/authContinuation'
import { isSearchQueryWithinLimit, normalizeSearchQuery } from '../core/queryPolicy'
import { useSearchLightbox } from './useSearchLightbox'
import { createSearchFeed } from './feed'
import { useVisionQueryLifecycle } from './useVisionQueryLifecycle'
import { type SearchAuthAction } from '../core/authAction'
import { type SearchLibraryCoordinator } from '../ports/libraryCoordinatorPort'

function useLibraryIslandState(cacheScope: string, isAuthenticated: () => boolean) {
  const islandState = getLibraryIslandState(cacheScope)
  // A persisted open state must never auto-mount the protected Library island
  // for an anonymous Search session. It would immediately issue 401 requests.
  const libraryIslandOpen = ref(isAuthenticated() && islandState.open)
  const libraryIslandExpanded = ref(isAuthenticated() && islandState.expanded)
  function closeLibraryIsland(): void {
    libraryIslandOpen.value = false
  }

  function toggleLibraryIsland(): void {
    if (libraryIslandOpen.value) {
      closeLibraryIsland()
      return
    }
    libraryIslandOpen.value = true
  }

  watch([libraryIslandOpen, libraryIslandExpanded], ([open, expanded]) => {
    setLibraryIslandState({ open, expanded: open && expanded }, cacheScope)
  })

  return { libraryIslandOpen, libraryIslandExpanded, closeLibraryIsland, toggleLibraryIsland }
}

type SearchSessionOptions = {
  libraryCoordinator: SearchLibraryCoordinator
  cacheScope?: string
  scrollContainer?: Ref<HTMLElement | null>
  isAuthenticated?: () => boolean
  workspaceId?: MaybeRefOrGetter<string | null | undefined>
  requireLogin?: (action: SearchAuthAction) => void
  initialVisionReferences?: VisionReference[]
  route?: ReturnType<typeof useRoute>
  router?: ReturnType<typeof useRouter>
  toast?: ReturnType<typeof useAppToast>
}

function useSearchQueryState(
  options: SearchSessionOptions,
  cacheScope: string,
  isAuthenticated: () => boolean,
  router: ReturnType<typeof useRouter>,
  toast: ReturnType<typeof useAppToast>,
) {
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
    workspaceId: options.workspaceId,
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
    workspaceId: options.workspaceId,
    router,
    toast,
  })
  return { draftQuery, submittedQuery, queryError, feed, vision }
}

export function useSearchSession(options: SearchSessionOptions) {
  const route = options.route ?? useRoute()
  const router = options.router ?? useRouter()
  const toast = options.toast ?? useAppToast()
  const cacheScope = options.cacheScope ?? 'anonymous'
  const isAuthenticated = options.isAuthenticated ?? (() => true)
  const libraryCoordinator = options.libraryCoordinator
  const { draftQuery, submittedQuery, queryError, feed, vision } = useSearchQueryState(
    options,
    cacheScope,
    isAuthenticated,
    router,
    toast,
  )
  const lightbox = useSearchLightbox()
  const { libraryIslandOpen, libraryIslandExpanded, closeLibraryIsland, toggleLibraryIsland } =
    useLibraryIslandState(cacheScope, isAuthenticated)
  const selectedBatchResults = ref<SearchResult[]>([])
  const heading = computed(() => {
    if (vision.references.value.length > 0) return i18n.global.t('search.headings.visualConcepts')
    return submittedQuery.value
      ? i18n.global.t('search.headings.resultsForQuery', { query: submittedQuery.value })
      : i18n.global.t('search.headings.inspiration')
  })

  function queryValue(): string {
    return stringParam(route.query.q) ?? ''
  }

  function clearSearch(): void {
    vision.clearReferences()
    draftQuery.value = ''
    submittedQuery.value = ''
    queryError.value = ''
    if (stringParam(route.query.q)) {
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
      toast.warning(i18n.global.t('search.toasts.maxBatchSelected'))
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
      toast.info(i18n.global.t('search.toasts.batchVisionReferencesAdded'))
    }
    clearSelection()
  }

  async function saveReferenceToQuickSaves(reference: LibrarySearchReference): Promise<boolean> {
    try {
      await libraryCoordinator.saveReference(
        reference,
        undefined,
        undefined,
        i18n.global.t('search.toasts.savedToQuickSaves', {
          title: reference.title || i18n.global.t('search.fallbackImageTitle'),
        }),
      )
      return true
    } catch {
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
    await libraryCoordinator.saveReferences(items.map(toLibraryReference))
    clearSelection()
  }

  function markAssetSaved(key: string): void {
    feed.markSavedKey(key)
  }

  function markAssetUnsaved(key: string): void {
    feed.markUnsavedKey(key)
  }

  async function submitQuery(): Promise<void> {
    const query = normalizeSearchQuery(draftQuery.value)
    if (!isSearchQueryWithinLimit(query)) {
      queryError.value = i18n.global.t('search.queryTooLong')
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
    // Replacing the same URL does not trigger the query watcher. Resubmission
    // explicitly retries the current URL-backed feed, including an empty query.
    if (query === queryValue()) {
      await feed.load(true)
      return
    }
    await router.replace({ path: USER_SEARCH_PATH, query: query ? { q: query } : {} })
  }

  watch(draftQuery, (value) => {
    if (value.trim() !== '' || vision.references.value.length > 0 || submittedQuery.value === '')
      return
    submittedQuery.value = ''
    if (stringParam(route.query.q)) {
      void router.replace({ path: USER_SEARCH_PATH, query: {} })
      return
    }
    void feed.load(true)
  })

  watch(feed.results, (items) => {
    clearSelection()
    const byKey = new Map(items.map((item) => [searchReferenceKey(item.provider, item.id), item]))
    vision.refreshReferencePresentation(items)
    const active = lightbox.activeResult.value
    if (active) {
      const fresh = byKey.get(searchReferenceKey(active.provider, active.id))
      if (fresh || isMediaDisplayExpired(active.display)) {
        lightbox.activeResult.value = { ...active, display: fresh?.display }
      }
    }
  })

  useFailureToast(vision.error, (message) => String(message ?? ''), { shouldFireOnMount: false })

  // Only empty-feed failures toast; retained results keep the inline hint.
  useFailureToast(
    () => feed.hasError.value && feed.results.value.length === 0,
    () => i18n.global.t('search.errors.unableToLoadInspiration'),
    { shouldFireOnMount: false },
  )

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
