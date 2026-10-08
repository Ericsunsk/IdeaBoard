import {
  computed,
  effectScope,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
  type Ref,
} from 'vue'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { useAuthOverlay } from '../../auth'
import { useRoute, useRouter } from 'vue-router'
import {
  libraryAssetKey,
  loadLibraryFloatingIsland,
  readSearchReferenceTransfer,
  writeSearchReferenceTransfer,
} from '../../library'
import { type SearchAuthAction } from '../domain/authAction'
import {
  peekPendingSearchAuthAction,
  setPendingSearchAuthAction,
} from '../infrastructure/authContinuation'
import {
  createSearchLibraryCoordinator,
  provideSearchLibraryCoordinator,
} from '../infrastructure/libraryCoordinator'
import { useWorkspaceSelection } from '../../workspace'
import { type SearchResult, type VisionReference } from '../domain/models'
import { useSearchSession } from './useSearchSession'
import {
  isSavePopoverController,
  type SearchSavePopoverController,
  type SearchSavePopoverRequest,
} from '../presentation/browser/savePopoverContract'
import {
  attachSearchDragImage,
  resolveDragAnchor,
  SEARCH_LIBRARY_AUTO_OPEN_DELAY_MS,
} from '../presentation/browser/dragGhost'

type SearchScreenRuntimeOptions = {
  userPublicID: Readonly<Ref<string | undefined>>
  isAuthenticated: () => boolean
  scrollContainer: Ref<HTMLElement | null>
}

type SearchSession = ReturnType<typeof useSearchSession>

type ScopedSearchSession = {
  session: SearchSession
  stop: () => void
}

function createSearchScreenView(
  session: () => SearchSession,
  libraryCoordinator: ReturnType<typeof createSearchLibraryCoordinator>,
  libraryIslandMounted: Ref<boolean>,
  isTriggerHoveredDuringDrag: Ref<boolean>,
) {
  return {
    search: {
      draftQuery: computed({
        get: () => session().query.draftQuery.value,
        set: (value: string) => {
          session().query.draftQuery.value = value
        },
      }),
      queryError: computed({
        get: () => session().query.queryError.value,
        set: (value: string) => {
          session().query.queryError.value = value
        },
      }),
      heading: computed(() => session().query.heading.value),
      references: computed(() => session().vision.references.value),
      referenceSlotCount: computed(() => session().vision.referenceSlotCount.value),
      canAddReference: computed(() => session().vision.canAddReference.value),
      visionState: computed(() => session().vision.state.value),
      fileInput: computed({
        get: () => session().vision.fileInput.value,
        set: (value: HTMLInputElement | null) => {
          session().vision.fileInput.value = value
        },
      }),
      buttonTitle: computed(() => session().vision.buttonTitle.value),
    },
    results: {
      items: computed(() => session().feed.results.value),
      hasMore: computed(() => session().feed.hasMore.value),
      loading: computed(() => session().feed.loading.value),
      loadingMore: computed(() => session().feed.loadingMore.value),
      hasError: computed(() => session().feed.hasError.value),
      failedImages: computed(() => session().feed.failedImages.value),
      savedKeys: computed(() => session().feed.savedKeys.value),
    },
    library: {
      open: computed({
        get: () => session().library.open.value,
        set: (value: boolean) => {
          session().library.open.value = value
        },
      }),
      expanded: computed({
        get: () => session().library.expanded.value,
        set: (value: boolean) => {
          session().library.expanded.value = value
        },
      }),
      folderFacts: libraryCoordinator.folderFacts,
      mutationPublisher: libraryCoordinator,
      mounted: libraryIslandMounted,
      isDropTarget: isTriggerHoveredDuringDrag,
    },
    selection: {
      count: computed(() => session().batch.selected.value.length),
    },
    preview: {
      visible: computed(() => session().lightbox.visible.value),
      activeResult: computed(() => session().lightbox.activeResult.value),
    },
  }
}

function createSearchSessionCommands(session: () => SearchSession) {
  return {
    search: {
      clear() {
        session().clearSearch()
      },
      async submit() {
        session().batch.clear()
        await session().submitQuery()
      },
      openFilePicker() {
        session().vision.openFilePicker()
      },
      handleFileInput(event: Event) {
        session().vision.handleFileInput(event)
      },
      removeReference(key: string) {
        session().vision.removeReference(key)
      },
      async useResultAsReference(result: SearchResult) {
        await session().vision.addSearchResult(result)
      },
      useLibraryReference(asset: Parameters<SearchSession['vision']['addLibraryAsset']>[0]) {
        session().vision.addLibraryAsset(asset)
      },
    },
    results: {
      reload() {
        return session().feed.reload()
      },
      loadNextPage() {
        return session().feed.load(false)
      },
      recordScroll(scrollY: number) {
        session().feed.recordScroll(scrollY)
      },
      markImageError(id: string) {
        session().feed.markImageError(id)
      },
      retryImage(id: string) {
        session().feed.retryImage(id)
      },
      imageRetryCount(id: string) {
        return session().feed.imageRetryCount(id)
      },
      openPreview(result: SearchResult) {
        session().lightbox.open(result)
      },
      closePreview() {
        session().lightbox.close()
      },
    },
    selection: {
      isSelected(result: SearchResult) {
        return session().batch.isSelected(result)
      },
      toggle(result: SearchResult) {
        session().batch.toggle(result)
      },
      clear() {
        session().batch.clear()
      },
      async useAsVisionReferences() {
        await session().batch.applyVisionReferences()
      },
      async saveAllToLibrary() {
        await session().batch.saveAllToLibrary()
      },
    },
  }
}

/** Owns the complete Search screen lifetime for one identity scope at a time. */
export function useSearchScreenRuntime(options: SearchScreenRuntimeOptions) {
  const authOverlay = useAuthOverlay()
  const route = useRoute()
  const router = useRouter()
  const toast = useAppToast()
  // Public-for-anonymous page: never fire the workspace list without a
  // session — an anonymous 401 would reload the whole page to login via
  // the shared auth-failure handler. The getter re-enables after login.
  const { selectedId: selectedWorkspaceId } = useWorkspaceSelection({
    enabled: () => options.isAuthenticated(),
  })
  const workspaceIdSource = () => selectedWorkspaceId.value
  const libraryCoordinator = createSearchLibraryCoordinator({ workspaceId: workspaceIdSource })
  provideSearchLibraryCoordinator(libraryCoordinator)
  const scopeKey = computed(() => options.userPublicID.value ?? 'anonymous')
  const current = shallowRef(createScopedSession(scopeKey.value))
  const mounted = ref(false)
  const libraryIslandMounted = ref(current.value.session.library.open.value)
  const isTriggerHoveredDuringDrag = ref(false)
  const savePopover = shallowRef<SearchSavePopoverController | null>(null)
  const savePopoverAnchor = ref<HTMLElement | null>(null)
  let autoOpenTimer: ReturnType<typeof setTimeout> | null = null
  const unsubscribeLibraryMutation = libraryCoordinator.subscribe((outcome) => {
    if (outcome.kind === 'asset-saved') {
      session().library.markAssetSaved(outcome.assetKey)
      return
    }
    if (outcome.kind === 'asset-removed') {
      session().library.markAssetUnsaved(outcome.assetKey)
    }
  })

  function session(): SearchSession {
    return current.value.session
  }

  function requireLogin(action: SearchAuthAction) {
    setPendingSearchAuthAction(action)
    authOverlay.commands.open('login')
  }

  function createScopedSession(scope: string, initialVisionReferences: VisionReference[] = []) {
    const sessionScope = effectScope()
    const scopedSession = sessionScope.run(() =>
      useSearchSession({
        cacheScope: scope,
        scrollContainer: options.scrollContainer,
        isAuthenticated: options.isAuthenticated,
        requireLogin,
        libraryCoordinator,
        initialVisionReferences,
        route,
        router,
        toast,
        workspaceId: workspaceIdSource,
      }),
    )
    if (!scopedSession) throw new Error('Failed to create Search session')
    return {
      session: scopedSession,
      stop() {
        scopedSession.dispose()
        sessionScope.stop()
      },
    } satisfies ScopedSearchSession
  }

  function clearAutoOpenTimer() {
    if (!autoOpenTimer) return
    clearTimeout(autoOpenTimer)
    autoOpenTimer = null
  }

  function clearSavePopoverAnchor() {
    savePopoverAnchor.value = null
  }

  function closeSavePopover() {
    savePopover.value?.hide()
    clearSavePopoverAnchor()
  }

  function replaceScopedSession(nextScope: string, previousScope: string) {
    const previous = current.value
    const shouldContinueVisionSearch =
      previousScope === 'anonymous' &&
      nextScope !== 'anonymous' &&
      peekPendingSearchAuthAction()?.kind === 'vision-search'
    const transferredReferences = shouldContinueVisionSearch
      ? previous.session.vision.transferReferences()
      : []

    closeSavePopover()
    clearAutoOpenTimer()
    isTriggerHoveredDuringDrag.value = false
    libraryCoordinator.reset()
    libraryIslandMounted.value = false
    previous.stop()
    current.value = createScopedSession(nextScope, transferredReferences)

    if (mounted.value) {
      session().activate()
      if (shouldContinueVisionSearch) void session().resumePendingAction()
    }
  }

  const view = createSearchScreenView(
    session,
    libraryCoordinator,
    libraryIslandMounted,
    isTriggerHoveredDuringDrag,
  )
  const workspaceId = computed(() => selectedWorkspaceId.value)

  const commands = {
    ...createSearchSessionCommands(session),
    library: {
      toggleForUser() {
        if (!options.isAuthenticated()) {
          requireLogin({ kind: 'open-library' })
          return
        }
        session().library.toggle()
      },
      preload() {
        void loadLibraryFloatingIsland()
      },
      close() {
        session().library.close()
      },
      openSavePopover(event: Event, result: SearchResult) {
        const anchor = resolveDragAnchor(event)
        if (!anchor) return
        savePopoverAnchor.value = anchor
        const request: SearchSavePopoverRequest = {
          result,
          saved: session().feed.savedKeys.value.has(libraryAssetKey(result.provider, result.id)),
          authenticated: options.isAuthenticated(),
        }
        savePopover.value?.toggle(event, request)
      },
      requestSaveAuthentication(result: SearchResult) {
        requireLogin({
          kind: 'save-reference',
          reference: session().toLibraryReference(result),
        })
      },
      registerSavePopover(instance: unknown) {
        savePopover.value = isSavePopoverController(instance) ? instance : null
      },
      setSavePopoverAnchor(anchor: HTMLElement | null) {
        savePopoverAnchor.value = anchor
      },
      handleAnchorHidden() {
        closeSavePopover()
      },
      handleCardDragStart(event: DragEvent, result: SearchResult) {
        const dataTransfer = event.dataTransfer
        if (!dataTransfer) return
        writeSearchReferenceTransfer(dataTransfer, session().toLibraryReference(result))
        attachSearchDragImage(dataTransfer, result)
      },
      handleCardDragEnd() {
        clearAutoOpenTimer()
        isTriggerHoveredDuringDrag.value = false
      },
      handleTriggerDragOver(event: DragEvent) {
        if (!options.isAuthenticated() || !event.dataTransfer) return
        event.dataTransfer.dropEffect = 'copy'
        isTriggerHoveredDuringDrag.value = true
        if (!session().library.open.value && !autoOpenTimer) {
          autoOpenTimer = setTimeout(() => {
            session().library.open.value = true
            autoOpenTimer = null
          }, SEARCH_LIBRARY_AUTO_OPEN_DELAY_MS)
        }
      },
      handleTriggerDragLeave() {
        isTriggerHoveredDuringDrag.value = false
        clearAutoOpenTimer()
      },
      async handleTriggerDrop(event: DragEvent) {
        isTriggerHoveredDuringDrag.value = false
        clearAutoOpenTimer()
        const reference = readSearchReferenceTransfer(event.dataTransfer)
        if (!reference) return
        if (!options.isAuthenticated()) {
          requireLogin({ kind: 'save-reference', reference })
          return
        }
        await session().library.saveReferenceToQuickSaves(reference)
      },
    },
  }

  watch(view.library.open, (open) => {
    if (open) libraryIslandMounted.value = true
  })
  watch(selectedWorkspaceId, (next, previous) => {
    if (next !== previous) libraryCoordinator.reset()
  })
  watch(
    scopeKey,
    (nextScope, previousScope) => {
      if (nextScope !== previousScope) replaceScopedSession(nextScope, previousScope)
    },
    { flush: 'sync' },
  )

  onMounted(() => {
    mounted.value = true
    session().activate()
    void session().resumePendingAction()
  })

  onBeforeUnmount(() => {
    clearAutoOpenTimer()
    closeSavePopover()
    current.value.stop()
    unsubscribeLibraryMutation()
    libraryCoordinator.dispose()
  })

  return { view, commands, savePopoverAnchor, workspaceId }
}
