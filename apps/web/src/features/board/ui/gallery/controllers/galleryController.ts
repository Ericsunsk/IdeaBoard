import { computed, readonly, ref, watch, type Ref } from 'vue'
import type { BoardRepository } from '../../../session/catalog/repository'
import type { CatalogEntry, CatalogSortMode, Catalog, CatalogNavigation } from '../styling/contract'
import type { BoardFeedback } from './useBoardFeedback'
import { createCommandHandlers } from './commandHandlers'
import { filterBoardsByRange, useGalleryRange, type BoardGalleryRange } from '../range/range'

type GalleryControllerDependencies = {
  catalog: BoardRepository
  scenes: Pick<Catalog['viewModel'], 'sceneMap' | 'workspaceId'>
  navigation: CatalogNavigation
  feedback: BoardFeedback
  warn: (...data: unknown[]) => void
}

function filterAndSortBoards(
  boards: readonly CatalogEntry[],
  keyword: string,
  sortMode: CatalogSortMode,
  range: BoardGalleryRange,
): CatalogEntry[] {
  // Range is a pure frontend derivation over the single canonical
  // authorized catalog: no refetch, no catalog overwrite, no permission
  // change. Search and sort apply within the selected range.
  const inRange = filterBoardsByRange(boards, range)
  const normalizedKeyword = keyword.toLowerCase()
  const filtered = inRange.filter((board) => {
    if (!normalizedKeyword) return true
    return board.name.toLowerCase().includes(normalizedKeyword)
  })
  if (sortMode === 'name') {
    return [...filtered].sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
  }
  return [...filtered].sort((a, b) => b.updatedAt - a.updatedAt)
}

function createBoardViews(dependencies: {
  boards: () => readonly CatalogEntry[]
  searchKeyword: Ref<string>
  sortMode: Ref<CatalogSortMode>
  galleryRange: Ref<BoardGalleryRange>
}) {
  const boards = computed<readonly CatalogEntry[]>(() => dependencies.boards())
  const visibleBoards = computed(() =>
    filterAndSortBoards(
      boards.value,
      dependencies.searchKeyword.value,
      dependencies.sortMode.value,
      dependencies.galleryRange.value,
    ),
  )
  const rangeCounts = computed(() => ({
    personal: filterBoardsByRange(boards.value, 'personal').length,
    team: filterBoardsByRange(boards.value, 'team').length,
  }))
  return { visibleBoards, rangeCounts }
}

function setupCatalogRefresh(refresh: {
  scope: Ref<string | null>
  isBoardsLoading: Ref<boolean>
  hasCatalogHydrationFailed: Ref<boolean>
  catalogRecoveryBoardId: Ref<string | null>
  initialize: (options?: { shouldForce?: boolean }) => Promise<void>
  reloadCatalog: () => Promise<void>
}): () => void {
  const FOCUS_RELOAD_THROTTLE_MS = 5_000
  let lastVisibleReloadAt = 0

  async function refreshOnVisible(): Promise<void> {
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return
    if (refresh.scope.value == null) return
    if (refresh.isBoardsLoading.value) return
    const now = Date.now()
    if (now - lastVisibleReloadAt < FOCUS_RELOAD_THROTTLE_MS) return
    lastVisibleReloadAt = now
    if (refresh.hasCatalogHydrationFailed.value || refresh.catalogRecoveryBoardId.value) {
      await refresh.reloadCatalog()
      return
    }
    await refresh.initialize({ shouldForce: true })
  }

  function handleVisibilityChange(): void {
    if (typeof document === 'undefined') return
    if (document.visibilityState !== 'visible') return
    void refreshOnVisible()
  }

  function handleWindowFocus(): void {
    void refreshOnVisible()
  }

  if (typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
  if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
    window.addEventListener('focus', handleWindowFocus)
  }

  const stopScopeWatch = watch(refresh.scope, (next, prev) => {
    if (next == null || next === prev) return
    if (prev == null) {
      if (!refresh.isBoardsLoading.value && !refresh.hasCatalogHydrationFailed.value) return
    }
    void refresh.initialize({ shouldForce: true })
  })

  return () => {
    stopScopeWatch()
    if (typeof document !== 'undefined' && typeof document.removeEventListener === 'function') {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
    if (typeof window !== 'undefined' && typeof window.removeEventListener === 'function') {
      window.removeEventListener('focus', handleWindowFocus)
    }
  }
}

export function createGalleryController(dependencies: GalleryControllerDependencies): Catalog {
  const activeMenuBoardId = ref<string | null>(null)
  const searchKeyword = ref('')
  const sortMode = ref<CatalogSortMode>('recent')
  const galleryRangeState = useGalleryRange()
  const renamingBoardId = ref<string | null>(null)
  const renameDraft = ref('')
  const catalogRecoveryBoardId = ref<string | null>(null)
  const isCatalogReloadPending = ref(false)
  const isDeleteDialogVisible = ref(false)
  const pendingDeleteBoard = ref<CatalogEntry | null>(null)
  const isDeletePending = ref(false)

  const { visibleBoards, rangeCounts } = createBoardViews({
    boards: () => dependencies.catalog.viewModel.boards.value,
    searchKeyword,
    sortMode,
    galleryRange: galleryRangeState.range,
  })

  const stopScopeWatch = watch(dependencies.catalog.viewModel.scope, () => {
    activeMenuBoardId.value = null
    clearRename()
    catalogRecoveryBoardId.value = null
    resetDeleteState()
    // The persisted range is isolated per User + Workspace: a scope switch
    // loads that scope's saved range (default personal), never the
    // previous scope's selection.
    galleryRangeState.reloadPersistedRange()
  })

  const commandHandlers = createCommandHandlers({
    ...dependencies,
    state: {
      activeMenuBoardId,
      renamingBoardId,
      renameDraft,
      catalogRecoveryBoardId,
      isCatalogReloadPending,
      pendingDeleteBoard,
      isDeletePending,
    },
    clearRename,
    resetDeleteState,
  })

  const { hydration, reloadCatalog } = commandHandlers

  async function createAndOpenBoard(): Promise<void> {
    const didOpen = await commandHandlers.createAndOpenBoard()
    // New boards are server-fixed to workspace_access none under the
    // caller, so they always land in the personal range: a successful
    // creation returns the gallery there whatever range it started from.
    // Creation sends a name-only body in every range; the current range
    // never changes the default permission.
    if (didOpen) galleryRangeState.setRange('personal')
  }

  function setSearchKeyword(value: string | undefined) {
    searchKeyword.value = (value ?? '').trim()
  }

  function setSortMode(mode: CatalogSortMode) {
    sortMode.value = mode
  }

  function toggleBoardMenu(boardId: string) {
    activeMenuBoardId.value = activeMenuBoardId.value === boardId ? null : boardId
  }

  function handleBoardMenuHidden(boardId: string) {
    if (activeMenuBoardId.value === boardId) {
      activeMenuBoardId.value = null
    }
  }

  function startRename(board: CatalogEntry) {
    activeMenuBoardId.value = null
    renamingBoardId.value = board.id
    renameDraft.value = board.name
  }

  function updateRenameDraft(nextName: string) {
    renameDraft.value = nextName
  }

  function clearRename() {
    renamingBoardId.value = null
    renameDraft.value = ''
  }

  function cancelRename() {
    clearRename()
  }

  function openBoardFromCard(boardId: string) {
    if (activeMenuBoardId.value) {
      activeMenuBoardId.value = null
      return
    }
    if (renamingBoardId.value) return
    dependencies.navigation.openBoard(boardId)
  }

  function requestDelete(board: CatalogEntry) {
    activeMenuBoardId.value = null
    pendingDeleteBoard.value = board
    isDeleteDialogVisible.value = true
  }

  function closeDeleteDialog() {
    isDeleteDialogVisible.value = false
  }

  function handleDeleteDialogHidden() {
    if (!isDeletePending.value) {
      pendingDeleteBoard.value = null
    }
  }

  function resetDeleteState() {
    isDeleteDialogVisible.value = false
    pendingDeleteBoard.value = null
  }

  const disposeRefresh = setupCatalogRefresh({
    scope: dependencies.catalog.viewModel.scope,
    ...hydration.viewModel,
    initialize: hydration.initialize,
    reloadCatalog,
  })

  return {
    viewModel: {
      visibleBoards,
      sceneMap: computed(() => dependencies.scenes.sceneMap.value),
      workspaceId: computed(() => dependencies.scenes.workspaceId.value),
      isBoardsLoading: hydration.viewModel.isBoardsLoading,
      hasCatalogHydrationFailed: hydration.viewModel.hasCatalogHydrationFailed,
      searchKeyword: readonly(searchKeyword),
      sortMode: readonly(sortMode),
      range: readonly(galleryRangeState.range),
      rangeCounts,
      activeMenuBoardId: readonly(activeMenuBoardId),
      renamingBoardId: readonly(renamingBoardId),
      renameDraft: readonly(renameDraft),
      catalogRecoveryBoardId: hydration.viewModel.catalogRecoveryBoardId,
      isCatalogReloadPending: hydration.viewModel.isCatalogReloadPending,
      isDeleteDialogVisible: readonly(isDeleteDialogVisible),
      pendingDeleteBoard: readonly(pendingDeleteBoard),
      isDeletePending: readonly(isDeletePending),
    },
    commands: {
      initialize: hydration.initialize,
      setSearchKeyword,
      setSortMode,
      setRange: galleryRangeState.setRange,
      toggleBoardMenu,
      handleBoardMenuHidden,
      createAndOpenBoard,
      duplicateBoard: commandHandlers.duplicateBoard,
      startRename,
      updateRenameDraft,
      commitRename: commandHandlers.commitRename,
      cancelRename,
      reloadCatalog,
      openBoardFromCard,
      requestDelete,
      closeDeleteDialog,
      handleDeleteDialogHidden,
      confirmDelete: commandHandlers.confirmDelete,
    },
    dispose() {
      hydration.dispose()
      stopScopeWatch()
      disposeRefresh()
    },
  }
}
