import {
  computed,
  onMounted,
  onScopeDispose,
  readonly,
  ref,
  watch,
  type ComputedRef,
  type Ref,
} from 'vue'
import type { CanvasBoardCatalog } from '../../canvas-board/catalog/canvasBoardCatalog'
import type { CanvasBoardMeta } from '../../canvas-board/catalog/canvasBoards'
import type { BoardPreviewRefreshQueue } from '../../canvas-board/preview'
import type { BoardFeedback } from '../feedback/useBoardFeedback'

export type BoardCatalogNavigation = {
  openBoard(boardId: string): void
}

export type BoardCatalogSortMode = 'recent' | 'name'

export type BoardCatalogControllerDependencies = {
  catalog: CanvasBoardCatalog
  preview: BoardPreviewRefreshQueue
  navigation: BoardCatalogNavigation
  feedback: BoardFeedback
  warn: (...data: unknown[]) => void
}

export type BoardCatalogController = {
  viewModel: {
    boards: Readonly<Ref<readonly CanvasBoardMeta[]>>
    visibleBoards: ComputedRef<readonly CanvasBoardMeta[]>
    previewMap: Readonly<Ref<Record<string, string>>>
    isBoardsLoading: Readonly<Ref<boolean>>
    searchKeyword: Readonly<Ref<string>>
    sortMode: Readonly<Ref<BoardCatalogSortMode>>
    activeMenuBoardId: Readonly<Ref<string | null>>
    renamingBoardId: Readonly<Ref<string | null>>
    renameDraft: Readonly<Ref<string>>
    catalogRecoveryBoardId: Readonly<Ref<string | null>>
    isCatalogReloadPending: Readonly<Ref<boolean>>
    deleteDialogVisible: Readonly<Ref<boolean>>
    pendingDeleteBoard: Readonly<Ref<CanvasBoardMeta | null>>
    isDeletePending: Readonly<Ref<boolean>>
  }
  commands: {
    initialize(): Promise<void>
    setSearchKeyword(value: string | undefined): void
    setSortMode(mode: BoardCatalogSortMode): void
    toggleBoardMenu(boardId: string): void
    handleBoardMenuHidden(boardId: string): void
    createAndOpenBoard(): Promise<void>
    duplicateBoard(board: CanvasBoardMeta): Promise<void>
    startRename(board: CanvasBoardMeta): void
    updateRenameDraft(nextName: string): void
    commitRename(): Promise<void>
    cancelRename(): void
    reloadCatalog(): Promise<void>
    openBoardFromCard(board: CanvasBoardMeta): void
    requestDelete(board: CanvasBoardMeta): void
    closeDeleteDialog(): void
    handleDeleteDialogHidden(): void
    confirmDelete(): Promise<void>
  }
  dispose(): void
}

export function createBoardCatalogController(
  dependencies: BoardCatalogControllerDependencies,
): BoardCatalogController {
  const activeMenuBoardId = ref<string | null>(null)
  const isBoardsLoading = ref(true)
  const searchKeyword = ref('')
  const sortMode = ref<BoardCatalogSortMode>('recent')
  const renamingBoardId = ref<string | null>(null)
  const renameDraft = ref('')
  const catalogRecoveryBoardId = ref<string | null>(null)
  const isCatalogReloadPending = ref(false)
  const deleteDialogVisible = ref(false)
  const pendingDeleteBoard = ref<CanvasBoardMeta | null>(null)
  const isDeletePending = ref(false)
  let suppressNextBoardOpen = false
  let initializePromise: Promise<void> | null = null

  const boards = dependencies.catalog.viewModel.boards
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

  async function duplicateBoard(board: CanvasBoardMeta) {
    activeMenuBoardId.value = null
    try {
      const result = await dependencies.catalog.commands.duplicate({ sourceBoardId: board.id })
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

  function startRename(board: CanvasBoardMeta) {
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

  function openBoardFromCard(board: CanvasBoardMeta) {
    if (activeMenuBoardId.value) {
      activeMenuBoardId.value = null
      return
    }
    if (renamingBoardId.value || suppressNextBoardOpen) {
      suppressNextBoardOpen = false
      return
    }
    dependencies.navigation.openBoard(board.id)
  }

  function requestDelete(board: CanvasBoardMeta) {
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
      boards,
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

export function useBoardCatalogController(
  dependencies: BoardCatalogControllerDependencies,
): BoardCatalogController {
  const controller = createBoardCatalogController(dependencies)
  onMounted(() => {
    void controller.commands.initialize()
  })
  onScopeDispose(() => {
    controller.dispose()
  })
  return controller
}

export function formatBoardCatalogDate(timestamp: number): string {
  if (!Number.isFinite(timestamp)) return '-'
  const date = new Date(timestamp)
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}.${month}.${day}`
}
