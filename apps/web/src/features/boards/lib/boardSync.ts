import boardApi, { type UserBoardItem } from '../../../shared/api/board'
import { getAuthStorageUserScope } from '../../../shared/utils/auth'
import {
  getCanvasBoardById,
  replaceCanvasBoards,
  type CanvasBoardMeta,
} from './canvasBoards'
import {
  getCanvasBoardState,
  replaceCanvasBoardStateMap,
  type CanvasBoardState,
} from './canvasBoardState'

const hydrateInFlight = new Map<string, Promise<void>>()

function toLocalMeta(board: UserBoardItem): CanvasBoardMeta | null {
  const boardId = board.board_id?.trim()
  const name = board.name?.trim()
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
    updatedAt: typeof state.updatedAt === 'number' ? state.updatedAt : Number(board.updated_at) || Date.now(),
  } as CanvasBoardState
}

function applyRemoteBoardsToLocal(boards: UserBoardItem[]) {
  const metas: CanvasBoardMeta[] = []
  const stateMap: Record<string, CanvasBoardState> = {}
  boards.forEach((board) => {
    const meta = toLocalMeta(board)
    if (!meta) return
    metas.push(meta)
    const state = toLocalState(board)
    if (state) stateMap[meta.id] = state
  })
  replaceCanvasBoards(metas)
  replaceCanvasBoardStateMap(stateMap)
}

async function listRemoteBoards(): Promise<UserBoardItem[]> {
  const res = await boardApi.list()
  return Array.isArray(res.data?.list) ? res.data.list : []
}

export async function hydrateCanvasBoardsFromServer() {
  const scope = getAuthStorageUserScope()
  if (!scope) return
  const inFlight = hydrateInFlight.get(scope)
  if (inFlight) return inFlight
  const task = (async () => {
    const remoteBoards = await listRemoteBoards()
    applyRemoteBoardsToLocal(remoteBoards)
  })().catch((error) => {
    console.warn('[boards] hydrate from server failed', error)
  }).finally(() => {
    hydrateInFlight.delete(scope)
  })
  hydrateInFlight.set(scope, task)
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
    updated_at: state?.updatedAt ?? board.updatedAt,
  })
}

export async function syncDeleteCanvasBoardToServer(boardId: string) {
  await boardApi.remove(boardId)
}
