import {
  fetchUserBoards,
  removeUserBoard,
  saveUserBoard,
  type BoardStateEnvelope,
  type UserBoardItem,
} from '../../infrastructure/api/boardsResource'
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
  replaceBoards,
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

function toLocalMeta(board: UserBoardItem): BoardMeta | null {
  const boardId = board.board_id?.trim()
  const name = normalizeBoardName(board.name || '')
  if (!boardId || !name) return null
  return {
    id: boardId,
    name,
    createdAt: Number(board.created_at) || Date.now(),
    updatedAt: Number(board.updated_at) || Date.now(),
  }
}

function toLocalState(board: UserBoardItem): BoardState | null {
  const result = normalizeBoardState(board.state, {
    mode: 'result',
    boundary: 'server-hydration',
  })
  if (result.ok) return result.value
  const boardId = board.board_id?.trim() || ''
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

function toGeneratedBoardStateProjection(state: BoardState): BoardStateEnvelope {
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
  boards: UserBoardItem[],
  options: ResolvedHydrateBoardsFromServerOptions,
) {
  const metas: BoardMeta[] = []
  const stateMap: Record<string, BoardState> = {}
  boards.forEach((board) => {
    const meta = toLocalMeta(board)
    if (!meta) return
    metas.push(meta)
    const state = toLocalState(board)
    if (state) stateMap[meta.id] = state
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
    mergedMetas.set(
      remoteMeta.id,
      localMeta &&
        shouldPreserveLocal(remoteMeta.id) &&
        isLocalNewer(localMeta.updatedAt, remoteMeta.updatedAt)
        ? localMeta
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

async function listRemoteBoards(): Promise<UserBoardItem[]> {
  return fetchUserBoards()
}

export async function hydrateBoardsFromServer(
  options: HydrateBoardsFromServerOptions = {},
) {
  const scope = getAuthStorageUserScope()
  if (!scope) return { ok: true } satisfies HydrateBoardsFromServerResult
  const resolvedOptions = resolveHydrateOptions(options)
  const rollbackKey = [...resolvedOptions.rollbackBoardIds].sort().join(',')
  const hydrateKey = `${scope}:${resolvedOptions.preserveLocalNewer ? 'preserve' : 'replace'}:${rollbackKey}`
  const inFlight = hydrateInFlight.get(hydrateKey)
  if (inFlight) return inFlight
  const task = (async () => {
    const remoteBoards = await listRemoteBoards()
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

export async function syncBoardToServer(
  boardId: string,
  stateOverride?: BoardState | null,
) {
  const syncScope = getAuthStorageUserScope()
  const board = getBoardById(boardId)
  if (!board) return
  const state =
    stateOverride ?? getBoardState(boardId) ?? createEmptyBoardState(board.updatedAt)
  try {
    await saveUserBoard(boardId, {
      name: board.name,
      state: toGeneratedBoardStateProjection(state),
      created_at: board.createdAt,
      updated_at: Math.max(board.updatedAt, state?.updatedAt ?? 0),
    })
    if (syncScope && getAuthStorageUserScope() === syncScope) {
      pendingRollbackBoardIds.delete(boardId.trim())
    }
  } catch (error) {
    if (syncScope && getAuthStorageUserScope() === syncScope && isBoardNameConflictError(error)) {
      pendingRollbackBoardIds.add(boardId.trim())
    }
    throw error
  }
}

export function clearPendingBoardSyncRollback(boardId: string) {
  const normalized = boardId.trim()
  if (!normalized) return
  pendingRollbackBoardIds.delete(normalized)
}

export async function syncDeleteBoardToServer(boardId: string) {
  await removeUserBoard(boardId)
}

export function isBoardNameConflictError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 409) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 409
}
