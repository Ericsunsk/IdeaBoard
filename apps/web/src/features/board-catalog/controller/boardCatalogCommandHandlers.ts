import type { Ref } from 'vue'
import type { CanvasBoardCatalog } from '../../canvas-board/catalog/canvasBoardCatalog'
import type { BoardCatalogEntry, BoardsCatalogNavigation } from '../boardCatalogContract'
import type { BoardFeedback } from '../feedback/useBoardFeedback'

type BoardCatalogCommandState = {
  activeMenuBoardId: Ref<string | null>
  renamingBoardId: Ref<string | null>
  renameDraft: Ref<string>
  catalogRecoveryBoardId: Ref<string | null>
  isCatalogReloadPending: Ref<boolean>
  pendingDeleteBoard: Ref<BoardCatalogEntry | null>
  isDeletePending: Ref<boolean>
}

type BoardCatalogCommandDependencies = {
  catalog: CanvasBoardCatalog
  navigation: BoardsCatalogNavigation
  feedback: BoardFeedback
  warn: (...data: unknown[]) => void
  state: BoardCatalogCommandState
  clearRename(suppressNextOpen: boolean): void
  resetDeleteState(): void
}

async function runCatalogCommand<T>(
  operation: () => Promise<T>,
  onError: (error: unknown) => void,
): Promise<T | undefined> {
  try {
    return await operation()
  } catch (error) {
    onError(error)
    return undefined
  }
}

export function createBoardCatalogCommandHandlers({
  catalog,
  navigation,
  feedback,
  warn,
  state,
  clearRename,
  resetDeleteState,
}: BoardCatalogCommandDependencies) {
  async function createAndOpenBoard() {
    state.activeMenuBoardId.value = null
    const result = await runCatalogCommand(
      () => catalog.commands.create(),
      (error) => {
        warn('[boards] failed to create board', error)
        feedback.catalogSyncFailed()
      },
    )
    if (!result) return
    if (!result.ok) {
      if (result.reason === 'scope-changed') return
      feedback.createFailed(result)
      warn('[boards] failed to sync new board', result.error)
      return
    }
    navigation.openBoard(result.board.id)
  }

  async function duplicateBoard(boardId: string) {
    state.activeMenuBoardId.value = null
    const result = await runCatalogCommand(
      () => catalog.commands.duplicate({ sourceBoardId: boardId }),
      (error) => {
        warn('[boards] failed to duplicate board', error)
        feedback.catalogSyncFailed()
      },
    )
    if (!result) return
    if (!result.ok) {
      if (result.reason === 'scope-changed') return
      feedback.copyFailed(result)
      if (result.reason !== 'source-missing') {
        warn('[boards] failed to sync duplicated board', result.error)
      }
      return
    }
    feedback.copied()
  }

  async function commitRename() {
    const boardId = state.renamingBoardId.value
    if (!boardId) return
    const result = await catalog.commands.rename({
      boardId,
      nextName: state.renameDraft.value,
    })
    if (!result.ok) {
      switch (result.reason) {
        case 'scope-changed':
          return
        case 'local-name-conflict':
          feedback.renameValidationFailed(result)
          return
        case 'invalid-name':
          clearRename(true)
          feedback.renameValidationFailed(result)
          return
        case 'remote-name-conflict':
        case 'sync-failed':
          clearRename(true)
          feedback.renameSyncFailed(result)
          state.catalogRecoveryBoardId.value =
            result.reason === 'remote-name-conflict'
              ? result.hydrate?.ok === true
                ? null
                : boardId
              : result.rollback.ok
                ? null
                : boardId
          warn('[boards] failed to sync renamed board', result.error)
          return
      }
    }
    clearRename(true)
    feedback.renamed()
  }

  async function reloadCatalog() {
    const boardId = state.catalogRecoveryBoardId.value
    if (!boardId || state.isCatalogReloadPending.value) return
    state.isCatalogReloadPending.value = true
    const result = await runCatalogCommand(
      () =>
        catalog.commands.hydrate({
          preserveLocalNewer: false,
          rollbackBoardIds: [boardId],
        }),
      (error) => {
        warn('[boards] failed to reload catalog', error)
        feedback.catalogSyncFailed()
      },
    )
    try {
      if (!result) return
      if (!result.ok) {
        feedback.catalogSyncFailed()
        return
      }
      state.catalogRecoveryBoardId.value = null
      feedback.catalogReloaded()
    } finally {
      state.isCatalogReloadPending.value = false
    }
  }

  async function confirmDelete() {
    const board = state.pendingDeleteBoard.value
    if (!board || state.isDeletePending.value) return
    state.isDeletePending.value = true
    const result = await runCatalogCommand(
      () => catalog.commands.delete({ boardId: board.id }),
      (error) => {
        const failure = { ok: false as const, reason: 'sync-failed' as const, error }
        feedback.deleteFailed(failure)
        warn('[boards] failed to sync deleted board', error)
      },
    )
    try {
      if (!result) return
      if (!result.ok) {
        if (result.reason === 'scope-changed') {
          resetDeleteState()
          return
        }
        feedback.deleteFailed(result)
        if (result.reason === 'sync-failed') {
          warn('[boards] failed to sync deleted board', result.error)
        } else {
          resetDeleteState()
        }
        return
      }
      if (state.renamingBoardId.value === board.id) {
        clearRename(false)
      }
      resetDeleteState()
      feedback.deleted()
    } finally {
      state.isDeletePending.value = false
    }
  }

  return { createAndOpenBoard, duplicateBoard, commitRename, reloadCatalog, confirmDelete }
}
