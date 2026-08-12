import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { UserBoardItem } from '../../../shared/api/generated-query/model'
import type { CanvasBoardState } from '../domain'
import { CANVAS_BOARD_STATE_SCHEMA_VERSION } from '../domain'
import { makeCanvasBoardState } from '../testing'
import { clearStoredAuth, setStoredAuth } from '../../../shared/utils/auth'
import { getCanvasBoardState, replaceCanvasBoardStateMap } from '../persistence'
import { listCanvasBoards, replaceCanvasBoards, subscribeCanvasBoardChanges } from './canvasBoards'
import { hydrateCanvasBoardsFromServer, syncCanvasBoardToServer } from './boardSync'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  const state: {
    listResponse: { code?: number; msg?: string; data: { list: UserBoardItem[] } }
    upsertResponse: { code: number; msg?: string; data: null }
  } = {
    listResponse: { data: { list: [] as UserBoardItem[] } },
    upsertResponse: { code: 0, data: null },
  }
  return { request, state }
})

vi.mock('../../../shared/api/client', () => ({
  api: {
    request: mocks.request,
  },
}))

function makeState(updatedAt: number, promptText: string): CanvasBoardState {
  return makeCanvasBoardState({
    updatedAt,
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

function boardMeta(id: string, name: string, updatedAt: number) {
  return { id, name, createdAt: 100, updatedAt }
}

function getPromptText(boardId: string): string | undefined {
  return getCanvasBoardState(boardId)?.promptNodes[0]?.text
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
      public_id: '00000000-0000-0000-0000-000000000001',
      display_name: 'tester',
      storage_namespace: 'ABC123',
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

  it('preserves newer local board meta and state during hydrate', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceCanvasBoardStateMap({ 'board-1': makeState(2_000, 'local-newer') })
    mocks.state.listResponse = {
      data: {
        list: [
          makeRemoteBoard({
            id: 'board-1',
            name: 'Remote Old',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 'remote-older'),
          }),
        ],
      },
    }

    await hydrateCanvasBoardsFromServer()

    expect(listCanvasBoards()[0]?.name).toBe('Local Rename')
    expect(getPromptText('board-1')).toBe('local-newer')
  })

  it('publishes remote hydrate only after metadata and board state are both committed', async () => {
    mocks.state.listResponse = {
      data: {
        list: [
          makeRemoteBoard({
            id: 'remote-board',
            name: 'Remote Board',
            createdAt: 100,
            updatedAt: 2_000,
            state: makeState(2_000, 'remote-ready'),
          }),
        ],
      },
    }
    const observedPromptTexts: Array<string | undefined> = []
    const unsubscribe = subscribeCanvasBoardChanges(() => {
      observedPromptTexts.push(getPromptText('remote-board'))
    })

    await hydrateCanvasBoardsFromServer()
    unsubscribe()

    expect(observedPromptTexts).toEqual(['remote-ready'])
  })

  it('preserves local-only boards while adding remote boards during hydrate', async () => {
    replaceCanvasBoards([
      { id: 'local-only', name: 'Unsynced Local', createdAt: 100, updatedAt: 2_000 },
    ])
    replaceCanvasBoardStateMap({ 'local-only': makeState(2_000, 'local-only-state') })
    mocks.state.listResponse = {
      data: {
        list: [
          makeRemoteBoard({
            id: 'remote-only',
            name: 'Remote Board',
            createdAt: 100,
            updatedAt: 1_500,
            state: makeState(1_500, 'remote-only-state'),
          }),
        ],
      },
    }

    await hydrateCanvasBoardsFromServer()

    expect(listCanvasBoards().map((board) => board.id)).toEqual(['local-only', 'remote-only'])
    expect(getPromptText('local-only')).toBe('local-only-state')
    expect(getPromptText('remote-only')).toBe('remote-only-state')
  })

  it('rolls back only the conflict target while preserving other local newer and local-only boards', async () => {
    replaceCanvasBoards([
      { id: 'board-a', name: 'Conflicting Local', createdAt: 100, updatedAt: 4_000 },
      { id: 'board-b', name: 'Local Newer', createdAt: 100, updatedAt: 5_000 },
      { id: 'board-c', name: 'Local Only', createdAt: 100, updatedAt: 6_000 },
    ])
    replaceCanvasBoardStateMap({
      'board-a': makeState(4_000, 'local-conflict'),
      'board-b': makeState(5_000, 'local-newer'),
      'board-c': makeState(6_000, 'local-only'),
    })
    mocks.request.mockRejectedValueOnce({ status: 409, msg: '画布名称已存在' })

    await expect(syncCanvasBoardToServer('board-a')).rejects.toMatchObject({ status: 409 })

    mocks.state.listResponse = {
      data: {
        list: [
          makeRemoteBoard({
            id: 'board-a',
            name: 'Remote Winner',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 'remote-winner'),
          }),
          makeRemoteBoard({
            id: 'board-b',
            name: 'Remote Older',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 'remote-older'),
          }),
        ],
      },
    }

    await hydrateCanvasBoardsFromServer({ preserveLocalNewer: false })

    const boardNames = new Map(listCanvasBoards().map((board) => [board.id, board.name]))
    expect(boardNames.get('board-a')).toBe('Remote Winner')
    expect(boardNames.get('board-b')).toBe('Local Newer')
    expect(boardNames.get('board-c')).toBe('Local Only')
    expect(getPromptText('board-a')).toBe('remote-winner')
    expect(getPromptText('board-b')).toBe('local-newer')
    expect(getPromptText('board-c')).toBe('local-only')
  })

  it('can replace local newer boards when explicitly requested for conflict recovery', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceCanvasBoardStateMap({ 'board-1': makeState(2_000, 'local-newer') })
    mocks.state.listResponse = {
      data: {
        list: [
          makeRemoteBoard({
            id: 'board-1',
            name: 'Remote Winner',
            createdAt: 100,
            updatedAt: 1_000,
            state: makeState(1_000, 'remote-winner'),
          }),
        ],
      },
    }

    await hydrateCanvasBoardsFromServer({ preserveLocalNewer: false })

    expect(listCanvasBoards()[0]?.name).toBe('Remote Winner')
    expect(getPromptText('board-1')).toBe('remote-winner')
  })

  it('returns hydrate failure without clearing local boards', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Board', createdAt: 100, updatedAt: 2_000 }])
    mocks.request.mockRejectedValueOnce(new Error('network failed'))

    const result = await hydrateCanvasBoardsFromServer()

    expect(result.ok).toBe(false)
    expect(listCanvasBoards()[0]?.name).toBe('Local Board')
  })

  it('rejects an in-flight hydrate response after the auth scope changes', async () => {
    let resolveRequest!: (value: unknown) => void
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )
    const hydrateTask = hydrateCanvasBoardsFromServer()

    setStoredAuth({
      public_id: '00000000-0000-0000-0000-000000000002',
      display_name: 'other-user',
      storage_namespace: 'XYZ789',
      avatar: '',
      role: 'user',
      status: 1,
      created_at: '2026-01-01T00:00:00.000Z',
      updated_at: '2026-01-01T00:00:00.000Z',
    })
    replaceCanvasBoards([boardMeta('board-b', 'User B Board', 3_000)])
    resolveRequest({
      data: {
        list: [
          makeRemoteBoard({
            id: 'board-a',
            name: 'Late User A Board',
            createdAt: 100,
            updatedAt: 2_000,
            state: makeState(2_000, 'late-user-a'),
          }),
        ],
      },
    })

    await expect(hydrateTask).resolves.toMatchObject({ ok: false })
    expect(listCanvasBoards()).toEqual([boardMeta('board-b', 'User B Board', 3_000)])
    expect(getCanvasBoardState('board-a')).toBeNull()
  })

  it('returns hydrate failure for a successful HTTP response with a failed business code', async () => {
    mocks.state.listResponse = {
      code: 503,
      msg: '服务暂不可用',
      data: { list: [] },
    }

    const result = await hydrateCanvasBoardsFromServer()

    expect(result).toMatchObject({ ok: false, error: expect.any(Error) })
  })

  it('rejects a board save with a failed business code', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Board', createdAt: 100, updatedAt: 2_000 }])
    mocks.state.upsertResponse = {
      code: 409,
      msg: '画布名称已存在',
      message_safe: true,
      data: null,
    }

    await expect(syncCanvasBoardToServer('board-1')).rejects.toThrow('画布名称已存在')
  })

  it('returns hydrate failure for old-shape remote board state without upgrading it', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      mocks.state.listResponse = {
        data: {
          list: [
            makeRemoteBoard({
              id: 'board-older',
              name: 'Older Remote',
              createdAt: 100,
              updatedAt: 1_500,
              state: {
                schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION - 1,
                imageNodes: [
                  {
                    id: 1,
                    type: 'image-node',
                    x: 0,
                    y: 0,
                    width: 280,
                    sourceImageUrl: 'https://example.com/source.png',
                    sourceImageRatio: 1,
                    outputImages: [{ url: 'https://example.com/output.png', ratio: 1 }],
                  },
                ],
                promptNodes: [
                  {
                    id: 2,
                    type: 'prompt',
                    x: 300,
                    y: 0,
                    width: 280,
                    text: 'older remote prompt',
                  },
                ],
                fusionNodes: [],
                connections: [],
                groups: [],
                updatedAt: 1_500,
              } as unknown as CanvasBoardState,
            }),
          ],
        },
      }

      const result = await hydrateCanvasBoardsFromServer()

      expect(result).toMatchObject({
        ok: false,
        error: expect.objectContaining({
          code: 'INVALID_BOARD_STATE',
          boundary: 'server-hydration',
          recoverable: true,
        }),
      })
      expect(getCanvasBoardState('board-older')).toBeNull()
      expect(warnSpy).toHaveBeenCalledWith(
        '[boards] invalid remote board state rejected',
        'board-older',
        expect.objectContaining({
          code: 'INVALID_BOARD_STATE',
          boundary: 'server-hydration',
          recoverable: true,
        }),
      )
    } finally {
      warnSpy.mockRestore()
    }
  })

  it('returns hydrate failure for invalid remote board state without clearing local boards', async () => {
    replaceCanvasBoards([
      { id: 'local-board', name: 'Local Board', createdAt: 100, updatedAt: 2_000 },
    ])
    replaceCanvasBoardStateMap({ 'local-board': makeState(2_000, 'local-state') })
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      mocks.state.listResponse = {
        data: {
          list: [
            makeRemoteBoard({
              id: 'board-invalid',
              name: 'Invalid Remote',
              createdAt: 100,
              updatedAt: 1_500,
              state: {
                ...makeState(1_500, 'invalid remote'),
                imageNodes: [
                  {
                    id: 1,
                    type: 'image-node',
                    x: 0,
                    y: 0,
                    width: 280,
                    sourceImageUrl: 'https://example.com/source.png',
                    sourceImageRatio: 0,
                    outputStatus: 'ready',
                    outputImages: [],
                  },
                ],
              },
            }),
          ],
        },
      }

      const result = await hydrateCanvasBoardsFromServer()

      expect(result).toMatchObject({
        ok: false,
        error: expect.objectContaining({
          code: 'INVALID_BOARD_STATE',
          boundary: 'server-hydration',
          recoverable: true,
        }),
      })
      expect(getCanvasBoardState('board-invalid')).toBeNull()
      expect(getPromptText('local-board')).toBe('local-state')
      expect(warnSpy).toHaveBeenCalledWith(
        '[boards] invalid remote board state rejected',
        'board-invalid',
        expect.objectContaining({
          code: 'INVALID_BOARD_STATE',
          boundary: 'server-hydration',
          recoverable: true,
        }),
      )
    } finally {
      warnSpy.mockRestore()
    }
  })

  it('syncs a rename with the board metadata timestamp when state is older', async () => {
    replaceCanvasBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 3_000 }])
    replaceCanvasBoardStateMap({ 'board-1': makeState(1_000, 'remote-older') })

    await syncCanvasBoardToServer('board-1')

    expect(readUpsertBoardPayload('board-1')).toEqual(
      expect.objectContaining({
        name: 'Local Rename',
        updated_at: 3_000,
      }),
    )
  })

  it('syncs the shared remote envelope shape without viewport', async () => {
    replaceCanvasBoards([
      { id: 'board-1', name: 'Envelope Board', createdAt: 100, updatedAt: 3_000 },
    ])
    replaceCanvasBoardStateMap({
      'board-1': makeCanvasBoardState({
        updatedAt: 2_000,
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
        previewSnapshotDataUrl: 'data:image/webp;base64,preview',
        previewSnapshotUpdatedAt: 2_001,
        previewSnapshotVersion: 4,
      }),
    })

    await syncCanvasBoardToServer('board-1')

    const payload = readUpsertBoardPayload('board-1')
    expect(payload.state).toEqual(
      expect.objectContaining({
        schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
        imageNodes: expect.any(Array),
        promptNodes: expect.any(Array),
        fusionNodes: expect.any(Array),
        connections: expect.any(Array),
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
        previewSnapshotDataUrl: 'data:image/webp;base64,preview',
        previewSnapshotUpdatedAt: 2_001,
        previewSnapshotVersion: 4,
        updatedAt: 2_000,
      }),
    )
    expect(payload.state).not.toHaveProperty('viewport')
  })
})

function mockBoardApiRequest(url: string, config?: { method?: string }) {
  const method = config?.method ?? 'GET'
  if (url === '/user/boards' && method === 'GET') {
    return Promise.resolve({ code: 0, msg: 'ok', ...mocks.state.listResponse })
  }
  if (url.startsWith('/user/boards/') && method === 'PUT') {
    return Promise.resolve(mocks.state.upsertResponse)
  }
  if (url.startsWith('/user/boards/') && method === 'DELETE') {
    return Promise.resolve({ code: 0, data: null })
  }
  return Promise.reject(new Error(`Unexpected board API request: ${method} ${url}`))
}

function readUpsertBoardPayload(boardId: string): Record<string, unknown> {
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
  return JSON.parse(body) as Record<string, unknown>
}
