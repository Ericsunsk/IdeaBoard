import { computed, readonly, ref, watch, type Ref } from 'vue'
import type { BoardRepository } from '../../session/catalog/repository'
import type { CatalogEntry, CatalogSortMode, Catalog, CatalogNavigation } from './contract'
import type { BoardFeedback } from './useBoardFeedback'
import { createCommandHandlers } from './commandHandlers'

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
): CatalogEntry[] {
  const normalizedKeyword = keyword.toLowerCase()
  const filtered = boards.filter((board) => {
    if (!normalizedKeyword) return true
    return board.name.toLowerCase().includes(normalizedKeyword)
  })
  if (sortMode === 'name') {
    return [...filtered].sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
  }
  return [...filtered].sort((a, b) => b.updatedAt - a.updatedAt)
}

function setupCatalogRefresh(refresh: {
  scope: Ref<string | null>
  isBoardsLoading: Ref<boolean>
  catalogHydrationFailed: Ref<boolean>
  catalogRecoveryBoardId: Ref<string | null>
  initialize: (options?: { force?: boolean }) => Promise<void>
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
    if (refresh.catalogHydrationFailed.value || refresh.catalogRecoveryBoardId.value) {
      await refresh.reloadCatalog()
      return
    }
    await refresh.initialize({ force: true })
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
      if (!refresh.isBoardsLoading.value && !refresh.catalogHydrationFailed.value) return
    }
    void refresh.initialize({ force: true })
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
  const renamingBoardId = ref<string | null>(null)
  const renameDraft = ref('')
  const catalogRecoveryBoardId = ref<string | null>(null)
  const isCatalogReloadPending = ref(false)
  const deleteDialogVisible = ref(false)
  const pendingDeleteBoard = ref<CatalogEntry | null>(null)
  const isDeletePending = ref(false)
  let suppressNextBoardOpen = false

  const boards = computed<readonly CatalogEntry[]>(
    () => dependencies.catalog.viewModel.boards.value,
  )
  const visibleBoards = computed(() =>
    filterAndSortBoards(boards.value, searchKeyword.value, sortMode.value),
  )

  const stopScopeWatch = watch(dependencies.catalog.viewModel.scope, () => {
    activeMenuBoardId.value = null
    clearRename(false)
    catalogRecoveryBoardId.value = null
    resetDeleteState()
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

  function clearRename(suppressNextOpen: boolean) {
    renamingBoardId.value = null
    renameDraft.value = ''
    suppressNextBoardOpen = suppressNextOpen
  }

  function cancelRename() {
    clearRename(true)
  }

  function openBoardFromCard(boardId: string) {
    if (activeMenuBoardId.value) {
      activeMenuBoardId.value = null
      return
    }
    if (renamingBoardId.value || suppressNextBoardOpen) {
      suppressNextBoardOpen = false
      return
    }
    dependencies.navigation.openBoard(boardId)
  }

  function requestDelete(board: CatalogEntry) {
    activeMenuBoardId.value = null
    pendingDeleteBoard.value = board
    deleteDialogVisible.value = true
  }

  function closeDeleteDialog() {
    deleteDialogVisible.value = false
  }

  function handleDeleteDialogHidden() {
    if (!isDeletePending.value) {
      pendingDeleteBoard.value = null
    }
  }

  function resetDeleteState() {
    deleteDialogVisible.value = false
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
      catalogHydrationFailed: hydration.viewModel.catalogHydrationFailed,
      searchKeyword: readonly(searchKeyword),
      sortMode: readonly(sortMode),
      activeMenuBoardId: readonly(activeMenuBoardId),
      renamingBoardId: readonly(renamingBoardId),
      renameDraft: readonly(renameDraft),
      catalogRecoveryBoardId: hydration.viewModel.catalogRecoveryBoardId,
      isCatalogReloadPending: hydration.viewModel.isCatalogReloadPending,
      deleteDialogVisible: readonly(deleteDialogVisible),
      pendingDeleteBoard: readonly(pendingDeleteBoard),
      isDeletePending: readonly(isDeletePending),
    },
    commands: {
      initialize: hydration.initialize,
      setSearchKeyword,
      setSortMode,
      toggleBoardMenu,
      handleBoardMenuHidden,
      createAndOpenBoard: commandHandlers.createAndOpenBoard,
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
