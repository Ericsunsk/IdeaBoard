import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { UserBoardItem } from '../../../shared/api/board'
import type { CanvasBoardState } from '../../canvas/lib/canvasBoardState'
import { CANVAS_BOARD_STATE_SCHEMA_VERSION } from '../../workspace/types/canvasNodes'
import { clearStoredAuth, setStoredAuth } from '../../../shared/utils/auth'
import { getCanvasBoardState, replaceCanvasBoardStateMap } from '../../canvas/lib/canvasBoardState'
import { listCanvasBoards, replaceCanvasBoards } from './canvasBoards'
import { hydrateCanvasBoardsFromServer, syncCanvasBoardToServer } from './boardSync'

const mocks = vi.hoisted(() => {
  const boardApi = {
    list: vi.fn(),
    upsert: vi.fn(),
    remove: vi.fn(),
    joinByShareToken: vi.fn(),
  }
  return { boardApi }
})

vi.mock('../../../shared/api/board', () => ({
  default: mocks.boardApi,
  boardApi: mocks.boardApi,
}))

function makeState(updatedAt: number, zoomLevel: number): CanvasBoardState {
  return {
    schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
    imageNodes: [],
    promptNodes: [],
    fusionNodes: [],
    connections: [],
    viewport: { canvasX: 0, canvasY: 0, zoomLevel },
    updatedAt,
  }
}

function makeRemoteBoard(params: {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  state?: CanvasBoardState | null
}): UserBoardItem {
  return {
    board_id: params.id,
    name: params.name,
    created_at: params.createdAt,
    updated_at: params.updatedAt,
    revision: 1,
    state_schema_version: CANVAS_BOARD_STATE_SCHEMA_VERSION,
    state: params.state as unknown as UserBoardItem['state'],
  }
}

describe('boardSync', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    setStoredAuth({
      id: 1,
      uid: 'ABC123',
      username: 'tester',
      avatar: '',
      role: 'user',
      status: 1,
      created_at: '2026-01-01T00:00:00.000Z',
      updated_at: '2026-01-01T00:00:00.000Z',
    })
    vi.clearAllMocks()
    mocks.boardApi.list.mockResolvedValue({ data: { list: [] } })
    mocks.boardApi.upsert.mockResolvedValue({ code: 0, data: null })
  })

  it('preserves newer local board meta and state during hydrate', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceCanvasBoardStateMap({ 'board-1': makeState(2_000, 220) })
    mocks.boardApi.list.mockResolvedValue({
      data: {
        list: [
          makeRemoteBoard({
            id: 'board-1',
            name: 'Remote Old',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 100),
          }),
        ],
      },
    })

    await hydrateCanvasBoardsFromServer()

    expect(listCanvasBoards()[0]?.name).toBe('Local Rename')
    expect(getCanvasBoardState('board-1')?.viewport.zoomLevel).toBe(220)
  })

  it('preserves local-only boards while adding remote boards during hydrate', async () => {
    replaceCanvasBoards([
      { id: 'local-only', name: 'Unsynced Local', createdAt: 100, updatedAt: 2_000 },
    ])
    replaceCanvasBoardStateMap({ 'local-only': makeState(2_000, 220) })
    mocks.boardApi.list.mockResolvedValue({
      data: {
        list: [
          makeRemoteBoard({
            id: 'remote-only',
            name: 'Remote Board',
            createdAt: 100,
            updatedAt: 1_500,
            state: makeState(1_500, 150),
          }),
        ],
      },
    })

    await hydrateCanvasBoardsFromServer()

    expect(listCanvasBoards().map((board) => board.id)).toEqual(['local-only', 'remote-only'])
    expect(getCanvasBoardState('local-only')?.viewport.zoomLevel).toBe(220)
    expect(getCanvasBoardState('remote-only')?.viewport.zoomLevel).toBe(150)
  })

  it('rolls back only the conflict target while preserving other local newer and local-only boards', async () => {
    replaceCanvasBoards([
      { id: 'board-a', name: 'Conflicting Local', createdAt: 100, updatedAt: 4_000 },
      { id: 'board-b', name: 'Local Newer', createdAt: 100, updatedAt: 5_000 },
      { id: 'board-c', name: 'Local Only', createdAt: 100, updatedAt: 6_000 },
    ])
    replaceCanvasBoardStateMap({
      'board-a': makeState(4_000, 400),
      'board-b': makeState(5_000, 500),
      'board-c': makeState(6_000, 600),
    })
    mocks.boardApi.upsert.mockRejectedValueOnce({ status: 409, msg: '画布名称已存在' })

    await expect(syncCanvasBoardToServer('board-a')).rejects.toMatchObject({ status: 409 })

    mocks.boardApi.list.mockResolvedValue({
      data: {
        list: [
          makeRemoteBoard({
            id: 'board-a',
            name: 'Remote Winner',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 100),
          }),
          makeRemoteBoard({
            id: 'board-b',
            name: 'Remote Older',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 110),
          }),
        ],
      },
    })

    await hydrateCanvasBoardsFromServer({ preserveLocalNewer: false })

    const boardNames = new Map(listCanvasBoards().map((board) => [board.id, board.name]))
    expect(boardNames.get('board-a')).toBe('Remote Winner')
    expect(boardNames.get('board-b')).toBe('Local Newer')
    expect(boardNames.get('board-c')).toBe('Local Only')
    expect(getCanvasBoardState('board-a')?.viewport.zoomLevel).toBe(100)
    expect(getCanvasBoardState('board-b')?.viewport.zoomLevel).toBe(500)
    expect(getCanvasBoardState('board-c')?.viewport.zoomLevel).toBe(600)
  })

  it('can replace local newer boards when explicitly requested for conflict recovery', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceCanvasBoardStateMap({ 'board-1': makeState(2_000, 220) })
    mocks.boardApi.list.mockResolvedValue({
      data: {
        list: [
          makeRemoteBoard({
            id: 'board-1',
            name: 'Remote Winner',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 100),
          }),
        ],
      },
    })

    await hydrateCanvasBoardsFromServer({ preserveLocalNewer: false })

    expect(listCanvasBoards()[0]?.name).toBe('Remote Winner')
    expect(getCanvasBoardState('board-1')?.viewport.zoomLevel).toBe(100)
  })

  it('returns hydrate failure without clearing local boards', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Board', createdAt: 100, updatedAt: 2_000 }])
    mocks.boardApi.list.mockRejectedValueOnce(new Error('network failed'))

    const result = await hydrateCanvasBoardsFromServer()

    expect(result.ok).toBe(false)
    expect(listCanvasBoards()[0]?.name).toBe('Local Board')
  })

  it('syncs a rename with the board metadata timestamp when state is older', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 3_000 }])
    replaceCanvasBoardStateMap({ 'board-1': makeState(1_000, 100) })

    await syncCanvasBoardToServer('board-1')

    expect(mocks.boardApi.upsert).toHaveBeenCalledWith(
      'board-1',
      expect.objectContaining({
        name: 'Local Rename',
        updated_at: 3_000,
      }),
    )
  })
})
