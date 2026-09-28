import {
  archiveWorkspaceBoard,
  fetchWorkspaceBoardDocument,
  fetchWorkspaceBoardMeta,
  fetchWorkspaceBoards,
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
  getAuthStorageUserScope,
  subscribeAuthStorageScopeChange,
} from '../../../../shared/utils/auth'
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

export type HydrateBoardsFromServerResult = { ok: true } | { ok: false; error: unknown }

const hydrateInFlight = new Map<string, Promise<HydrateBoardsFromServerResult>>()
const pendingRollbackBoardIds = new Set<string>()
subscribeAuthStorageScopeChange(() => pendingRollbackBoardIds.clear())

export type HydrateBoardsFromServerOptions = {
  preserveLocalNewer?: boolean
  rollbackBoardIds?: string[]
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
    // Keep the local optimistic content, but always advance its concurrency
    // base to the server revision observed during hydration.
    mergedMetas.set(
      remoteMeta.id,
      preserveLocal
        ? {
            ...localMeta,
            ...(remoteMeta.revision === undefined ? {} : { revision: remoteMeta.revision }),
          }
        : remoteMeta,
    )
  })

  localMetas.forEach((localMeta) => {
    if (mergedMetas.has(localMeta.id) || !shouldPreserveLocal(localMeta.id)) return
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

export async function hydrateBoardsFromServer(options: HydrateBoardsFromServerOptions = {}) {
  const scope = getAuthStorageUserScope()
  if (!scope) return { ok: true } satisfies HydrateBoardsFromServerResult
  let workspaceId: string
  try {
    workspaceId = requireBoardWorkspaceId()
  } catch (error) {
    // Fail closed: no workspace selection means no sync, never an
    // inferred workspace. Callers surface the guidance via feedback.
    return { ok: false, error } satisfies HydrateBoardsFromServerResult
  }
  const resolvedOptions = resolveHydrateOptions(options)
  const rollbackKey = [...resolvedOptions.rollbackBoardIds].sort().join(',')
  const hydrateKey = `${scope}:${workspaceId}:${resolvedOptions.preserveLocalNewer ? 'preserve' : 'replace'}:${rollbackKey}`
  const inFlight = hydrateInFlight.get(hydrateKey)
  if (inFlight) return inFlight
  const task = (async () => {
    const remoteBoards = await listRemoteBoards(workspaceId)
    if (getAuthStorageUserScope() !== scope) {
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
  const syncScope = getAuthStorageUserScope()
  const board = getBoardById(boardId)
  if (!board) return
  // Fail closed when no workspace is selected (throws with guidance);
  // the save path never infers a workspace from the board id.
  const workspaceId = requireBoardWorkspaceId()
  const state = stateOverride ?? getBoardState(boardId) ?? createEmptyBoardState(board.updatedAt)
  try {
    const response = await saveWorkspaceBoard(workspaceId, boardId, {
      name: board.name,
      state: toGeneratedBoardStateProjection(state),
      // v1 `If-Match` requires revision >= 1; an unknown local base
      // reconciles through the 409 rollback path below, never a 400.
      baseRevision: board.revision ?? 1,
    })
    applySyncSuccess(boardId, response, syncScope)
  } catch (error) {
    markSyncFailure(boardId, syncScope, error)
    throw error
  }
}

function applySyncSuccess(
  boardId: string,
  response: SaveWorkspaceBoardResult,
  syncScope: string | null,
) {
  const serverRevision = Number(response.newRevision)
  if (Number.isFinite(serverRevision)) setBoardRevision(boardId, serverRevision)
  if (syncScope && getAuthStorageUserScope() === syncScope) {
    pendingRollbackBoardIds.delete(boardId.trim())
  }
}

function markSyncFailure(boardId: string, syncScope: string | null, error: unknown) {
  if (syncScope && getAuthStorageUserScope() === syncScope && isBoardNameConflictError(error)) {
    pendingRollbackBoardIds.add(boardId.trim())
  }
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

export function isBoardNameConflictError(error: unknown): boolean {
  if (isBoardArchivedError(error)) return false
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 409) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  // v1 has no board-name uniqueness: 409s are `board_revision_conflict`
  // (stale If-Match) or `board_idempotency_conflict`. They drive the same
  // rollback/reconcile policy the legacy name conflict used.
  // `board_archived` (409) is excluded above: archived writes surface the
  // archived banner instead of the conflict recovery path.
  return code === 409 || code === 'board_revision_conflict' || code === 'board_idempotency_conflict'
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
  const meta = await fetchWorkspaceBoardMeta(requireBoardWorkspaceId(), normalized, options)
  const archivedAt = typeof meta.archivedAt === 'string' && meta.archivedAt ? meta.archivedAt : null
  if (getBoardById(normalized)) {
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
  const meta = await restoreWorkspaceBoard(requireBoardWorkspaceId(), normalized)
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
