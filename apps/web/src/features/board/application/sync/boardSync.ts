import {
  archiveWorkspaceBoard,
  BoardWorkspaceUnavailableError,
  fetchWorkspaceBoardDocument,
  fetchWorkspaceBoardMeta,
  fetchWorkspaceBoards,
  renameWorkspaceBoard,
  requireBoardWorkspaceId,
  restoreWorkspaceBoard,
  saveWorkspaceBoard,
  type BoardsRequestOptions,
  type SaveWorkspaceBoardResult,
  type WorkspaceBoardDocument,
  type WorkspaceBoardMeta,
} from '../../infrastructure/api/boardsResource'
import { userErrorCode } from '../../../../shared/errors/errorCatalog'
import { stripUndefined } from '../../../../shared/api/stripUndefined'
import {
  boardScopeKey,
  getBoardScope,
  isBoardScopeCurrent,
  subscribeBoardScopeChange,
  type BoardScope,
  type CompleteBoardScope,
} from '../catalog/scope'
import {
  getBoardById,
  listBoards,
  normalizeBoardName,
  publishCatalogChange,
  registerBoardMeta,
  replaceBoards,
  setBoardArchivedAt,
  setBoardRevision,
  type BoardMeta,
} from '../catalog/boards'
import {
  getBoardState,
  normalizeBoardState,
  replaceBoardStateMap,
  serializeBoardState,
} from '../../infrastructure/persistence'
import {
  createEmptyBoardState,
  InvalidBoardStateError,
  type BoardStateValidationErrorContext,
  type BoardState,
} from '../../domain'
import { queryClient } from '../../../../shared/query/queryClient'
import { workspaceQueryKeys } from '../../../workspace'

export type HydrateBoardsFromServerResult = { ok: true } | { ok: false; error: unknown }

const hydrateInFlight = new Map<string, Promise<HydrateBoardsFromServerResult>>()
const pendingRollbackBoardIds = new Set<string>()
// Auth sign-out and workspace switches both invalidate pending per-board
// rollback markers: a marker captured under another User + Workspace scope
// must never steer hydration in the new scope.
subscribeBoardScopeChange(() => pendingRollbackBoardIds.clear())

export type HydrateBoardsFromServerOptions = {
  preserveLocalNewer?: boolean
  rollbackBoardIds?: string[]
  waitForWorkspaceTimeoutMs?: number
}

type ResolvedHydrateBoardsFromServerOptions = {
  preserveLocalNewer: boolean
  rollbackBoardIds: Set<string>
}

function toLocalMeta(board: WorkspaceBoardMeta): BoardMeta | null {
  const boardId = board.id?.trim()
  const name = normalizeBoardName(board.name || '')
  if (!boardId || !name) return null
  const revision = Number(board.revision)
  // Fail closed on broken timestamps: skip the entry so a corrupt remote row
  // can never masquerade as "just now" via a fabricated Date.now().
  const createdAt = Number(board.createdAt)
  const updatedAt = Number(board.updatedAt)
  if (!Number.isFinite(createdAt) || !Number.isFinite(updatedAt)) return null
  return {
    id: boardId,
    name,
    createdAt,
    updatedAt,
    ...(Number.isFinite(revision) && revision >= 0 ? { revision } : {}),
    ...(typeof board.archivedAt === 'string' && board.archivedAt
      ? { archivedAt: board.archivedAt }
      : {}),
  }
}

function toLocalState(boardId: string, document: WorkspaceBoardDocument): BoardState | null {
  // v1 splits the envelope: `schema_version` rides beside `payload`, while
  // the domain validator expects one `{ schemaVersion, ...nodes }` snapshot.
  // The write path (`saveWorkspaceBoard`) nests nodes under `payload`, so
  // the read path must unwrap it — otherwise every saved board fails
  // validation and hydration rejects the whole catalog. Recomposition stays
  // here (validation boundary), never in the transport.
  const snapshot =
    document.document && typeof document.document === 'object' && !Array.isArray(document.document)
      ? (document.document as Record<string, unknown>)
      : null
  const nested =
    snapshot &&
    typeof snapshot.payload === 'object' &&
    snapshot.payload !== null &&
    !Array.isArray(snapshot.payload)
      ? (snapshot.payload as Record<string, unknown>)
      : null
  const payload = { ...(nested ?? snapshot ?? {}), schemaVersion: document.schemaVersion }
  const result = normalizeBoardState(payload, {
    mode: 'result',
    boundary: 'server-hydration',
  })
  if (result.ok) return result.value
  console.warn('[boards] invalid remote board state rejected', boardId, result.error)
  throw new RemoteBoardStateError(boardId, result.error)
}

class RemoteBoardStateError extends InvalidBoardStateError {
  readonly boardId: string

  constructor(boardId: string, context: BoardStateValidationErrorContext) {
    super(context)
    this.name = 'RemoteBoardStateError'
    this.boardId = boardId
  }
}

function toGeneratedBoardStateProjection(state: BoardState): BoardState {
  const envelope = serializeBoardState(state, 'remote-envelope')
  return stripUndefined({
    schemaVersion: envelope.schemaVersion,
    imageNodes: envelope.imageNodes.map((node) => ({ ...node })),
    promptNodes: envelope.promptNodes.map((node) => ({ ...node })),
    fusionNodes: envelope.fusionNodes.map((node) => ({ ...node })),
    connections: envelope.connections.map((connection) => ({ ...connection })),
    groups: envelope.groups.map((group) => ({
      ...group,
      nodeIds: [...group.nodeIds],
    })),
    mediaRefs: envelope.mediaRefs?.map((ref) => stripUndefined({ ...ref })),
    previewSnapshotDataUrl: envelope.previewSnapshotDataUrl,
    previewSnapshotUpdatedAt: envelope.previewSnapshotUpdatedAt,
    previewSnapshotVersion: envelope.previewSnapshotVersion,
    updatedAt: envelope.updatedAt,
  })
}

function isLocalNewer(localUpdatedAt: number | undefined, remoteUpdatedAt: number | undefined) {
  const local = Number(localUpdatedAt)
  const remote = Number(remoteUpdatedAt)
  if (!Number.isFinite(local)) return false
  if (!Number.isFinite(remote)) return true
  return local > remote
}

function applyRemoteBoardsToLocal(
  boards: Array<{ meta: WorkspaceBoardMeta; state: BoardState | null }>,
  options: ResolvedHydrateBoardsFromServerOptions,
) {
  const metas: BoardMeta[] = []
  const stateMap: Record<string, BoardState> = {}
  boards.forEach((board) => {
    const meta = toLocalMeta(board.meta)
    if (!meta) return
    metas.push(meta)
    if (board.state) stateMap[meta.id] = board.state
  })
  if (!options.preserveLocalNewer) {
    replaceBoards(metas, { publish: false })
    replaceBoardStateMap(stateMap)
    publishCatalogChange()
    return
  }

  const mergedMetas = new Map<string, BoardMeta>()
  const localMetas = listBoards()
  const localMetaMap = new Map(localMetas.map((meta) => [meta.id, meta]))
  const shouldPreserveLocal = (boardId: string) => !options.rollbackBoardIds.has(boardId)

  metas.forEach((remoteMeta) => {
    const localMeta = localMetaMap.get(remoteMeta.id)
    const preserveLocal =
      localMeta &&
      shouldPreserveLocal(remoteMeta.id) &&
      isLocalNewer(localMeta.updatedAt, remoteMeta.updatedAt)
    // Keep the local draft with its true concurrency base: a newer
    // timestamp proves recency, never that the local content was rebased
    // onto the server revision. Carrying the remote revision here would
    // make the next save send a false `If-Match` and silently overwrite
    // the collaborator's snapshot. The base only advances when the remote
    // snapshot is accepted (replace path) or after an explicit rebase.
    mergedMetas.set(remoteMeta.id, preserveLocal ? { ...localMeta } : remoteMeta)
  })

  // Local-only boards (absent from the full remote list, which is a
  // complete cursor walk so absence means deleted/archived elsewhere):
  // absent + revision means the board synced before and is now deleted
  // elsewhere — deleted-elsewhere wins, so drop the local meta without
  // pushing it back to the server, discarding any unsynced local edits
  // with it (delete semantics, not a conflict). Absent + no revision
  // means a never-synced offline creation — keep it locally.
  // Rollback markers still drop first via shouldPreserveLocal above.
  localMetas.forEach((localMeta) => {
    if (mergedMetas.has(localMeta.id) || !shouldPreserveLocal(localMeta.id)) return
    if (localMeta.revision !== undefined) return
    mergedMetas.set(localMeta.id, localMeta)
  })

  const mergedStateMap: Record<string, BoardState> = {}
  mergedMetas.forEach((_, boardId) => {
    const localState = getBoardState(boardId)
    const remoteState = stateMap[boardId]
    if (
      localState &&
      remoteState &&
      shouldPreserveLocal(boardId) &&
      isLocalNewer(localState.updatedAt, remoteState.updatedAt)
    ) {
      mergedStateMap[boardId] = localState
      return
    }
    if (remoteState) {
      mergedStateMap[boardId] = remoteState
      return
    }
    if (localState && shouldPreserveLocal(boardId)) {
      mergedStateMap[boardId] = localState
    }
  })

  replaceBoards([...mergedMetas.values()], { publish: false })
  replaceBoardStateMap(mergedStateMap)
  publishCatalogChange()
}

function normalizeRollbackBoardIds(boardIds: Iterable<string | null | undefined>): Set<string> {
  const result = new Set<string>()
  Array.from(boardIds).forEach((boardId) => {
    const normalized = `${boardId ?? ''}`.trim()
    if (normalized) result.add(normalized)
  })
  return result
}

function resolveHydrateOptions(
  options: HydrateBoardsFromServerOptions,
): ResolvedHydrateBoardsFromServerOptions {
  const requestedRollbackBoardIds = normalizeRollbackBoardIds(options.rollbackBoardIds ?? [])
  const shouldUsePendingRollback = options.preserveLocalNewer === false
  const rollbackBoardIds = new Set([
    ...requestedRollbackBoardIds,
    ...(shouldUsePendingRollback ? pendingRollbackBoardIds : []),
  ])
  return {
    preserveLocalNewer: options.preserveLocalNewer !== false || rollbackBoardIds.size > 0,
    rollbackBoardIds,
  }
}

async function listRemoteBoards(
  workspaceId: string,
): Promise<Array<{ meta: WorkspaceBoardMeta; state: BoardState | null }>> {
  const metas = await fetchWorkspaceBoards(workspaceId)
  return Promise.all(
    metas.map(async (meta) => {
      const document = await fetchWorkspaceBoardDocument(workspaceId, meta.id)
      // Invalid remote snapshots fail the whole hydrate (legacy parity);
      // local boards stay untouched on the failure path below.
      const state = toLocalState(meta.id, document)
      return { meta, state }
    }),
  )
}

const WORKSPACE_SELECTION_READY_TIMEOUT_MS = 5_000

function readWorkspaceListQueryStatus(): 'pending' | 'error' | 'success' | null {
  try {
    return queryClient.getQueryState(workspaceQueryKeys.list)?.status ?? null
  } catch {
    return null
  }
}

function waitForBoardScopeSettled(timeoutMs: number): Promise<void> {
  const current = getBoardScope()
  if (current.workspaceId || !current.userScope) return Promise.resolve()
  return new Promise((resolve) => {
    let settled = false
    const finish = () => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      unsubscribe()
      resolve()
    }
    const unsubscribe = subscribeBoardScopeChange((scope) => {
      if (scope.workspaceId || !scope.userScope) finish()
    })
    const timer = setTimeout(finish, Math.max(0, timeoutMs))
  })
}

async function resolveHydrateScope(options: HydrateBoardsFromServerOptions): Promise<BoardScope> {
  const initial = getBoardScope()
  if (!initial.userScope || initial.workspaceId) return initial
  // Authed without a selection: wait only when explicitly requested or
  // while the workspace list is still loading. Query errors, empty lists,
  // and idle caches fail closed immediately (existing behavior); only a
  // pending list (fresh profile) waits for the ready/failed/timeout tri-state.
  const explicitTimeout = options.waitForWorkspaceTimeoutMs
  const shouldWait =
    explicitTimeout != null ? explicitTimeout > 0 : readWorkspaceListQueryStatus() === 'pending'
  if (!shouldWait) return initial
  const timeoutMs = explicitTimeout != null ? explicitTimeout : WORKSPACE_SELECTION_READY_TIMEOUT_MS
  await waitForBoardScopeSettled(timeoutMs)
  return getBoardScope()
}

export async function hydrateBoardsFromServer(options: HydrateBoardsFromServerOptions = {}) {
  const waited = await resolveHydrateScope(options)
  // Signed-out surfaces (public landing) have no scope and nothing to sync.
  if (!waited.userScope) return { ok: true } satisfies HydrateBoardsFromServerResult
  // Authed without a workspace selection fails closed: the request target
  // must never be inferred from a board id.
  if (!waited.workspaceId) {
    return {
      ok: false,
      error: new BoardWorkspaceUnavailableError(),
    } satisfies HydrateBoardsFromServerResult
  }
  const scope: CompleteBoardScope = {
    userScope: waited.userScope,
    workspaceId: waited.workspaceId,
  }
  const workspaceId = scope.workspaceId
  const resolvedOptions = resolveHydrateOptions(options)
  const rollbackKey = [...resolvedOptions.rollbackBoardIds].sort().join(',')
  const hydrateKey = `${boardScopeKey(scope)}:${resolvedOptions.preserveLocalNewer ? 'preserve' : 'replace'}:${rollbackKey}`
  const inFlight = hydrateInFlight.get(hydrateKey)
  if (inFlight) return inFlight
  const task = (async () => {
    const remoteBoards = await listRemoteBoards(workspaceId)
    if (!isBoardScopeCurrent(scope)) {
      throw new Error('BOARD_HYDRATE_SCOPE_CHANGED')
    }
    applyRemoteBoardsToLocal(remoteBoards, resolvedOptions)
    resolvedOptions.rollbackBoardIds.forEach((boardId) => {
      pendingRollbackBoardIds.delete(boardId)
    })
    return { ok: true } satisfies HydrateBoardsFromServerResult
  })()
    .catch((error) => {
      console.warn('[boards] hydrate from server failed', error)
      return { ok: false, error } satisfies HydrateBoardsFromServerResult
    })
    .finally(() => {
      hydrateInFlight.delete(hydrateKey)
    })
  hydrateInFlight.set(hydrateKey, task)
  return task
}

export async function syncBoardToServer(boardId: string, stateOverride?: BoardState | null) {
  // Fail closed when no workspace is selected (throws with guidance);
  // the save path never infers a workspace from the board id. Resolve the
  // workspace before touching scoped storage: without a selection the
  // catalog reads empty, which must reject instead of silently no-op.
  // Document-only: ordinary saves (autosave included) never PATCH the board
  // name, so a stale local name can never overwrite a collaborator rename.
  // Explicit renames go through `syncBoardRenameToServer` only.
  const workspaceId = requireBoardWorkspaceId()
  const board = getBoardById(boardId)
  if (!board) return
  // Capture the User + Workspace scope: a switch mid-save must not apply
  // this response (revision advance, rollback markers) to the new scope.
  const captured: BoardScope = { ...getBoardScope(), workspaceId }
  const state = stateOverride ?? getBoardState(boardId) ?? createEmptyBoardState(board.updatedAt)
  try {
    const response = await saveWorkspaceBoard(workspaceId, boardId, {
      state: toGeneratedBoardStateProjection(state),
      // v1 `If-Match` requires revision >= 1; an unknown local base
      // reconciles through the 409 rollback path below, never a 400.
      baseRevision: board.revision ?? 1,
    })
    applySyncSuccess(boardId, response, captured)
  } catch (error) {
    markSyncFailure(boardId, captured, error)
    throw error
  }
}

/**
 * Explicit rename path (canvas header, gallery): PATCHes the board name
 * without writing the document. Ordinary document saves never call this,
 * so PATCH/PUT partial failures from the old combined save cannot recur.
 * Advances the local revision base from the rename response so the next
 * document PUT keeps a fresh `If-Match`.
 */
export async function syncBoardRenameToServer(boardId: string, nextName: string): Promise<void> {
  const normalizedBoardId = boardId.trim()
  if (!normalizedBoardId) throw new Error('MISSING_BOARD_ID')
  const workspaceId = requireBoardWorkspaceId()
  if (!getBoardById(normalizedBoardId)) return
  const captured: BoardScope = { ...getBoardScope(), workspaceId }
  try {
    const meta = await renameWorkspaceBoard(workspaceId, normalizedBoardId, nextName)
    if (!isBoardScopeCurrent(captured)) return
    const serverRevision = Number(meta.revision)
    if (Number.isFinite(serverRevision)) setBoardRevision(normalizedBoardId, serverRevision)
    pendingRollbackBoardIds.delete(normalizedBoardId)
  } catch (error) {
    markSyncFailure(normalizedBoardId, captured, error)
    throw error
  }
}

function applySyncSuccess(
  boardId: string,
  response: SaveWorkspaceBoardResult,
  captured: BoardScope,
) {
  // A User or Workspace switch mid-save drops the local follow-up: the
  // server write already happened, but its revision must not land in the
  // new scope's catalog (it converges on the next scoped hydrate).
  if (!isBoardScopeCurrent(captured)) return
  const serverRevision = Number(response.newRevision)
  if (Number.isFinite(serverRevision)) setBoardRevision(boardId, serverRevision)
  pendingRollbackBoardIds.delete(boardId.trim())
}

function markSyncFailure(boardId: string, captured: BoardScope, error: unknown) {
  if (isBoardScopeCurrent(captured) && isBoardSyncConflictError(error)) {
    pendingRollbackBoardIds.add(boardId.trim())
  }
  // Observability for the status-less failure class (offline/timeout,
  // workspace-missing): summary only — status/code/message, never payloads
  // or secrets — so the next "saved locally" report is attributable.
  console.info('[boards] remote sync failed', boardId, summarizeSyncFailure(error))
}

function summarizeSyncFailure(error: unknown): string {
  if (!error || typeof error !== 'object') return String(error)
  const record = error as { status?: unknown; message?: unknown }
  const status = typeof record.status === 'number' ? `status=${record.status}` : null
  const code = userErrorCode(error)
  const message =
    typeof record.message === 'string' && record.message.trim()
      ? record.message.trim().slice(0, 120)
      : null
  return (
    [status, code ? `code=${code}` : null, message].filter(Boolean).join(' ') || 'unknown error'
  )
}

export function clearPendingBoardSyncRollback(boardId: string) {
  const normalized = boardId.trim()
  if (!normalized) return
  pendingRollbackBoardIds.delete(normalized)
}

export async function syncDeleteBoardToServer(boardId: string) {
  // v1 has no hard delete; gallery delete archives (already-archived
  // reads 404 so replays converge to remoteWasMissing upstream).
  await archiveWorkspaceBoard(requireBoardWorkspaceId(), boardId)
}

/**
 * Board sync conflict vocabulary (issue #910).
 *
 * v1 has no board-name uniqueness: every 409 that is not `board_archived`
 * is an optimistic-concurrency signal, never a server name-uniqueness
 * verdict. Local duplicate-name validation (`isBoardNameTaken`) stays the
 * only name gate; these classifiers describe revision/idempotency conflicts
 * only.
 */
export type BoardSyncConflictKind = 'revision-conflict' | 'idempotency-conflict'

function eachErrorInCauseChain(error: unknown): unknown[] {
  const chain: unknown[] = []
  const seen = new Set<unknown>()
  let current: unknown = error
  while (current && typeof current === 'object' && !seen.has(current)) {
    seen.add(current)
    chain.push(current)
    current = (current as { cause?: unknown }).cause
  }
  return chain
}

function hasHttpConflictStatusFallback(error: unknown): boolean {
  return eachErrorInCauseChain(error).some((node) => {
    if (!node || typeof node !== 'object') return false
    const record = node as { status?: unknown; code?: unknown }
    return record.status === 409 || record.code === 409
  })
}

/**
 * Canonical conflict kind for a transport failure: `revision-conflict` for
 * stale `If-Match` writes (`board_revision_conflict`, plus a bare-409
 * HTTP-status fallback for transports that drop the wire code — stale
 * revisions are the only code-less 409 the v1 contract emits), or
 * `idempotency-conflict` for `Idempotency-Key` reuse
 * (`board_idempotency_conflict`). Returns null for archived writes and
 * ordinary failures. Wrapped causes resolve through the shared error
 * catalog, so `BoardSessionRemoteSyncError.cause` chains classify.
 */
export function getBoardSyncConflictKind(error: unknown): BoardSyncConflictKind | null {
  if (isBoardArchivedError(error)) return null
  const code = userErrorCode(error)
  if (code === 'board_revision_conflict') return 'revision-conflict'
  if (code === 'board_idempotency_conflict') return 'idempotency-conflict'
  if (code === 'board_archived') return null
  // Necessary HTTP-status fallback: a bare 409 without a wire code can only
  // be a stale-revision conflict under the v1 contract (idempotency
  // conflicts always carry their wire code; archived is excluded above).
  if (hasHttpConflictStatusFallback(error)) return 'revision-conflict'
  return null
}

/**
 * Any optimistic-concurrency conflict (revision or idempotency). Archived
 * writes are never conflicts: they surface the archived banner instead of
 * the conflict recovery path.
 */
export function isBoardSyncConflictError(error: unknown): boolean {
  return getBoardSyncConflictKind(error) !== null
}

/**
 * Archived-write classifier for the v1 `board_archived` contract: archived
 * writes render HTTP 409 with wire code `board_archived` (previously 403
 * `board_forbidden`). Classification goes through the shared error catalog
 * so wrapped causes (e.g. `BoardSessionRemoteSyncError.cause`) resolve.
 */
export function isBoardArchivedError(error: unknown): boolean {
  return userErrorCode(error) === 'board_archived'
}

/**
 * Reads the remote board meta to learn its archived flag (list excludes
 * archived boards, so hydration alone can never reveal it). Syncs the flag
 * into the local catalog when the board is cached locally; returns the raw
 * `archivedAt` (string when archived, null when live).
 */
export async function fetchBoardArchivedAt(
  boardId: string,
  options: BoardsRequestOptions = {},
): Promise<string | null> {
  const normalized = boardId.trim()
  if (!normalized) throw new Error('MISSING_BOARD_ID')
  const workspaceId = requireBoardWorkspaceId()
  const captured: BoardScope = { ...getBoardScope(), workspaceId }
  const meta = await fetchWorkspaceBoardMeta(workspaceId, normalized, options)
  const archivedAt = typeof meta.archivedAt === 'string' && meta.archivedAt ? meta.archivedAt : null
  if (isBoardScopeCurrent(captured) && getBoardById(normalized)) {
    setBoardArchivedAt(normalized, archivedAt)
  }
  return archivedAt
}

/** Marks the local catalog entry archived without a network call (save-409 fast path). */
export function markBoardArchivedLocally(boardId: string, archivedAt?: string): void {
  const normalized = boardId.trim()
  if (!normalized || !getBoardById(normalized)) return
  setBoardArchivedAt(normalized, archivedAt ?? new Date().toISOString())
}

export async function syncRestoreBoardToServer(boardId: string): Promise<WorkspaceBoardMeta> {
  const normalized = boardId.trim()
  if (!normalized) throw new Error('MISSING_BOARD_ID')
  const workspaceId = requireBoardWorkspaceId()
  const captured: BoardScope = { ...getBoardScope(), workspaceId }
  const meta = await restoreWorkspaceBoard(workspaceId, normalized)
  if (!isBoardScopeCurrent(captured)) return meta
  if (getBoardById(normalized)) {
    setBoardArchivedAt(normalized, null)
    if (Number.isFinite(meta.revision)) setBoardRevision(normalized, meta.revision)
  } else {
    registerBoardMeta({
      id: meta.id,
      name: meta.name,
      createdAt: meta.createdAt,
      updatedAt: meta.updatedAt,
      revision: meta.revision,
    })
  }
  return meta
}
