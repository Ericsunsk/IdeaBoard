import {
  createBoard,
  deleteBoard,
  duplicateBoard,
  getBoardById,
  isBoardNameTaken,
  registerBoardMeta,
  renameBoard,
  type BoardMeta,
} from './boards'
import {
  clearPendingBoardSyncRollback,
  hydrateBoardsFromServer,
  isBoardNameConflictError,
  syncBoardToServer,
  syncDeleteBoardToServer,
  type HydrateBoardsFromServerOptions,
  type HydrateBoardsFromServerResult,
} from '../sync/boardSync'
import {
  createWorkspaceBoard,
  requireBoardWorkspaceId,
  type WorkspaceBoardMeta,
} from '../../infrastructure/api/boardsResource'
import { deleteBoardState, duplicateBoardState } from '../../infrastructure/persistence'

export type BoardScopeChangedResult = { ok: false; reason: 'scope-changed' }

export type BoardCreateResult =
  | { ok: true; board: BoardMeta }
  | { ok: false; reason: 'name-conflict' | 'sync-failed'; board: BoardMeta; error: unknown }
  | BoardScopeChangedResult

export type BoardRenameResult =
  | { ok: true; board: BoardMeta }
  | { ok: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | {
      ok: false
      reason: 'remote-name-conflict' | 'sync-failed'
      boardId: string
      error: unknown
    }
  | BoardScopeChangedResult

export type BoardRenameRemoteResult =
  | { ok: true; board: BoardMeta }
  | {
      ok: false
      reason: 'remote-name-conflict' | 'sync-failed'
      boardId: string
      board: BoardMeta
      previousBoard: BoardMeta
      error: unknown
    }

export type BoardOptimisticRenameResult =
  | {
      ok: true
      board: BoardMeta
      previousBoard: BoardMeta
      confirm: () => Promise<BoardRenameRemoteResult>
    }
  | { ok: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | BoardScopeChangedResult

export type BoardRollbackRenameResult =
  | { ok: true; board: BoardMeta }
  | { ok: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | BoardScopeChangedResult

export type BoardDuplicateResult =
  | { ok: true; board: BoardMeta }
  | { ok: false; reason: 'source-missing'; error?: undefined }
  | { ok: false; reason: 'name-conflict' | 'sync-failed'; board: BoardMeta; error: unknown }
  | BoardScopeChangedResult

export type BoardDeleteResult =
  | { ok: true; remoteWasMissing: boolean }
  | { ok: false; reason: 'sync-failed'; error: unknown }
  | { ok: false; reason: 'local-missing'; error?: undefined }
  | BoardScopeChangedResult

export interface BoardCommandDependencies {
  createBoard(name?: string): BoardMeta
  getBoard(boardId: string): BoardMeta | null
  renameBoard(boardId: string, nextName: string): BoardMeta | null
  duplicateBoard(boardId: string): BoardMeta | null
  deleteBoard(boardId: string): boolean
  isNameTaken(name: string, excludeBoardId?: string): boolean
  duplicateBoardState(sourceBoardId: string, targetBoardId: string): boolean
  deleteBoardState(boardId: string): boolean
  syncBoard(boardId: string): Promise<void>
  syncDeleteBoard(boardId: string): Promise<void>
  hydrateBoards(options?: HydrateBoardsFromServerOptions): Promise<HydrateBoardsFromServerResult>
  isNameConflictError(error: unknown): boolean
  isNotFoundError(error: unknown): boolean
  clearSyncRollback(boardId: string): void
  isMutationCurrent(): boolean
  // v1 server-mint seams (optional so existing call sites keep working):
  // the server owns board ids/revisions, so create/duplicate POST first,
  // register the minted identity locally, then sync the document.
  resolveWorkspace?(): string
  createServerBoard?(workspaceId: string, name: string): Promise<WorkspaceBoardMeta>
  registerServerBoard?(input: {
    id: string
    name: string
    revision?: number
  }): BoardMeta | null
}

export interface BoardCommands {
  create(input?: { name?: string }): Promise<BoardCreateResult>
  rename(input: { boardId: string; nextName: string }): Promise<BoardRenameResult>
  renameOptimistic(input: { boardId: string; nextName: string }): BoardOptimisticRenameResult
  rollbackOptimisticRename(input: {
    boardId: string
    previousName: string
  }): BoardRollbackRenameResult
  duplicate(input: { sourceBoardId: string }): Promise<BoardDuplicateResult>
  delete(input: { boardId: string }): Promise<BoardDeleteResult>
}

const defaultDependencies: BoardCommandDependencies = {
  createBoard,
  getBoard: getBoardById,
  renameBoard: renameBoard,
  duplicateBoard: duplicateBoard,
  deleteBoard: deleteBoard,
  isNameTaken: isBoardNameTaken,
  duplicateBoardState: duplicateBoardState,
  deleteBoardState: deleteBoardState,
  syncBoard: syncBoardToServer,
  syncDeleteBoard: syncDeleteBoardToServer,
  hydrateBoards: hydrateBoardsFromServer,
  isNameConflictError: isBoardNameConflictError,
  isNotFoundError: isBoardNotFoundError,
  clearSyncRollback: clearPendingBoardSyncRollback,
  isMutationCurrent: () => true,
  resolveWorkspace: () => requireBoardWorkspaceId(),
  createServerBoard: (workspaceId, name) => createWorkspaceBoard(workspaceId, name),
  registerServerBoard: (input) => registerBoardMeta(input),
}

export function createBoardCommands(
  dependencyOverrides: Partial<BoardCommandDependencies> = {},
): BoardCommands {
  const deps = { ...defaultDependencies, ...dependencyOverrides }

  function confirmOptimisticRename(
    boardId: string,
    board: BoardMeta,
    previousBoard: BoardMeta,
  ): Promise<BoardRenameRemoteResult> {
    return deps
      .syncBoard(boardId)
      .then(() => ({ ok: true as const, board }))
      .catch((error) => ({
        ok: false as const,
        reason: deps.isNameConflictError(error) ? 'remote-name-conflict' : 'sync-failed',
        boardId,
        board,
        previousBoard,
        error,
      }))
  }

  function renameOptimistic({
    boardId,
    nextName,
  }: {
    boardId: string
    nextName: string
  }): BoardOptimisticRenameResult {
    if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
    if (deps.isNameTaken(nextName, boardId)) {
      return { ok: false, reason: 'local-name-conflict' }
    }
    const previousBoard = deps.getBoard(boardId)
    if (!previousBoard) {
      return { ok: false, reason: 'invalid-name' }
    }
    const previous = cloneBoardMeta(previousBoard)
    const renamed = deps.renameBoard(boardId, nextName)
    if (!renamed) {
      return { ok: false, reason: 'invalid-name' }
    }
    const board = cloneBoardMeta(renamed)
    return {
      ok: true,
      board,
      previousBoard: previous,
      confirm: () => confirmOptimisticRename(boardId, board, previous),
    }
  }

  function rollbackOptimisticRename({
    boardId,
    previousName,
  }: {
    boardId: string
    previousName: string
  }): BoardRollbackRenameResult {
    if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
    if (deps.isNameTaken(previousName, boardId)) {
      return { ok: false, reason: 'local-name-conflict' }
    }
    const rolledBack = deps.renameBoard(boardId, previousName)
    if (!rolledBack) {
      return { ok: false, reason: 'invalid-name' }
    }
    deps.clearSyncRollback(boardId)
    return { ok: true, board: cloneBoardMeta(rolledBack) }
  }

  return {
    async create(input = {}) {
      const board = deps.createBoard(input.name || 'Untitled')
      const resolveWorkspace = deps.resolveWorkspace ?? (() => requireBoardWorkspaceId())
      const createServerBoard =
        deps.createServerBoard ?? ((workspaceId, name) => createWorkspaceBoard(workspaceId, name))
      const registerServerBoard = deps.registerServerBoard ?? ((entry) => registerBoardMeta(entry))
      let workspaceId: string
      try {
        workspaceId = resolveWorkspace()
      } catch (error) {
        deps.deleteBoard(board.id)
        deps.deleteBoardState(board.id)
        return { ok: false as const, reason: 'sync-failed' as const, board, error }
      }
      let server: WorkspaceBoardMeta
      try {
        server = await createServerBoard(workspaceId, board.name)
      } catch (error) {
        deps.deleteBoard(board.id)
        deps.deleteBoardState(board.id)
        if (deps.isNameConflictError(error)) {
          await deps.hydrateBoards()
          return { ok: false as const, reason: 'name-conflict' as const, board, error }
        }
        return { ok: false as const, reason: 'sync-failed' as const, board, error }
      }
      const registered = registerServerBoard({
        id: server.id,
        name: server.name || board.name,
        revision: server.revision,
      })
      deps.deleteBoard(board.id)
      deps.deleteBoardState(board.id)
      if (!registered) {
        return {
          ok: false as const,
          reason: 'sync-failed' as const,
          board: { ...board, id: server.id },
          error: new Error('Board registration failed'),
        }
      }
      try {
        await deps.syncBoard(registered.id)
        if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
        return { ok: true, board: registered }
      } catch (error) {
        if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
        deps.deleteBoard(registered.id)
        deps.deleteBoardState(registered.id)
        if (deps.isNameConflictError(error)) {
          await deps.hydrateBoards()
          return { ok: false as const, reason: 'name-conflict' as const, board: registered, error }
        }
        return { ok: false as const, reason: 'sync-failed' as const, board: registered, error }
      }
    },

    async rename({ boardId, nextName }) {
      const optimistic = renameOptimistic({ boardId, nextName })
      if (!optimistic.ok) return optimistic
      const remote = await optimistic.confirm()
      if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
      if (remote.ok) return { ok: true, board: remote.board }
      {
        const isConflict = remote.reason === 'remote-name-conflict'
        await deps.hydrateBoards(
          isConflict ? { preserveLocalNewer: false, rollbackBoardIds: [boardId] } : undefined,
        )
        return {
          ok: false,
          reason: isConflict ? 'remote-name-conflict' : 'sync-failed',
          boardId,
          error: remote.error,
        }
      }
    },

    renameOptimistic,

    rollbackOptimisticRename,

    async duplicate({ sourceBoardId }) {
      const copy = deps.duplicateBoard(sourceBoardId)
      if (!copy) {
        return { ok: false, reason: 'source-missing' }
      }
      const resolveWorkspace = deps.resolveWorkspace ?? (() => requireBoardWorkspaceId())
      const createServerBoard =
        deps.createServerBoard ?? ((workspaceId, name) => createWorkspaceBoard(workspaceId, name))
      const registerServerBoard = deps.registerServerBoard ?? ((entry) => registerBoardMeta(entry))
      let workspaceId: string
      try {
        workspaceId = resolveWorkspace()
      } catch (error) {
        deps.deleteBoard(copy.id)
        return { ok: false as const, reason: 'sync-failed' as const, board: copy, error }
      }
      let server: WorkspaceBoardMeta
      try {
        server = await createServerBoard(workspaceId, copy.name)
      } catch (error) {
        deps.deleteBoard(copy.id)
        if (deps.isNameConflictError(error)) {
          await deps.hydrateBoards()
          return { ok: false as const, reason: 'name-conflict' as const, board: copy, error }
        }
        return { ok: false as const, reason: 'sync-failed' as const, board: copy, error }
      }
      const registered = registerServerBoard({
        id: server.id,
        name: server.name || copy.name,
        revision: server.revision,
      })
      deps.deleteBoard(copy.id)
      if (!registered) {
        return {
          ok: false as const,
          reason: 'sync-failed' as const,
          board: { ...copy, id: server.id },
          error: new Error('Board registration failed'),
        }
      }
      deps.duplicateBoardState(sourceBoardId, registered.id)
      try {
        await deps.syncBoard(registered.id)
        if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
        return { ok: true, board: registered }
      } catch (error) {
        if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
        deps.deleteBoard(registered.id)
        deps.deleteBoardState(registered.id)
        if (deps.isNameConflictError(error)) {
          await deps.hydrateBoards()
          return {
            ok: false as const,
            reason: 'name-conflict' as const,
            board: registered,
            error,
          }
        }
        return { ok: false as const, reason: 'sync-failed' as const, board: registered, error }
      }
    },

    async delete({ boardId }) {
      let remoteWasMissing = false
      try {
        await deps.syncDeleteBoard(boardId)
      } catch (error) {
        if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }
        if (!deps.isNotFoundError(error)) {
          return { ok: false, reason: 'sync-failed', error }
        }
        remoteWasMissing = true
      }

      if (!deps.isMutationCurrent()) return { ok: false, reason: 'scope-changed' }

      const deleted = deps.deleteBoard(boardId)
      if (!deleted) {
        return { ok: false, reason: 'local-missing' }
      }
      deps.deleteBoardState(boardId)
      return { ok: true, remoteWasMissing }
    },
  }
}

function cloneBoardMeta(board: BoardMeta): BoardMeta {
  return {
    id: board.id,
    name: board.name,
    createdAt: board.createdAt,
    updatedAt: board.updatedAt,
    ...(board.revision === undefined ? {} : { revision: board.revision }),
    ...(board.archivedAt === undefined ? {} : { archivedAt: board.archivedAt }),
  }
}

function isBoardNotFoundError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 404
}
