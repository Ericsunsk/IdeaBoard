import { computed, readonly, ref, watch } from 'vue'
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
}

export function createGalleryController(dependencies: GalleryControllerDependencies): Catalog {
  const activeMenuBoardId = ref<string | null>(null)
  const isBoardsLoading = ref(true)
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
  const visibleBoards = computed(() => {
    const keyword = searchKeyword.value.toLowerCase()
    const filtered = boards.value.filter((board) => {
      if (!keyword) return true
      return board.name.toLowerCase().includes(keyword)
    })
    if (sortMode.value === 'name') {
      return [...filtered].sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
    }
    return [...filtered].sort((a, b) => b.updatedAt - a.updatedAt)
  })

  const stopPreviewWatch = watch(
    boards,
    (nextBoards) => {
      const snapshot = [...nextBoards]
      dependencies.preview.reloadFromLocal(snapshot)
      dependencies.preview.schedule(snapshot)
    },
    { immediate: true },
  )

  const stopScopeWatch = watch(dependencies.catalog.viewModel.scope, () => {
    dependencies.preview.invalidate()
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

  function initialize(): Promise<void> {
    if (initializePromise) return initializePromise
    initializePromise = (async () => {
      try {
        const result = await dependencies.catalog.commands.hydrate()
        if (!result.ok) {
          dependencies.feedback.catalogSyncFailed()
        }
      } catch (error) {
        dependencies.warn('[boards] failed to hydrate catalog', error)
        dependencies.feedback.catalogSyncFailed()
      } finally {
        isBoardsLoading.value = false
      }
    })()
    return initializePromise
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
      reloadCatalog: commandHandlers.reloadCatalog,
      openBoardFromCard,
      requestDelete,
      closeDeleteDialog,
      handleDeleteDialogHidden,
      confirmDelete: commandHandlers.confirmDelete,
    },
    dispose() {
      stopPreviewWatch()
      stopScopeWatch()
      dependencies.preview.dispose()
    },
  }
}
