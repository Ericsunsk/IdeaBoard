import {
  syncBoardToServer,
  useBoardRepository,
  getBoardById,
  touchBoard,
  loadBoardState,
  serializeBoardState,
  setBoardPreviewSnapshot,
  setBoardState,
  InvalidBoardStateError,
  type BoardSnapshot,
  type BoardState,
} from '../../../board'

export type BoardSessionRepository = {
  exists: (boardId: string) => boolean
  load: (boardId: string) => Promise<BoardSnapshot | null>
  saveLocal: (boardId: string, snapshot: BoardSnapshot) => Promise<BoardState>
  save: (boardId: string, snapshot: BoardSnapshot) => Promise<BoardState>
  setPreview: (
    boardId: string,
    previewSnapshotDataUrl: string | undefined,
    previewSnapshotUpdatedAt: number | undefined,
    previewSnapshotVersion: number | undefined,
  ) => BoardState | null
  sync: (boardId: string, stateOverride?: BoardState | null) => Promise<void>
}

export class BoardSessionRemoteSyncError extends Error {
  readonly boardId: string
  readonly savedState: BoardState

  constructor(boardId: string, savedState: BoardState, cause: unknown) {
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
  const boardRepository = useBoardRepository()

  async function load(boardId: string): Promise<BoardSnapshot | null> {
    const normalized = normalizeBoardId(boardId)
    if (!normalized) return null
    // Read before hydration so malformed local state is rejected at its storage boundary;
    // hydration must not silently replace or mask a recoverable local validation error.
    loadBoardState(normalized)
    const hydrateResult = await boardRepository.commands.hydrate()
    if (!hydrateResult.ok && hydrateResult.error instanceof InvalidBoardStateError) {
      throw hydrateResult.error
    }
    const state = loadBoardState(normalized)
    return state ? serializeBoardState(state, 'session-snapshot') : null
  }

  async function saveLocal(boardId: string, snapshot: BoardSnapshot): Promise<BoardState> {
    const normalized = normalizeBoardId(boardId)
    if (!normalized) {
      throw new Error('MISSING_BOARD_ID')
    }
    const board = getBoardById(normalized)
    if (!board) {
      throw new Error('BOARD_NOT_FOUND')
    }
    const saved = setBoardState(normalized, snapshot)
    if (!saved) {
      throw new Error('SET_BOARD_STATE_FAILED')
    }
    const touched = touchBoard(normalized)
    if (!touched) {
      throw new Error('TOUCH_BOARD_FAILED')
    }
    return saved
  }

  async function save(boardId: string, snapshot: BoardSnapshot): Promise<BoardState> {
    const normalized = normalizeBoardId(boardId)
    const saved = await saveLocal(normalized, snapshot)
    try {
      await syncBoardToServer(normalized, saved)
    } catch (error) {
      throw new BoardSessionRemoteSyncError(normalized, saved, error)
    }
    return saved
  }

  return {
    exists(boardId: string) {
      const normalized = normalizeBoardId(boardId)
      return !!normalized && !!getBoardById(normalized)
    },
    load,
    saveLocal,
    save,
    setPreview: setBoardPreviewSnapshot,
    sync: syncBoardToServer,
  }
}

function normalizeBoardId(boardId: string): string {
  return boardId.trim()
}
