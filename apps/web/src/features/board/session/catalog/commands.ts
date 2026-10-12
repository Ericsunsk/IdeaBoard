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
  getBoardSyncConflictKind,
  hydrateBoardsFromServer,
  syncBoardRenameToServer,
  syncBoardToServer,
  syncDeleteBoardToServer,
  type BoardSyncConflictKind,
  type HydrateBoardsFromServerOptions,
  type HydrateBoardsFromServerResult,
} from '../sync/boardSync'
import {
  archiveWorkspaceBoard,
  createWorkspaceBoard,
  requireBoardWorkspaceId,
  type WorkspaceBoardMeta,
} from '../adapters/boardsResource'
import { deleteBoardState, duplicateBoardState } from '../persistence/state'
import { writeBoardListMeta } from '../sync/boardQueries'

export type BoardScopeChangedResult = { isOk: false; reason: 'scope-changed' }

export type BoardCreateResult =
  | { isOk: true; board: BoardMeta }
  | {
      isOk: false
      reason: BoardSyncConflictKind | 'sync-failed'
      board: BoardMeta
      error: unknown
    }
  | BoardScopeChangedResult

export type BoardRenameResult =
  | { isOk: true; board: BoardMeta }
  | { isOk: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | {
      isOk: false
      reason: BoardSyncConflictKind | 'sync-failed'
      boardId: string
      error: unknown
    }
  | BoardScopeChangedResult

export type BoardRenameRemoteResult =
  | { isOk: true; board: BoardMeta }
  | {
      isOk: false
      reason: BoardSyncConflictKind | 'sync-failed'
      boardId: string
      board: BoardMeta
      previousBoard: BoardMeta
      error: unknown
    }

export type BoardOptimisticRenameResult =
  | {
      isOk: true
      board: BoardMeta
      previousBoard: BoardMeta
      confirm: () => Promise<BoardRenameRemoteResult>
    }
  | { isOk: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | BoardScopeChangedResult

export type BoardRollbackRenameResult =
  | { isOk: true; board: BoardMeta }
  | { isOk: false; reason: 'local-name-conflict' | 'invalid-name'; error?: undefined }
  | BoardScopeChangedResult

export type BoardDuplicateResult =
  | { isOk: true; board: BoardMeta }
  | { isOk: false; reason: 'source-missing'; error?: undefined }
  | {
      isOk: false
      reason: BoardSyncConflictKind | 'sync-failed'
      board: BoardMeta
      error: unknown
    }
  | BoardScopeChangedResult

export type BoardDeleteResult =
  | { isOk: true; isRemoteWasMissing: boolean }
  | { isOk: false; reason: 'sync-failed'; error: unknown }
  | { isOk: false; reason: 'local-missing'; error?: undefined }
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
  syncRenameBoard(boardId: string, nextName: string): Promise<void>
  syncDeleteBoard(boardId: string): Promise<void>
  hydrateBoards(options?: HydrateBoardsFromServerOptions): Promise<HydrateBoardsFromServerResult>
  getSyncConflictKind(error: unknown): BoardSyncConflictKind | null
  isNotFoundError(error: unknown): boolean
  clearSyncRollback(boardId: string): void
  isMutationCurrent(): boolean
  // v1 server-mint seams (optional so existing call sites keep working):
  // the server owns board ids/revisions, so create/duplicate POST first,
  // register the minted identity locally, then sync the document.
  resolveWorkspace?(): string
  createServerBoard?(workspaceId: string, name: string): Promise<WorkspaceBoardMeta>
  /** Archives a server-created board using the workspace captured before the request. */
  archiveServerBoard?(workspaceId: string, boardId: string): Promise<WorkspaceBoardMeta>
  registerServerBoard?(input: { id: string; name: string; revision?: number }): BoardMeta | null
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
  syncRenameBoard: syncBoardRenameToServer,
  syncDeleteBoard: syncDeleteBoardToServer,
  hydrateBoards: hydrateBoardsFromServer,
  getSyncConflictKind: getBoardSyncConflictKind,
  isNotFoundError: isBoardNotFoundError,
  clearSyncRollback: clearPendingBoardSyncRollback,
  isMutationCurrent: () => true,
  resolveWorkspace: () => requireBoardWorkspaceId(),
  createServerBoard: (workspaceId, name) => createWorkspaceBoard(workspaceId, name),
  archiveServerBoard: (workspaceId, boardId) => archiveWorkspaceBoard(workspaceId, boardId),
  registerServerBoard: (input) => registerBoardMeta(input),
}

/**
 * A create/duplicate response belongs to the workspace captured before the
 * POST. If the user changes scope before the response is applied, archive the
 * minted board through that captured workspace so it cannot become an unseen
 * server orphan. Archiving is the v1 reversible delete semantic. Cleanup is
 * best effort: the mutation must still terminate as scope-changed even when
 * the cleanup request is unavailable.
 */
async function archiveServerBoardAfterScopeChange(
  deps: BoardCommandDependencies,
  workspaceId: string,
  boardId: string,
): Promise<void> {
  const archive = deps.archiveServerBoard
  if (!archive) return
  try {
    await archive(workspaceId, boardId)
  } catch {
    // The caller cannot safely retry in the new scope. Keep the original
    // scope-changed result; a later reconciliation can restore the board if
    // the best-effort archive failed.
  }
}

type SyncRegisteredBoardResult =
  | { isOk: true; board: BoardMeta }
  | {
      isOk: false
      reason: BoardSyncConflictKind | 'sync-failed'
      board: BoardMeta
      error: unknown
    }
  | BoardScopeChangedResult

// Shared document-sync confirmation for server-minted boards (create and
// duplicate share this phase verbatim): a scope switch at any await drops
// every local follow-up instead of mutating the new scope.
async function syncRegisteredBoard(
  deps: BoardCommandDependencies,
  board: BoardMeta,
): Promise<SyncRegisteredBoardResult> {
  try {
    await deps.syncBoard(board.id)
    if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
    return { isOk: true, board }
  } catch (error) {
    if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
    deps.deleteBoard(board.id)
    deps.deleteBoardState(board.id)
    const conflictKind = deps.getSyncConflictKind(error)
    if (conflictKind) {
      await deps.hydrateBoards()
      return { isOk: false as const, reason: conflictKind, board, error }
    }
    return { isOk: false as const, reason: 'sync-failed' as const, board, error }
  }
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
    // Explicit rename only: PATCH the name without writing the document, so
    // ordinary document content can never piggyback a rename (or vice versa).
    return deps
      .syncRenameBoard(boardId, board.name)
      .then(() => ({ isOk: true as const, board }))
      .catch((error) => ({
        isOk: false as const,
        reason: deps.getSyncConflictKind(error) ?? ('sync-failed' as const),
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
    if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
    if (deps.isNameTaken(nextName, boardId)) {
      return { isOk: false, reason: 'local-name-conflict' }
    }
    const previousBoard = deps.getBoard(boardId)
    if (!previousBoard) {
      return { isOk: false, reason: 'invalid-name' }
    }
    const previous = cloneBoardMeta(previousBoard)
    const renamed = deps.renameBoard(boardId, nextName)
    if (!renamed) {
      return { isOk: false, reason: 'invalid-name' }
    }
    const board = cloneBoardMeta(renamed)
    return {
      isOk: true,
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
    if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
    if (deps.isNameTaken(previousName, boardId)) {
      return { isOk: false, reason: 'local-name-conflict' }
    }
    const rolledBack = deps.renameBoard(boardId, previousName)
    if (!rolledBack) {
      return { isOk: false, reason: 'invalid-name' }
    }
    deps.clearSyncRollback(boardId)
    return { isOk: true, board: cloneBoardMeta(rolledBack) }
  }

  async function createServerBoardAndSync(
    localBoard: BoardMeta,
    cleanupLocal: () => void,
    afterRegister?: (registeredBoardId: string) => void,
  ): Promise<SyncRegisteredBoardResult> {
    const resolveWorkspace = deps.resolveWorkspace ?? (() => requireBoardWorkspaceId())
    const createServerBoard =
      deps.createServerBoard ?? ((workspaceId, name) => createWorkspaceBoard(workspaceId, name))
    const registerServerBoard = deps.registerServerBoard ?? ((entry) => registerBoardMeta(entry))

    let workspaceId: string
    try {
      workspaceId = resolveWorkspace()
    } catch (error) {
      cleanupLocal()
      return { isOk: false, reason: 'sync-failed', board: localBoard, error }
    }

    let server: WorkspaceBoardMeta
    try {
      server = await createServerBoard(workspaceId, localBoard.name)
    } catch (error) {
      // A scope switch mid-create drops every local follow-up: the temp
      // board stays in its own scope and the new scope is never touched.
      if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
      cleanupLocal()
      const conflictKind = deps.getSyncConflictKind(error)
      if (conflictKind) {
        await deps.hydrateBoards()
        return { isOk: false, reason: conflictKind, board: localBoard, error }
      }
      return { isOk: false, reason: 'sync-failed', board: localBoard, error }
    }

    // The server-minted identity belongs to the captured workspace. If the
    // selection changed while the POST was in flight, archive it through
    // the captured workspace and never register it into the new scope.
    if (!deps.isMutationCurrent()) {
      await archiveServerBoardAfterScopeChange(deps, workspaceId, server.id)
      return { isOk: false, reason: 'scope-changed' }
    }

    const registered = registerServerBoard({
      id: server.id,
      name: server.name || localBoard.name,
      revision: server.revision,
    })
    cleanupLocal()
    if (!registered) {
      return {
        isOk: false,
        reason: 'sync-failed',
        board: { ...localBoard, id: server.id },
        error: new Error('Board registration failed'),
      }
    }
    // Create write-back: the server-minted meta enters the Query catalog
    // list immediately; the document snapshot follows on the sync below.
    writeBoardListMeta(workspaceId, server)
    afterRegister?.(registered.id)
    return syncRegisteredBoard(deps, registered)
  }

  return {
    async create(input = {}) {
      const board = deps.createBoard(input.name || 'Untitled')
      return createServerBoardAndSync(board, () => {
        deps.deleteBoard(board.id)
        deps.deleteBoardState(board.id)
      })
    },

    async rename({ boardId, nextName }) {
      const optimistic = renameOptimistic({ boardId, nextName })
      if (!optimistic.isOk) return optimistic
      const remote = await optimistic.confirm()
      if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
      if (remote.isOk) return { isOk: true, board: remote.board }
      {
        const isConflict =
          remote.reason === 'revision-conflict' || remote.reason === 'idempotency-conflict'
        await deps.hydrateBoards(
          isConflict ? { shouldPreserveLocalNewer: false, rollbackBoardIds: [boardId] } : undefined,
        )
        return {
          isOk: false,
          reason: remote.reason,
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
        return { isOk: false, reason: 'source-missing' }
      }
      return createServerBoardAndSync(
        copy,
        () => {
          deps.deleteBoard(copy.id)
        },
        (registeredBoardId) => {
          deps.duplicateBoardState(sourceBoardId, registeredBoardId)
        },
      )
    },

    async delete({ boardId }) {
      let isRemoteWasMissing = false
      try {
        await deps.syncDeleteBoard(boardId)
      } catch (error) {
        if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
        if (!deps.isNotFoundError(error)) {
          return { isOk: false, reason: 'sync-failed', error }
        }
        isRemoteWasMissing = true
      }

      if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }

      const deleted = deps.deleteBoard(boardId)
      if (!deleted) {
        return { isOk: false, reason: 'local-missing' }
      }
      deps.deleteBoardState(boardId)
      return { isOk: true, isRemoteWasMissing }
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
    ...(board.workspaceId === undefined ? {} : { workspaceId: board.workspaceId }),
  }
}

function isBoardNotFoundError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 404
}
