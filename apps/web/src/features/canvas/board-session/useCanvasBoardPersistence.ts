import { computed } from 'vue'
import {
  createBoardPersistenceSession,
  type BoardPersistenceSessionOptions,
  type SaveCurrentBoardSnapshotOptions,
} from './boardPersistenceSession'

export type { SaveCurrentBoardSnapshotOptions }

export type UseCanvasBoardPersistenceOptions = Omit<BoardPersistenceSessionOptions, 'canSaveBoard'>

export function useCanvasBoardPersistence(options: UseCanvasBoardPersistenceOptions) {
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
  const session = createBoardPersistenceSession({
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
