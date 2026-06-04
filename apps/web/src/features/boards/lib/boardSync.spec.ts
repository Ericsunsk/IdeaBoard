import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { UserBoardItem } from '../../../shared/api/board'
import type { CanvasBoardState } from '../../canvas/lib/canvasBoardState'
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
    imageNodes: [],
    promptNodes: [],
    viewpointNodes: [],
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
    state: (params.state ?? null) as UserBoardItem['state'],
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

  it('drops local-only boards while adding remote boards during hydrate', async () => {
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

    expect(listCanvasBoards().map((board) => board.id)).toEqual(['remote-only'])
    expect(getCanvasBoardState('local-only')).toBeNull()
    expect(getCanvasBoardState('remote-only')?.viewport.zoomLevel).toBe(150)
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
