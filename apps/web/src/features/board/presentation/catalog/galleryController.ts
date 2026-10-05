import { computed, readonly, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { BoardRepository } from '../../application/catalog/repository'
import type { BoardPreviewRefreshQueue } from '../../infrastructure/preview'
import type { CatalogEntry, CatalogSortMode, Catalog, CatalogNavigation } from './contract'
import type { BoardFeedback } from './useBoardFeedback'
import { createCommandHandlers } from './commandHandlers'

type GalleryControllerDependencies = {
  catalog: BoardRepository
  preview: BoardPreviewRefreshQueue
  navigation: CatalogNavigation
  feedback: BoardFeedback
  warn: (...data: unknown[]) => void
  theme?: { isDark: Ref<boolean> | ComputedRef<boolean> }
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

function setupCatalogWatchers(
  boards: ComputedRef<readonly CatalogEntry[]>,
  scope: Ref<string | null>,
  isDarkRef: Ref<boolean> | ComputedRef<boolean> | undefined,
  preview: BoardPreviewRefreshQueue,
  onScopeReset: () => void,
) {
  const stopPreviewWatch = watch(
    boards,
    (nextBoards) => {
      const snapshot = [...nextBoards]
      preview.reloadFromLocal(snapshot)
      preview.schedule(snapshot)
    },
    { immediate: true },
  )

  const stopThemeWatch = isDarkRef
    ? watch(isDarkRef, (isDark) => {
        preview.handleThemeChange?.(isDark, [...boards.value])
      })
    : undefined

  const stopScopeWatch = watch(scope, () => {
    preview.invalidate()
    onScopeReset()
  })

  return () => {
    stopPreviewWatch()
    stopScopeWatch()
    stopThemeWatch?.()
    preview.dispose()
  }
}

export function createGalleryController(dependencies: GalleryControllerDependencies): Catalog {
  const activeMenuBoardId = ref<string | null>(null)
  const isBoardsLoading = ref(true)
  const catalogHydrationFailed = ref(false)
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
  let initializePromise: Promise<void> | null = null

  const boards = computed<readonly CatalogEntry[]>(
    () => dependencies.catalog.viewModel.boards.value,
  )
  const visibleBoards = computed(() =>
    filterAndSortBoards(boards.value, searchKeyword.value, sortMode.value),
  )

  const disposeWatchers = setupCatalogWatchers(
    boards,
    dependencies.catalog.viewModel.scope,
    dependencies.theme?.isDark,
    dependencies.preview,
    () => {
      activeMenuBoardId.value = null
      clearRename(false)
      catalogRecoveryBoardId.value = null
      resetDeleteState()
    },
  )

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

  function initialize(options: { force?: boolean } = {}): Promise<void> {
    if (options.force) initializePromise = null
    if (initializePromise) return initializePromise
    initializePromise = (async () => {
      try {
        const result = await dependencies.catalog.commands.hydrate()
        if (!result.ok) {
          catalogHydrationFailed.value = true
          dependencies.feedback.catalogSyncFailed()
          return
        }
        catalogHydrationFailed.value = false
      } catch (error) {
        catalogHydrationFailed.value = true
        dependencies.warn('[boards] failed to hydrate catalog', error)
        dependencies.feedback.catalogSyncFailed()
      } finally {
        isBoardsLoading.value = false
      }
    })()
    return initializePromise
  }

  async function reloadCatalog() {
    if (catalogHydrationFailed.value && !catalogRecoveryBoardId.value) {
      if (isCatalogReloadPending.value) return
      isCatalogReloadPending.value = true
      try {
        await initialize({ force: true })
        if (!catalogHydrationFailed.value) dependencies.feedback.catalogReloaded()
      } finally {
        isCatalogReloadPending.value = false
      }
      return
    }
    await commandHandlers.reloadCatalog()
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

  return {
    viewModel: {
      visibleBoards,
      previewMap: readonly(dependencies.preview.previewMap),
      isBoardsLoading: readonly(isBoardsLoading),
      catalogHydrationFailed: readonly(catalogHydrationFailed),
      searchKeyword: readonly(searchKeyword),
      sortMode: readonly(sortMode),
      activeMenuBoardId: readonly(activeMenuBoardId),
      renamingBoardId: readonly(renamingBoardId),
      renameDraft: readonly(renameDraft),
      catalogRecoveryBoardId: readonly(catalogRecoveryBoardId),
      isCatalogReloadPending: readonly(isCatalogReloadPending),
      deleteDialogVisible: readonly(deleteDialogVisible),
      pendingDeleteBoard: readonly(pendingDeleteBoard),
      isDeletePending: readonly(isDeletePending),
    },
    commands: {
      initialize,
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
      disposeWatchers()
    },
  }
}
