import { resolveMediaDisplayUrl } from '../../../core/media/mediaDisplay'
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
import { useAppToast } from '../../../core/composables/useAppToast'
import { useAuthOverlay } from '../../auth'
import { useRoute, useRouter } from 'vue-router'
import {
  libraryAssetKeyFor,
  loadLibraryFloatingIsland,
  type LibraryDragPayload,
} from '../../library'
import { type SearchAuthAction } from '../core/authAction'
import {
  peekPendingSearchAuthAction,
  setPendingSearchAuthAction,
} from './adapters/authContinuation'
import {
  createSearchLibraryCoordinator,
  provideSearchLibraryCoordinator,
} from './adapters/libraryCoordinator'
import { useWorkspaceSelection } from '../../workspace'
import { type SearchResult, type VisionReference } from '../core/models'
import { useSearchSession } from './useSearchSession'
import {
  isSavePopoverController,
  resolveSavePopoverAnchor,
  type SearchSavePopoverController,
  type SearchSavePopoverRequest,
} from '../ui/browser/savePopoverContract'
import { startPointerDrag } from '../../../core/interaction/pointerDrag'
import { createPointerDropTargets } from '../../../core/interaction/pointerDropTargets'

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
      isLoading: computed(() => session().feed.loading.value),
      isLoadingMore: computed(() => session().feed.loadingMore.value),
      hasError: computed(() => session().feed.hasError.value),
      failedImages: computed(() => session().feed.failedImages.value),
      savedKeys: computed(() => session().feed.savedKeys.value),
    },
    library: {
      isOpen: computed({
        get: () => session().library.open.value,
        set: (isOpen: boolean) => {
          session().library.open.value = isOpen
        },
      }),
      isExpanded: computed({
        get: () => session().library.expanded.value,
        set: (isExpanded: boolean) => {
          session().library.expanded.value = isExpanded
        },
      }),
      folderFacts: libraryCoordinator.folderFacts,
      mutationPublisher: libraryCoordinator,
      saveModule: libraryCoordinator.saveModule,
      isMounted: libraryIslandMounted,
      isDropTarget: isTriggerHoveredDuringDrag,
    },
    selection: {
      count: computed(() => session().batch.selected.value.length),
    },
    preview: {
      isVisible: computed(() => session().lightbox.visible.value),
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
export function screenRuntime(options: SearchScreenRuntimeOptions) {
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
  const isMounted = ref(false)
  const isLibraryIslandMounted = ref(current.value.session.library.open.value)
  const isTriggerHoveredDuringDrag = ref(false)
  const dropTargets = createPointerDropTargets<LibraryDragPayload>()
  let unregisterTrigger: (() => void) | null = null
  let triggerElement: HTMLElement | null = null
  const savePopover = shallowRef<SearchSavePopoverController | null>(null)
  const savePopoverAnchor = ref<HTMLElement | null>(null)
  let autoOpenTimer: ReturnType<typeof setTimeout> | null = null
  const unsubscribeLibraryMutation = libraryCoordinator.subscribe((outcomes) => {
    for (const outcome of outcomes) {
      if (outcome.kind === 'asset-saved') session().library.markAssetSaved(outcome.assetKey)
      if (outcome.kind === 'asset-removed') session().library.markAssetUnsaved(outcome.assetKey)
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
    dropTargets.cancel()
    clearAutoOpenTimer()
    isTriggerHoveredDuringDrag.value = false
    libraryCoordinator.reset()
    isLibraryIslandMounted.value = false
    previous.stop()
    current.value = createScopedSession(nextScope, transferredReferences)

    if (isMounted.value) {
      session().activate()
      if (shouldContinueVisionSearch) void session().resumePendingAction()
    }
  }

  const view = createSearchScreenView(
    session,
    libraryCoordinator,
    isLibraryIslandMounted,
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
        const anchor = resolveSavePopoverAnchor(event)
        if (!anchor) return
        savePopoverAnchor.value = anchor
        const request: SearchSavePopoverRequest = {
          result,
          isSaved: session().feed.savedKeys.value.has(
            libraryAssetKeyFor({
              source: 'external',
              provider: result.provider,
              sourceId: result.id,
            }),
          ),
          isAuthenticated: options.isAuthenticated(),
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
      handleCardPointerDown(event: PointerEvent, result: SearchResult) {
        startPointerDrag(event, {
          payload: { kind: 'reference', reference: session().toLibraryReference(result) },
          previewUrl: resolveMediaDisplayUrl(result.display, 256),
          targets: dropTargets,
          onEnd: () => {
            clearAutoOpenTimer()
            isTriggerHoveredDuringDrag.value = false
          },
        })
      },
      registerTrigger(element: unknown) {
        const next = element instanceof HTMLElement ? element : null
        if (triggerElement === next) return
        unregisterTrigger?.()
        unregisterTrigger = null
        triggerElement = next
        if (!next) return
        unregisterTrigger = dropTargets.register({
          element: next,
          canReceive: (payload) => payload.kind === 'reference',
          onHoverChange(isHovered) {
            isTriggerHoveredDuringDrag.value = isHovered && options.isAuthenticated()
            clearAutoOpenTimer()
            if (isTriggerHoveredDuringDrag.value && !session().library.open.value) {
              autoOpenTimer = setTimeout(() => {
                session().library.open.value = true
                autoOpenTimer = null
              }, 280)
            }
          },
          onDrop(payload) {
            if (payload.kind !== 'reference') return
            const { reference } = payload
            if (!options.isAuthenticated()) {
              requireLogin({ kind: 'save-reference', reference })
              return
            }
            void session().library.saveReferenceToQuickSaves(reference)
          },
        })
      },
    },
  }

  watch(view.library.isOpen, (isOpen) => {
    if (isOpen) isLibraryIslandMounted.value = true
  })
  watch(
    selectedWorkspaceId,
    (next, previous) => {
      if (next !== previous) {
        dropTargets.cancel()
        libraryCoordinator.reset()
      }
    },
    { flush: 'sync' },
  )
  watch(
    scopeKey,
    (nextScope, previousScope) => {
      if (nextScope !== previousScope) replaceScopedSession(nextScope, previousScope)
    },
    { flush: 'sync' },
  )

  onMounted(() => {
    isMounted.value = true
    session().activate()
    void session().resumePendingAction()
  })

  onBeforeUnmount(() => {
    dropTargets.cancel()
    unregisterTrigger?.()
    clearAutoOpenTimer()
    closeSavePopover()
    current.value.stop()
    unsubscribeLibraryMutation()
    libraryCoordinator.dispose()
  })

  return { view, commands, savePopoverAnchor, workspaceId, dropTargets }
}
