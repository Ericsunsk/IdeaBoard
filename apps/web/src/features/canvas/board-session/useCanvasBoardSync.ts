import { computed } from 'vue'
import {
  createBoardSyncCoordinator,
  type BoardSyncCoordinatorOptions,
} from './boardSyncCoordinator'

export type {
  CanvasBoardLoadResult,
  CanvasBoardSaveResult,
  CanvasBoardSaveReconciliation,
  SaveStatus,
} from './boardSyncCoordinator'

export type UseCanvasBoardSyncOptions = Omit<BoardSyncCoordinatorOptions, 'canSaveBoard'>

export function useCanvasBoardSync(options: UseCanvasBoardSyncOptions) {
  const currentBoardId = options.boardSession.currentBoardId
  const currentBoardExists = computed(() => {
    const boardId = currentBoardId.value
    if (!boardId) return false
    return options.boardSession.exists(boardId)
  })
  const canSaveBoard = computed(
    () =>
      !!currentBoardId.value && currentBoardExists.value && !options.isLoadingBoardSnapshot.value,
  )
  const session = createBoardSyncCoordinator({
    ...options,
    canSaveBoard: () => canSaveBoard.value,
  })

  return {
    currentBoardId,
    canSaveBoard,
    loadInitialOrRouteBoard: session.loadInitialOrRouteBoard,
    saveCurrentBoardSnapshot: session.saveCurrentBoardSnapshot,
  }
}
