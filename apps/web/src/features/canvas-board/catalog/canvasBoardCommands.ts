import {
  createCanvasBoard,
  deleteCanvasBoard,
  duplicateCanvasBoard,
  isCanvasBoardNameTaken,
  renameCanvasBoard,
  type CanvasBoardMeta,
} from './canvasBoards'
import {
  hydrateCanvasBoardsFromServer,
  isBoardNameConflictError,
  syncCanvasBoardToServer,
  syncDeleteCanvasBoardToServer,
  type HydrateCanvasBoardsFromServerOptions,
  type HydrateCanvasBoardsFromServerResult,
} from './boardSync'
import { deleteCanvasBoardState, duplicateCanvasBoardState } from '../persistence'

export type CanvasBoardCreateResult =
  | { ok: true; board: CanvasBoardMeta }
  | { ok: false; reason: 'name-conflict' | 'sync-failed'; board: CanvasBoardMeta; error: unknown }

export type CanvasBoardRenameResult =
  | { ok: true; board: CanvasBoardMeta }
  | { ok: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | {
      ok: false
      reason: 'remote-name-conflict' | 'sync-failed'
      boardId: string
      error: unknown
    }

export type CanvasBoardDuplicateResult =
  | { ok: true; board: CanvasBoardMeta }
  | { ok: false; reason: 'source-missing'; error?: undefined }
  | { ok: false; reason: 'name-conflict' | 'sync-failed'; board: CanvasBoardMeta; error: unknown }

export type CanvasBoardDeleteResult =
  | { ok: true; remoteWasMissing: boolean }
  | { ok: false; reason: 'sync-failed'; error: unknown }
  | { ok: false; reason: 'local-missing'; error?: undefined }

export interface CanvasBoardCommandDependencies {
  createBoard(name?: string): CanvasBoardMeta
  renameBoard(boardId: string, nextName: string): CanvasBoardMeta | null
  duplicateBoard(boardId: string): CanvasBoardMeta | null
  deleteBoard(boardId: string): boolean
  isNameTaken(name: string, excludeBoardId?: string): boolean
  duplicateBoardState(sourceBoardId: string, targetBoardId: string): boolean
  deleteBoardState(boardId: string): boolean
  syncBoard(boardId: string): Promise<void>
  syncDeleteBoard(boardId: string): Promise<void>
  hydrateBoards(
    options?: HydrateCanvasBoardsFromServerOptions,
  ): Promise<HydrateCanvasBoardsFromServerResult>
  isNameConflictError(error: unknown): boolean
  isNotFoundError(error: unknown): boolean
}

export interface CanvasBoardCommands {
  create(input?: { name?: string }): Promise<CanvasBoardCreateResult>
  rename(input: { boardId: string; nextName: string }): Promise<CanvasBoardRenameResult>
  duplicate(input: { sourceBoardId: string }): Promise<CanvasBoardDuplicateResult>
  delete(input: { boardId: string }): Promise<CanvasBoardDeleteResult>
}

const defaultDependencies: CanvasBoardCommandDependencies = {
  createBoard: createCanvasBoard,
  renameBoard: renameCanvasBoard,
  duplicateBoard: duplicateCanvasBoard,
  deleteBoard: deleteCanvasBoard,
  isNameTaken: isCanvasBoardNameTaken,
  duplicateBoardState: duplicateCanvasBoardState,
  deleteBoardState: deleteCanvasBoardState,
  syncBoard: syncCanvasBoardToServer,
  syncDeleteBoard: syncDeleteCanvasBoardToServer,
  hydrateBoards: hydrateCanvasBoardsFromServer,
  isNameConflictError: isBoardNameConflictError,
  isNotFoundError: isBoardNotFoundError,
}

export function createCanvasBoardCommands(
  dependencyOverrides: Partial<CanvasBoardCommandDependencies> = {},
): CanvasBoardCommands {
  const deps = { ...defaultDependencies, ...dependencyOverrides }

  return {
    async create(input = {}) {
      const board = deps.createBoard(input.name || 'Untitled')
      try {
        await deps.syncBoard(board.id)
        return { ok: true, board }
      } catch (error) {
        deps.deleteBoard(board.id)
        deps.deleteBoardState(board.id)
        if (deps.isNameConflictError(error)) {
          await deps.hydrateBoards()
          return { ok: false, reason: 'name-conflict', board, error }
        }
        return { ok: false, reason: 'sync-failed', board, error }
      }
    },

    async rename({ boardId, nextName }) {
      if (deps.isNameTaken(nextName, boardId)) {
        return { ok: false, reason: 'local-name-conflict' }
      }
      const renamed = deps.renameBoard(boardId, nextName)
      if (!renamed) {
        return { ok: false, reason: 'invalid-name' }
      }
      try {
        await deps.syncBoard(boardId)
        return { ok: true, board: renamed }
      } catch (error) {
        const isConflict = deps.isNameConflictError(error)
        await deps.hydrateBoards(
          isConflict ? { preserveLocalNewer: false, rollbackBoardIds: [boardId] } : undefined,
        )
        return {
          ok: false,
          reason: isConflict ? 'remote-name-conflict' : 'sync-failed',
          boardId,
          error,
        }
      }
    },

    async duplicate({ sourceBoardId }) {
      const copy = deps.duplicateBoard(sourceBoardId)
      if (!copy) {
        return { ok: false, reason: 'source-missing' }
      }
      deps.duplicateBoardState(sourceBoardId, copy.id)
      try {
        await deps.syncBoard(copy.id)
        return { ok: true, board: copy }
      } catch (error) {
        deps.deleteBoard(copy.id)
        deps.deleteBoardState(copy.id)
        if (deps.isNameConflictError(error)) {
          await deps.hydrateBoards()
          return { ok: false, reason: 'name-conflict', board: copy, error }
        }
        return { ok: false, reason: 'sync-failed', board: copy, error }
      }
    },

    async delete({ boardId }) {
      let remoteWasMissing = false
      try {
        await deps.syncDeleteBoard(boardId)
      } catch (error) {
        if (!deps.isNotFoundError(error)) {
          return { ok: false, reason: 'sync-failed', error }
        }
        remoteWasMissing = true
      }

      const deleted = deps.deleteBoard(boardId)
      if (!deleted) {
        return { ok: false, reason: 'local-missing' }
      }
      deps.deleteBoardState(boardId)
      return { ok: true, remoteWasMissing }
    },
  }
}

export function isBoardNotFoundError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  if (code === 404) return true
  const msg = 'msg' in error ? (error as { msg?: unknown }).msg : undefined
  return (
    typeof msg === 'string' &&
    (msg.includes('画布不存在') || msg.toLowerCase().includes('not found'))
  )
}
