import { computed, ref, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { USER_SEARCH_PATH } from '../../../shared/constants/routePaths'
import { stringParam } from '../../../shared/router/queryParams'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { i18n } from '../../../shared/i18n'
import { libraryAssetKey } from '../../library'
import type { LibrarySearchReference } from '../domain/authAction'
import {
  getLibraryIslandState,
  setLibraryIslandState,
} from '../infrastructure/libraryIslandPersistence'
import { type SearchResult, type VisionReference } from '../domain/models'
import {
  allowAnonymousSearchQuery,
  consumePendingSearchAuthAction,
  peekPendingSearchAuthAction,
} from '../infrastructure/authContinuation'
import { isSearchQueryWithinLimit, normalizeSearchQuery } from '../domain/queryPolicy'
import { useSearchLightbox } from '../presentation/useSearchLightbox'
import { createSearchFeed } from './feed'
import { useVisionQueryLifecycle } from './useVisionQueryLifecycle'
import { type SearchAuthAction } from '../domain/authAction'
import { type SearchLibraryCoordinator } from '../ports/libraryCoordinatorPort'

function useLibraryIslandState(cacheScope: string, isAuthenticated: () => boolean) {
  const islandState = getLibraryIslandState(cacheScope)
  // A persisted open state must never auto-mount the protected Library island
  // for an anonymous Search session. It would immediately issue 401 requests.
  const libraryIslandOpen = ref(isAuthenticated() && islandState.open)
  const libraryIslandExpanded = ref(isAuthenticated() && islandState.expanded)
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

  watch([libraryIslandOpen, libraryIslandExpanded], ([open, expanded]) => {
    setLibraryIslandState({ open, expanded }, cacheScope)
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
      const saved = await libraryCoordinator.saveReference(reference)
      feed.markSavedKey(libraryAssetKey(saved.asset.provider, saved.asset.sourceId))
      toast.success(
        i18n.global.t('search.toasts.savedToQuickSaves', {
          title: reference.title || i18n.global.t('search.fallbackImageTitle'),
        }),
      )
      return true
    } catch (error) {
      toast.error(userFacingErrorCopy(error, i18n.global.t('library.errors.saveAssetFailed')))
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
    for (const outcome of outcomes) {
      if (outcome.status !== 'fulfilled') continue
      feed.markSavedKey(libraryAssetKey(outcome.value.asset.provider, outcome.value.asset.sourceId))
      successCount += 1
    }
    const failureCount = outcomes.length - successCount
    if (failureCount === 0) {
      toast.success(i18n.global.t('search.toasts.savedBatchSuccess', { count: successCount }))
    } else if (successCount === 0) {
      toast.error(i18n.global.t('search.toasts.savedBatchFailed'))
    } else {
      toast.warning(
        i18n.global.t('search.toasts.savedBatchPartial', { successCount, failureCount }),
      )
    }
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
