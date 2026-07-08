import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearStoredAuth, setStoredAuth } from '../../../shared/utils/auth'
import { createCanvasBoard, getCanvasBoardById } from '../../canvas-board/catalog/canvasBoards'
import { getCanvasBoardState, replaceCanvasBoardStateMap } from '../../canvas-board/persistence'
import type { CanvasBoardSnapshot, CanvasBoardState } from '../../canvas-board/domain'
import {
  DEFAULT_PROMPT_ASPECT_RATIO,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
} from '../../canvas-board/domain'
import { makeCanvasBoardSnapshot, makeCanvasBoardState } from '../../canvas-board/testing'
import {
  createBoardSessionRepository,
  isBoardSessionRemoteSyncError,
} from './boardSessionRepository'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  const state = {
    listResponse: { data: { list: [] } },
    upsertResponse: { code: 0, data: null },
  }
  return { request, state }
})

vi.mock('../../../shared/api/client', () => ({
  api: {
    request: mocks.request,
  },
}))

function makeSnapshot(promptText: string): CanvasBoardSnapshot {
  return makeCanvasBoardSnapshot({
    promptNodes: [
      {
        id: 1,
        type: 'prompt',
        x: 0,
        y: 0,
        width: 280,
        text: promptText,
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ],
  })
}

function makeState(updatedAt: number, promptText: string): CanvasBoardState {
  return makeCanvasBoardState({
    ...makeSnapshot(promptText),
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
    mocks.state.listResponse = { data: { list: [] } }
    mocks.state.upsertResponse = { code: 0, data: null }
    mocks.request.mockImplementation(mockBoardApiRequest)
  })

  it('drops preview data and still syncs remotely when localStorage write throws', async () => {
    const board = createCanvasBoard('Quota Test')
    replaceCanvasBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
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
      await repository.save(board.id, makeSnapshot('saved'))
    } finally {
      setItemSpy.mockRestore()
      warnSpy.mockRestore()
    }

    const savedState = getCanvasBoardState(board.id)
    expect(savedState?.promptNodes[0]?.text).toBe('saved')
    expect(savedState).not.toHaveProperty('viewport')
    expect(savedState?.previewSnapshotDataUrl).toBeUndefined()
    expect(mocks.request).toHaveBeenCalledTimes(1)
    const payload = readUpsertBoardPayload(board.id)
    expect(payload?.state?.promptNodes[0]?.text).toBe('saved')
    expect(payload?.state).not.toHaveProperty('viewport')
    expect(payload?.state?.previewSnapshotDataUrl).toBeUndefined()
  })

  it('keeps the local state and reports recoverable remote sync failure', async () => {
    const nowSpy = vi.spyOn(Date, 'now').mockReturnValue(1_000)
    try {
      const board = createCanvasBoard('Remote Failure')
      replaceCanvasBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
      const repository = createBoardSessionRepository()
      const remoteError = new Error('network down')
      mocks.request.mockRejectedValueOnce(remoteError)
      nowSpy.mockReturnValue(2_000)

      let thrown: unknown
      try {
        await repository.save(board.id, makeSnapshot('saved-after-remote-failure'))
      } catch (error) {
        thrown = error
      }

      const savedState = getCanvasBoardState(board.id)
      expect(savedState?.promptNodes[0]?.text).toBe('saved-after-remote-failure')
      expect(savedState).not.toHaveProperty('viewport')
      expect(savedState?.updatedAt).toBe(2_000)
      expect(getCanvasBoardById(board.id)?.updatedAt).toBe(2_000)
      expect(isBoardSessionRemoteSyncError(thrown)).toBe(true)
      if (!isBoardSessionRemoteSyncError(thrown)) return
      expect(thrown.boardId).toBe(board.id)
      expect(thrown.savedState).toEqual(savedState)
      expect(thrown.cause).toBe(remoteError)
      expect(mocks.request).toHaveBeenCalledTimes(1)
    } finally {
      nowSpy.mockRestore()
    }
  })

  it('keeps saving local board drafts when stored auth is temporarily missing', async () => {
    const board = createCanvasBoard('Session Recovery')
    replaceCanvasBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
    clearStoredAuth()
    const repository = createBoardSessionRepository()

    const savedState = await repository.saveLocal(
      board.id,
      makeSnapshot('saved-without-stored-auth'),
    )

    expect(repository.exists(board.id)).toBe(true)
    expect(savedState.promptNodes[0]?.text).toBe('saved-without-stored-auth')
    expect(getCanvasBoardState(board.id)?.promptNodes[0]?.text).toBe('saved-without-stored-auth')
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('loads a session snapshot without preview or local metadata', async () => {
    const board = createCanvasBoard('Load Snapshot')
    replaceCanvasBoardStateMap({
      [board.id]: makeCanvasBoardState({
        ...makeSnapshot('loaded'),
        updatedAt: 2_000,
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1' }],
        previewSnapshotDataUrl: 'data:image/webp;base64,preview',
        previewSnapshotUpdatedAt: 2_001,
        previewSnapshotVersion: 2,
      }),
    })
    const repository = createBoardSessionRepository()

    const snapshot = await repository.load(board.id)

    expect(snapshot?.promptNodes[0]?.text).toBe('loaded')
    expect(snapshot?.mediaRefs).toEqual([
      { mediaAssetId: 12, refType: 'board_node', refId: 'image-1' },
    ])
    expect(snapshot).not.toHaveProperty('updatedAt')
    expect(snapshot).not.toHaveProperty('previewSnapshotDataUrl')
    expect(snapshot).not.toHaveProperty('viewport')
  })

  it('loads a normalizable older local state as a latest-schema session snapshot', async () => {
    const board = createCanvasBoard('Older Local Snapshot')
    replaceCanvasBoardStateMap({
      [board.id]: {
        schemaVersion: 1,
        imageNodes: [],
        promptNodes: [
          {
            id: 1,
            type: 'prompt',
            x: 0,
            y: 0,
            width: 280,
            text: 'older local',
          },
        ],
        fusionNodes: [],
        connections: [],
        updatedAt: 2_000,
      } as unknown as CanvasBoardState,
    })
    const repository = createBoardSessionRepository()

    const snapshot = await repository.load(board.id)

    expect(snapshot?.promptNodes[0]).toEqual(
      expect.objectContaining({
        text: 'older local',
        outputCount: DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
        aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
        inputImages: [],
      }),
    )
    expect(snapshot).not.toHaveProperty('updatedAt')
  })

  it('rejects invalid save snapshots with recoverable validation context', async () => {
    const board = createCanvasBoard('Invalid Save')
    const repository = createBoardSessionRepository()
    const invalidSnapshot = {
      ...makeCanvasBoardSnapshot(),
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'https://example.com/source.png',
          sourceImageRatio: 1,
          outputStatus: 'not-a-status',
          outputImages: [],
        } as unknown as CanvasBoardSnapshot['imageNodes'][number],
      ],
    } as CanvasBoardSnapshot

    await expect(repository.save(board.id, invalidSnapshot)).rejects.toMatchObject({
      code: 'INVALID_BOARD_STATE',
      boundary: 'board-save',
      recoverable: true,
    })
  })
})

function mockBoardApiRequest(url: string, config?: { method?: string }) {
  const method = config?.method ?? 'GET'
  if (url === '/user/boards' && method === 'GET') return Promise.resolve(mocks.state.listResponse)
  if (url.startsWith('/user/boards/') && method === 'PUT') {
    return Promise.resolve(mocks.state.upsertResponse)
  }
  if (url.startsWith('/user/boards/') && method === 'DELETE') {
    return Promise.resolve({ code: 0, data: null })
  }
  return Promise.reject(new Error(`Unexpected board API request: ${method} ${url}`))
}

function readUpsertBoardPayload(boardId: string): { state?: CanvasBoardState } {
  const call = mocks.request.mock.calls.find(
    ([url, config]) =>
      url === `/user/boards/${boardId}` &&
      typeof config === 'object' &&
      config !== null &&
      'method' in config &&
      config.method === 'PUT',
  )
  if (!call) throw new Error(`Missing upsert call for ${boardId}`)
  const body = call[1]?.body
  if (typeof body !== 'string') throw new Error(`Missing upsert body for ${boardId}`)
  return JSON.parse(body) as { state?: CanvasBoardState }
}
