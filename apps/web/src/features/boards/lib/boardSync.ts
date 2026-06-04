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

export type HydrateCanvasBoardsFromServerOptions = {
  preserveLocalNewer?: boolean
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
  options: Required<HydrateCanvasBoardsFromServerOptions>,
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

  metas.forEach((remoteMeta) => {
    const localMeta = localMetaMap.get(remoteMeta.id)
    mergedMetas.set(
      remoteMeta.id,
      localMeta && isLocalNewer(localMeta.updatedAt, remoteMeta.updatedAt) ? localMeta : remoteMeta,
    )
  })

  const mergedStateMap: Record<string, CanvasBoardState> = {}
  mergedMetas.forEach((_, boardId) => {
    const localState = getCanvasBoardState(boardId)
    const remoteState = stateMap[boardId]
    if (localState && remoteState && isLocalNewer(localState.updatedAt, remoteState.updatedAt)) {
      mergedStateMap[boardId] = localState
      return
    }
    if (remoteState) {
      mergedStateMap[boardId] = remoteState
    }
  })

  replaceCanvasBoards([...mergedMetas.values()])
  replaceCanvasBoardStateMap(mergedStateMap)
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
  const resolvedOptions = { preserveLocalNewer: options.preserveLocalNewer !== false }
  const hydrateKey = `${scope}:${resolvedOptions.preserveLocalNewer ? 'preserve' : 'replace'}`
  const inFlight = hydrateInFlight.get(hydrateKey)
  if (inFlight) return inFlight
  const task = (async () => {
    const remoteBoards = await listRemoteBoards()
    applyRemoteBoardsToLocal(remoteBoards, resolvedOptions)
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

export async function syncCanvasBoardToServer(boardId: string) {
  const board = getCanvasBoardById(boardId)
  if (!board) return
  const state = getCanvasBoardState(boardId)
  await boardApi.upsert(boardId, {
    name: board.name,
    state,
    created_at: board.createdAt,
    updated_at: Math.max(board.updatedAt, state?.updatedAt ?? 0),
  })
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
