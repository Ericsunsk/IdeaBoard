import { computed, readonly, ref, watch } from 'vue'
import type { CanvasBoardCatalog } from '../../canvas-board/catalog/canvasBoardCatalog'
import type { BoardPreviewRefreshQueue } from '../../canvas-board/preview'
import type {
  BoardCatalogEntry,
  BoardCatalogSortMode,
  BoardsCatalog,
  BoardsCatalogNavigation,
} from '../boardCatalogContract'
import type { BoardFeedback } from '../feedback/useBoardFeedback'

type BoardCatalogControllerDependencies = {
  catalog: CanvasBoardCatalog
  preview: BoardPreviewRefreshQueue
  navigation: BoardsCatalogNavigation
  feedback: BoardFeedback
  warn: (...data: unknown[]) => void
}

export function createBoardCatalogController(
  dependencies: BoardCatalogControllerDependencies,
): BoardsCatalog {
  const activeMenuBoardId = ref<string | null>(null)
  const isBoardsLoading = ref(true)
  const searchKeyword = ref('')
  const sortMode = ref<BoardCatalogSortMode>('recent')
  const renamingBoardId = ref<string | null>(null)
  const renameDraft = ref('')
  const catalogRecoveryBoardId = ref<string | null>(null)
  const isCatalogReloadPending = ref(false)
  const deleteDialogVisible = ref(false)
  const pendingDeleteBoard = ref<BoardCatalogEntry | null>(null)
  const isDeletePending = ref(false)
  let suppressNextBoardOpen = false
  let initializePromise: Promise<void> | null = null

  const boards = computed<readonly BoardCatalogEntry[]>(
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

  function setSortMode(mode: BoardCatalogSortMode) {
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

  async function createAndOpenBoard() {
    activeMenuBoardId.value = null
    try {
      const result = await dependencies.catalog.commands.create()
      if (!result.ok) {
        if (result.reason === 'scope-changed') return
        dependencies.feedback.createFailed(result)
        dependencies.warn('[boards] failed to sync new board', result.error)
        return
      }
      dependencies.navigation.openBoard(result.board.id)
    } catch (error) {
      dependencies.warn('[boards] failed to create board', error)
      dependencies.feedback.catalogSyncFailed()
    }
  }

  async function duplicateBoard(boardId: string) {
    activeMenuBoardId.value = null
    try {
      const result = await dependencies.catalog.commands.duplicate({ sourceBoardId: boardId })
      if (!result.ok) {
        if (result.reason === 'scope-changed') return
        dependencies.feedback.copyFailed(result)
        if (result.reason !== 'source-missing') {
          dependencies.warn('[boards] failed to sync duplicated board', result.error)
        }
        return
      }
      dependencies.feedback.copied()
    } catch (error) {
      dependencies.warn('[boards] failed to duplicate board', error)
      dependencies.feedback.catalogSyncFailed()
    }
  }

  function startRename(board: BoardCatalogEntry) {
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

  async function commitRename() {
    const boardId = renamingBoardId.value
    if (!boardId) return
    const result = await dependencies.catalog.commands.rename({
      boardId,
      nextName: renameDraft.value,
    })
    if (!result.ok) {
      switch (result.reason) {
        case 'scope-changed':
          return
        case 'local-name-conflict':
          dependencies.feedback.renameValidationFailed(result)
          return
        case 'invalid-name':
          clearRename(true)
          dependencies.feedback.renameValidationFailed(result)
          return
        case 'remote-name-conflict':
        case 'sync-failed':
          clearRename(true)
          dependencies.feedback.renameSyncFailed(result)
          catalogRecoveryBoardId.value =
            result.reason === 'remote-name-conflict'
              ? result.hydrate?.ok === true
                ? null
                : boardId
              : result.rollback.ok
                ? null
                : boardId
          dependencies.warn('[boards] failed to sync renamed board', result.error)
          return
      }
    }
    clearRename(true)
    dependencies.feedback.renamed()
  }

  async function reloadCatalog() {
    const boardId = catalogRecoveryBoardId.value
    if (!boardId || isCatalogReloadPending.value) return
    isCatalogReloadPending.value = true
    try {
      const result = await dependencies.catalog.commands.hydrate({
        preserveLocalNewer: false,
        rollbackBoardIds: [boardId],
      })
      if (!result.ok) {
        dependencies.feedback.catalogSyncFailed()
        return
      }
      catalogRecoveryBoardId.value = null
      dependencies.feedback.catalogReloaded()
    } catch (error) {
      dependencies.warn('[boards] failed to reload catalog', error)
      dependencies.feedback.catalogSyncFailed()
    } finally {
      isCatalogReloadPending.value = false
    }
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

  function requestDelete(board: BoardCatalogEntry) {
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

  async function confirmDelete() {
    const board = pendingDeleteBoard.value
    if (!board || isDeletePending.value) return
    isDeletePending.value = true
    try {
      const result = await dependencies.catalog.commands.delete({ boardId: board.id })
      if (!result.ok) {
        if (result.reason === 'scope-changed') {
          resetDeleteState()
          return
        }
        dependencies.feedback.deleteFailed(result)
        if (result.reason === 'sync-failed') {
          dependencies.warn('[boards] failed to sync deleted board', result.error)
        } else {
          resetDeleteState()
        }
        return
      }
      if (renamingBoardId.value === board.id) {
        clearRename(false)
      }
      resetDeleteState()
      dependencies.feedback.deleted()
    } catch (error) {
      const failure = { ok: false as const, reason: 'sync-failed' as const, error }
      dependencies.feedback.deleteFailed(failure)
      dependencies.warn('[boards] failed to sync deleted board', error)
    } finally {
      isDeletePending.value = false
    }
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
      createAndOpenBoard,
      duplicateBoard,
      startRename,
      updateRenameDraft,
      commitRename,
      cancelRename,
      reloadCatalog,
      openBoardFromCard,
      requestDelete,
      closeDeleteDialog,
      handleDeleteDialogHidden,
      confirmDelete,
    },
    dispose() {
      stopPreviewWatch()
      stopScopeWatch()
      dependencies.preview.dispose()
    },
  }
}

export function formatBoardCatalogDate(timestamp: number): string {
  if (!Number.isFinite(timestamp)) return '-'
  const date = new Date(timestamp)
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}.${month}.${day}`
}
