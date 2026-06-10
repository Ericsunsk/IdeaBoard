import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearStoredAuth, setStoredAuth } from '../../../shared/utils/auth'
import { createCanvasBoard } from '../../boards/lib/canvasBoards'
import {
  getCanvasBoardState,
  replaceCanvasBoardStateMap,
  type CanvasBoardState,
} from '../../canvas/lib/canvasBoardState'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'
import { saveBoardSnapshotFromFlow } from './flowBoardSnapshotSaver'

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

function makeSnapshot(zoomLevel: number): CanvasBoardSnapshot {
  return {
    imageNodes: [],
    promptNodes: [],
    viewpointNodes: [],
    fusionNodes: [],
    connections: [],
    viewport: { canvasX: 0, canvasY: 0, zoomLevel },
  }
}

function makeState(updatedAt: number, zoomLevel: number): CanvasBoardState {
  return {
    ...makeSnapshot(zoomLevel),
    previewSnapshotDataUrl: 'data:image/webp;base64,preview',
    previewSnapshotUpdatedAt: updatedAt,
    previewSnapshotVersion: 1,
    updatedAt,
  }
}

describe('saveBoardSnapshotFromFlow', () => {
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
    mocks.boardApi.upsert.mockResolvedValue({ code: 0, data: null })
  })

  it('drops preview data and still syncs remotely when localStorage write throws', async () => {
    const board = createCanvasBoard('Quota Test')
    replaceCanvasBoardStateMap({ [board.id]: makeState(1_000, 100) })
    const originalSetItem = localStorage.setItem.bind(localStorage)
    let setItemCalls = 0
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const setItemSpy = vi
      .spyOn(localStorage, 'setItem')
      .mockImplementation((key: string, value: string) => {
        setItemCalls += 1
        if (setItemCalls === 1) {
          throw new Error('quota exceeded')
        }
        return originalSetItem(key, value)
      })

    try {
      await saveBoardSnapshotFromFlow(board.id, makeSnapshot(250))
    } finally {
      setItemSpy.mockRestore()
      warnSpy.mockRestore()
    }

    const savedState = getCanvasBoardState(board.id)
    expect(savedState?.viewport.zoomLevel).toBe(250)
    expect(savedState?.previewSnapshotDataUrl).toBeUndefined()
    expect(mocks.boardApi.upsert).toHaveBeenCalledTimes(1)
    const payload = mocks.boardApi.upsert.mock.calls[0]?.[1]
    expect(payload?.state?.viewport.zoomLevel).toBe(250)
    expect(payload?.state?.previewSnapshotDataUrl).toBeUndefined()
  })
})
