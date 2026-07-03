import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearStoredAuth, setStoredAuth } from '../../../shared/utils/auth'
import {
  createCanvasBoard,
  getCanvasBoardById,
} from '../../canvas-board/catalog/canvasBoards'
import {
  getCanvasBoardState,
  replaceCanvasBoardStateMap,
} from '../../canvas-board/persistence'
import type { CanvasBoardSnapshot, CanvasBoardState } from '../../canvas-board/domain'
import {
  makeCanvasBoardSnapshot,
  makeCanvasBoardState,
  makeCanvasViewport,
} from '../../canvas-board/testing'
import {
  createBoardSessionRepository,
  isBoardSessionRemoteSyncError,
} from './boardSessionRepository'

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
  return makeCanvasBoardSnapshot({ viewport: makeCanvasViewport({ zoomLevel }) })
}

function makeState(updatedAt: number, zoomLevel: number): CanvasBoardState {
  return makeCanvasBoardState({
    ...makeSnapshot(zoomLevel),
    previewSnapshotDataUrl: 'data:image/webp;base64,preview',
    previewSnapshotUpdatedAt: updatedAt,
    previewSnapshotVersion: 1,
    updatedAt,
  })
}

describe('createBoardSessionRepository', () => {
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
    const repository = createBoardSessionRepository()
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
      await repository.save(board.id, makeSnapshot(250))
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

  it('keeps the local state and reports recoverable remote sync failure', async () => {
    const nowSpy = vi.spyOn(Date, 'now').mockReturnValue(1_000)
    try {
      const board = createCanvasBoard('Remote Failure')
      replaceCanvasBoardStateMap({ [board.id]: makeState(1_000, 100) })
      const repository = createBoardSessionRepository()
      const remoteError = new Error('network down')
      mocks.boardApi.upsert.mockRejectedValueOnce(remoteError)
      nowSpy.mockReturnValue(2_000)

      let thrown: unknown
      try {
        await repository.save(board.id, makeSnapshot(300))
      } catch (error) {
        thrown = error
      }

      const savedState = getCanvasBoardState(board.id)
      expect(savedState?.viewport.zoomLevel).toBe(300)
      expect(savedState?.updatedAt).toBe(2_000)
      expect(getCanvasBoardById(board.id)?.updatedAt).toBe(2_000)
      expect(isBoardSessionRemoteSyncError(thrown)).toBe(true)
      if (!isBoardSessionRemoteSyncError(thrown)) return
      expect(thrown.boardId).toBe(board.id)
      expect(thrown.savedState).toEqual(savedState)
      expect(thrown.cause).toBe(remoteError)
      expect(mocks.boardApi.upsert).toHaveBeenCalledTimes(1)
    } finally {
      nowSpy.mockRestore()
    }
  })
})
