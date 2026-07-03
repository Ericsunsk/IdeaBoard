import {
  hydrateCanvasBoardsFromServer,
  syncCanvasBoardToServer,
} from '../../canvas-board/catalog/boardSync'
import { getCanvasBoardById, touchCanvasBoard } from '../../canvas-board/catalog/canvasBoards'
import {
  getCanvasBoardState,
  setCanvasBoardPreviewSnapshot,
  setCanvasBoardState,
} from '../../canvas-board/persistence'
import type { CanvasBoardSnapshot, CanvasBoardState } from '../../canvas-board/domain'

export type BoardSessionRepository = {
  exists: (boardId: string) => boolean
  load: (boardId: string) => Promise<CanvasBoardSnapshot | null>
  save: (boardId: string, snapshot: CanvasBoardSnapshot) => Promise<CanvasBoardState>
  setPreview: (
    boardId: string,
    previewSnapshotDataUrl: string | undefined,
    previewSnapshotUpdatedAt: number | undefined,
    previewSnapshotVersion: number | undefined,
  ) => CanvasBoardState | null
  sync: (boardId: string, stateOverride?: CanvasBoardState | null) => Promise<void>
}

export class BoardSessionRemoteSyncError extends Error {
  readonly boardId: string
  readonly savedState: CanvasBoardState

  constructor(boardId: string, savedState: CanvasBoardState, cause: unknown) {
    super('BOARD_REMOTE_SYNC_FAILED', { cause })
    this.name = 'BoardSessionRemoteSyncError'
    this.boardId = boardId
    this.savedState = savedState
  }
}

export function isBoardSessionRemoteSyncError(
  error: unknown,
): error is BoardSessionRemoteSyncError {
  return error instanceof BoardSessionRemoteSyncError
}

export function createBoardSessionRepository(): BoardSessionRepository {
  async function load(boardId: string): Promise<CanvasBoardSnapshot | null> {
    const normalized = normalizeBoardId(boardId)
    if (!normalized) return null
    await hydrateCanvasBoardsFromServer()
    return getCanvasBoardState(normalized)
  }

  async function save(
    boardId: string,
    snapshot: CanvasBoardSnapshot,
  ): Promise<CanvasBoardState> {
    const normalized = normalizeBoardId(boardId)
    if (!normalized) {
      throw new Error('MISSING_BOARD_ID')
    }
    const board = getCanvasBoardById(normalized)
    if (!board) {
      throw new Error('BOARD_NOT_FOUND')
    }
    const saved = setCanvasBoardState(normalized, snapshot)
    if (!saved) {
      throw new Error('SET_BOARD_STATE_FAILED')
    }
    const touched = touchCanvasBoard(normalized)
    if (!touched) {
      throw new Error('TOUCH_BOARD_FAILED')
    }
    try {
      await syncCanvasBoardToServer(normalized, saved)
    } catch (error) {
      throw new BoardSessionRemoteSyncError(normalized, saved, error)
    }
    return saved
  }

  return {
    exists(boardId: string) {
      const normalized = normalizeBoardId(boardId)
      return !!normalized && !!getCanvasBoardById(normalized)
    },
    load,
    save,
    setPreview: setCanvasBoardPreviewSnapshot,
    sync: syncCanvasBoardToServer,
  }
}

function normalizeBoardId(boardId: string): string {
  return boardId.trim()
}
