import boardApi, { type UserBoardItem } from '../../../shared/api/board'
import { getAuthStorageUserScope } from '../../../shared/utils/auth'
import {
  getCanvasBoardById,
  listCanvasBoards,
  normalizeCanvasBoardName,
  replaceCanvasBoards,
  type CanvasBoardMeta,
} from './canvasBoards'
import {
  getCanvasBoardState,
  replaceCanvasBoardStateMap,
  type CanvasBoardState,
} from '../../canvas/lib/canvasBoardState'

const hydrateInFlight = new Map<string, Promise<void>>()
const pendingRollbackBoardIds = new Set<string>()

export type HydrateCanvasBoardsFromServerOptions = {
  preserveLocalNewer?: boolean
  rollbackBoardIds?: string[]
}

type ResolvedHydrateCanvasBoardsFromServerOptions = {
  preserveLocalNewer: boolean
  rollbackBoardIds: Set<string>
}

function toLocalMeta(board: UserBoardItem): CanvasBoardMeta | null {
  const boardId = board.board_id?.trim()
  const name = normalizeCanvasBoardName(board.name || '')
  if (!boardId || !name) return null
  return {
    id: boardId,
    name,
    createdAt: Number(board.created_at) || Date.now(),
    updatedAt: Number(board.updated_at) || Date.now(),
  }
}

function toLocalState(board: UserBoardItem): CanvasBoardState | null {
  if (!board.state || typeof board.state !== 'object') return null
  const state = board.state as Record<string, unknown>
  return {
    ...state,
    updatedAt:
      typeof state.updatedAt === 'number'
        ? state.updatedAt
        : Number(board.updated_at) || Date.now(),
  } as CanvasBoardState
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
  options: ResolvedHydrateCanvasBoardsFromServerOptions,
) {
  const metas: CanvasBoardMeta[] = []
  const stateMap: Record<string, CanvasBoardState> = {}
  boards.forEach((board) => {
    const meta = toLocalMeta(board)
    if (!meta) return
    metas.push(meta)
    const state = toLocalState(board)
    if (state) stateMap[meta.id] = state
  })
  if (!options.preserveLocalNewer) {
    replaceCanvasBoards(metas)
    replaceCanvasBoardStateMap(stateMap)
    return
  }

  const mergedMetas = new Map<string, CanvasBoardMeta>()
  const localMetas = listCanvasBoards()
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

  const mergedStateMap: Record<string, CanvasBoardState> = {}
  mergedMetas.forEach((_, boardId) => {
    const localState = getCanvasBoardState(boardId)
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

  replaceCanvasBoards([...mergedMetas.values()])
  replaceCanvasBoardStateMap(mergedStateMap)
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
  options: HydrateCanvasBoardsFromServerOptions,
): ResolvedHydrateCanvasBoardsFromServerOptions {
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
  const res = await boardApi.list()
  return Array.isArray(res.data?.list) ? res.data.list : []
}

export async function hydrateCanvasBoardsFromServer(
  options: HydrateCanvasBoardsFromServerOptions = {},
) {
  const scope = getAuthStorageUserScope()
  if (!scope) return
  const resolvedOptions = resolveHydrateOptions(options)
  const rollbackKey = [...resolvedOptions.rollbackBoardIds].sort().join(',')
  const hydrateKey = `${scope}:${resolvedOptions.preserveLocalNewer ? 'preserve' : 'replace'}:${rollbackKey}`
  const inFlight = hydrateInFlight.get(hydrateKey)
  if (inFlight) return inFlight
  const task = (async () => {
    const remoteBoards = await listRemoteBoards()
    applyRemoteBoardsToLocal(remoteBoards, resolvedOptions)
    resolvedOptions.rollbackBoardIds.forEach((boardId) => {
      pendingRollbackBoardIds.delete(boardId)
    })
  })()
    .catch((error) => {
      console.warn('[boards] hydrate from server failed', error)
    })
    .finally(() => {
      hydrateInFlight.delete(hydrateKey)
    })
  hydrateInFlight.set(hydrateKey, task)
  return task
}

export async function syncCanvasBoardToServer(
  boardId: string,
  stateOverride?: CanvasBoardState | null,
) {
  const board = getCanvasBoardById(boardId)
  if (!board) return
  const state = stateOverride ?? getCanvasBoardState(boardId)
  try {
    await boardApi.upsert(boardId, {
      name: board.name,
      state,
      created_at: board.createdAt,
      updated_at: Math.max(board.updatedAt, state?.updatedAt ?? 0),
    })
  } catch (error) {
    if (isBoardNameConflictError(error)) {
      pendingRollbackBoardIds.add(boardId.trim())
    }
    throw error
  }
}

export async function syncDeleteCanvasBoardToServer(boardId: string) {
  await boardApi.remove(boardId)
}

export function isBoardNameConflictError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 409) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  if (code === 409) return true
  const msg = 'msg' in error ? (error as { msg?: unknown }).msg : undefined
  return typeof msg === 'string' && msg.includes('画布名称已存在')
}
