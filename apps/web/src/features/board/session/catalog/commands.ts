import { createEmptyBoardState } from '../../core/document/nodes'
import {
  createBoard,
  deleteBoard,
  duplicateBoard,
  getBoardById,
  isBoardNameTaken,
  normalizeBoardName,
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
  fetchWorkspaceBoardDocument,
  requireBoardWorkspaceId,
  saveWorkspaceBoard,
  type WorkspaceBoardDocument,
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
  | { isOk: false; reason: 'source-unreadable'; error?: unknown }
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
  initializeServerBoard(workspaceId: string, board: WorkspaceBoardMeta): Promise<WorkspaceBoardMeta>
  /** Reads the source document to prove copy readability before minting. */
  fetchSourceDocument?(workspaceId: string, boardId: string): Promise<WorkspaceBoardDocument>
  /** Archives a server-created board using the workspace captured before the request. */
  archiveServerBoard?(workspaceId: string, boardId: string): Promise<WorkspaceBoardMeta>
  registerServerBoard?(input: {
    id: string
    name: string
    revision?: number
    capabilities?: WorkspaceBoardMeta['capabilities']
    accessState?: WorkspaceBoardMeta['accessState']
    workspaceAccess?: WorkspaceBoardMeta['workspaceAccess']
    isCreator?: boolean
    hasDirectGrant?: boolean
    creatorUserId?: WorkspaceBoardMeta['creatorUserId']
    ownerAvatarUrl?: WorkspaceBoardMeta['ownerAvatarUrl']
  }): BoardMeta | null
}

export interface BoardCommands {
  create(input?: { name?: string }): Promise<BoardCreateResult>
  /** Initializes a readable document, opens it, then publishes it to the catalog. */
  createAndOpen(input: {
    name?: string
    openBoard(boardId: string): void | Promise<unknown>
  }): Promise<BoardCreateAndOpenResult>
  rename(input: { boardId: string; nextName: string }): Promise<BoardRenameResult>
  renameOptimistic(input: { boardId: string; nextName: string }): BoardOptimisticRenameResult
  rollbackOptimisticRename(input: {
    boardId: string
    previousName: string
  }): BoardRollbackRenameResult
  duplicate(input: { sourceBoardId: string }): Promise<BoardDuplicateResult>
  delete(input: { boardId: string }): Promise<BoardDeleteResult>
}

export type BoardCreateAndOpenResult =
  | BoardCreateResult
  | {
      isOk: false
      reason: 'navigation-failed'
      board: BoardMeta
      error: unknown
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
  initializeServerBoard: async (workspaceId, board) => {
    const saved = await saveWorkspaceBoard(workspaceId, board.id, {
      state: createEmptyBoardState(),
      baseRevision: board.revision,
    })
    return saved.board
  },
  fetchSourceDocument: (workspaceId, boardId) => fetchWorkspaceBoardDocument(workspaceId, boardId),
  archiveServerBoard: (workspaceId, boardId) => archiveWorkspaceBoard(workspaceId, boardId),
  registerServerBoard: (input) => registerBoardMeta(input),
}

/** Archives only a board minted by this creation attempt before it was published.
 * Initialization failure or a scope change must not leave an unseen server
 * orphan. Archiving is reversible; cleanup never replaces the original failure.
 */
async function archiveCreatedServerBoard(
  deps: BoardCommandDependencies,
  workspaceId: string,
  boardId: string,
): Promise<void> {
  const archive = deps.archiveServerBoard
  if (!archive) return
  try {
    await archive(workspaceId, boardId)
  } catch {
    // Preserve the creation/scope failure if best-effort cleanup is unavailable.
  }
}

async function ensureScopeOrArchive(
  deps: BoardCommandDependencies,
  workspaceId: string,
  serverId: string,
): Promise<BoardScopeChangedResult | null> {
  if (deps.isMutationCurrent()) return null
  await archiveCreatedServerBoard(deps, workspaceId, serverId)
  return { isOk: false, reason: 'scope-changed' }
}

function registerMintedBoard(
  deps: BoardCommandDependencies,
  workspaceId: string,
  server: WorkspaceBoardMeta,
  fallbackName?: string,
): BoardMeta | null {
  const registerServerBoard = deps.registerServerBoard ?? ((entry) => registerBoardMeta(entry))
  const registered = registerServerBoard({
    id: server.id,
    name: server.name || fallbackName || server.id,
    revision: server.revision,
    // A minted board carries the server's access fact (workspace_access
    // none, creator-only): capabilities register verbatim so the copy
    // never inherits the source ACL.
    capabilities: server.capabilities,
    accessState: server.accessState,
    workspaceAccess: server.workspaceAccess,
    isCreator: server.isCreator,
    hasDirectGrant: server.hasDirectGrant,
    creatorUserId: server.creatorUserId,
    ownerAvatarUrl: server.ownerAvatarUrl,
  })
  if (!registered) return null
  writeBoardListMeta(workspaceId, server)
  return registered
}

async function handleSyncConflict(
  deps: BoardCommandDependencies,
  error: unknown,
): Promise<BoardSyncConflictKind | null> {
  const conflictKind = deps.getSyncConflictKind(error)
  if (conflictKind) {
    await deps.hydrateBoards()
  }
  return conflictKind
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

const MAX_BOARD_NAME_LEN = 60

function resolveUniqueCreateName(
  inputName: string | undefined,
  isTaken: (name: string) => boolean,
): string {
  const normalized = normalizeBoardName((inputName ?? '').trim() || 'Untitled') || 'Untitled'
  if (!isTaken(normalized)) return normalized
  for (let index = 2; index < 10000; index += 1) {
    const suffix = ` ${index}`
    const maxBase = Math.max(1, MAX_BOARD_NAME_LEN - suffix.length)
    const candidate = normalizeBoardName(`${normalized.slice(0, maxBase).trimEnd()}${suffix}`)
    if (candidate && !isTaken(candidate)) return candidate
  }
  const timestampSuffix = `${Date.now()}`.slice(-6)
  const maxBase = Math.max(1, MAX_BOARD_NAME_LEN - timestampSuffix.length - 1)
  const base = normalized.slice(0, maxBase).trimEnd()
  return normalizeBoardName(`${base} ${timestampSuffix}`)
}

function failedAutoOpenBoard(uniqueName: string): BoardMeta {
  const now = Date.now()
  return { id: '', name: uniqueName, createdAt: now, updatedAt: now }
}

function toAutoOpenBoard(server: WorkspaceBoardMeta, uniqueName: string): BoardMeta {
  return {
    id: server.id,
    name: server.name || uniqueName,
    createdAt: server.createdAt,
    updatedAt: server.updatedAt,
    ...(server.revision === undefined ? {} : { revision: server.revision }),
    ...(server.archivedAt === undefined || !server.archivedAt
      ? {}
      : { archivedAt: server.archivedAt }),
    ...(server.capabilities === undefined ? {} : { capabilities: server.capabilities }),
    ...(server.accessState === undefined ? {} : { accessState: server.accessState }),
    ...(server.workspaceAccess === undefined ? {} : { workspaceAccess: server.workspaceAccess }),
    ...(server.isCreator === undefined ? {} : { isCreator: server.isCreator }),
    ...(server.hasDirectGrant === undefined ? {} : { hasDirectGrant: server.hasDirectGrant }),
    ...(server.creatorUserId === undefined ? {} : { creatorUserId: server.creatorUserId }),
    ...(server.ownerAvatarUrl === undefined ? {} : { ownerAvatarUrl: server.ownerAvatarUrl }),
  }
}

async function mintAutoOpenServer(
  deps: BoardCommandDependencies,
  workspaceId: string,
  uniqueName: string,
): Promise<
  | { isOk: true; server: WorkspaceBoardMeta }
  | {
      isOk: false
      reason: BoardSyncConflictKind | 'sync-failed'
      board: BoardMeta
      error: unknown
    }
  | BoardScopeChangedResult
> {
  const createServerBoard =
    deps.createServerBoard ?? ((workspaceId, name) => createWorkspaceBoard(workspaceId, name))
  let server: WorkspaceBoardMeta
  try {
    server = await createServerBoard(workspaceId, uniqueName)
  } catch (error) {
    if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' as const }
    const board = failedAutoOpenBoard(uniqueName)
    const conflictKind = await handleSyncConflict(deps, error)
    if (conflictKind) {
      return { isOk: false as const, reason: conflictKind, board, error }
    }
    return { isOk: false as const, reason: 'sync-failed' as const, board, error }
  }
  const scopeChanged = await ensureScopeOrArchive(deps, workspaceId, server.id)
  if (scopeChanged) return scopeChanged
  return { isOk: true as const, server }
}

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
    const conflictKind = await handleSyncConflict(deps, error)
    if (conflictKind) {
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
      const conflictKind = await handleSyncConflict(deps, error)
      if (conflictKind) {
        return { isOk: false, reason: conflictKind, board: localBoard, error }
      }
      return { isOk: false, reason: 'sync-failed', board: localBoard, error }
    }

    // The server-minted identity belongs to the captured workspace. If the
    // selection changed while the POST was in flight, archive it through
    // the captured workspace and never register it into the new scope.
    const scopeChanged = await ensureScopeOrArchive(deps, workspaceId, server.id)
    if (scopeChanged) return scopeChanged

    const registered = registerMintedBoard(deps, workspaceId, server, localBoard.name)
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

    async createAndOpen(input) {
      const uniqueName = resolveUniqueCreateName(input.name, deps.isNameTaken)
      let workspaceId: string
      try {
        workspaceId = (deps.resolveWorkspace ?? requireBoardWorkspaceId)()
      } catch (error) {
        return { isOk: false, reason: 'sync-failed', board: failedAutoOpenBoard(uniqueName), error }
      }
      const minted = await mintAutoOpenServer(deps, workspaceId, uniqueName)
      if (!minted.isOk) return minted
      let server: WorkspaceBoardMeta
      try {
        // Initialize directly through the adapter: neither local metadata nor
        // Query catalog entries may appear while the gallery is still open.
        server = await deps.initializeServerBoard(workspaceId, minted.server)
      } catch (error) {
        await archiveCreatedServerBoard(deps, workspaceId, minted.server.id)
        if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
        return {
          isOk: false,
          reason: 'sync-failed',
          board: toAutoOpenBoard(minted.server, uniqueName),
          error,
        }
      }
      const beforeOpen = await ensureScopeOrArchive(deps, workspaceId, server.id)
      if (beforeOpen) return beforeOpen
      let navigationFailure: { error: unknown } | undefined
      try {
        await input.openBoard(server.id)
      } catch (error) {
        navigationFailure = { error }
      }
      const afterOpen = await ensureScopeOrArchive(deps, workspaceId, server.id)
      if (afterOpen) return afterOpen
      // A failed navigation still leaves a readable board in the gallery so
      // the user can open it again. Publication never conceals that failure.
      const board = registerMintedBoard(deps, workspaceId, server, uniqueName)
      if (!board)
        return {
          isOk: false,
          reason: 'sync-failed',
          board: toAutoOpenBoard(server, uniqueName),
          error: new Error('Board registration failed'),
        }
      if (navigationFailure)
        return { isOk: false, reason: 'navigation-failed', board, error: navigationFailure.error }
      return { isOk: true, board }
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
      // #943 copy precondition: prove the source board (and, through it,
      // the document the copy will carry) is still readable by the copier
      // BEFORE minting anything. A revoked or deleted source fails here
      // with no board created; per-asset readability is enforced again at
      // accept time by the server media gate, which fails the document
      // sync below when an asset is no longer usable.
      const cleanupCopy = () => {
        deps.deleteBoard(copy.id)
      }
      try {
        const resolveWorkspace = deps.resolveWorkspace ?? (() => requireBoardWorkspaceId())
        const fetchSourceDocument =
          deps.fetchSourceDocument ??
          ((workspaceId: string, boardId: string) =>
            fetchWorkspaceBoardDocument(workspaceId, boardId))
        await fetchSourceDocument(resolveWorkspace(), sourceBoardId)
      } catch (error) {
        if (!deps.isMutationCurrent()) return { isOk: false, reason: 'scope-changed' }
        cleanupCopy()
        return { isOk: false, reason: 'source-unreadable', error }
      }
      let mintedServerBoardId: string | null = null
      const result = await createServerBoardAndSync(copy, cleanupCopy, (registeredBoardId) => {
        mintedServerBoardId = registeredBoardId
        deps.duplicateBoardState(sourceBoardId, registeredBoardId)
      })
      // #943 copy atomicity: a minted board whose document sync failed must
      // not linger as a partial copy. The failure already dropped every
      // local follow-up; archive the server board best-effort through the
      // current workspace (scope switches return scope-changed above and
      // archive through the captured workspace there instead).
      if (!result.isOk && result.reason !== 'scope-changed' && mintedServerBoardId) {
        try {
          const resolveWorkspace = deps.resolveWorkspace ?? (() => requireBoardWorkspaceId())
          await archiveCreatedServerBoard(deps, resolveWorkspace(), mintedServerBoardId)
        } catch {
          // Best effort: the orphan stays creator-only (workspace_access
          // none) so it never exposes tenant content; a later
          // reconciliation can restore or re-archive it.
        }
      }
      return result
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
    // Server access facts ride optimistic renames untouched: the rename
    // path must never drop the range derivation or capability gating.
    ...(board.capabilities === undefined ? {} : { capabilities: board.capabilities }),
    ...(board.accessState === undefined ? {} : { accessState: board.accessState }),
    ...(board.workspaceAccess === undefined ? {} : { workspaceAccess: board.workspaceAccess }),
    ...(board.isCreator === undefined ? {} : { isCreator: board.isCreator }),
    ...(board.hasDirectGrant === undefined ? {} : { hasDirectGrant: board.hasDirectGrant }),
  }
}

function isBoardNotFoundError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 404
}
