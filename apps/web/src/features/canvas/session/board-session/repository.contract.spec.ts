import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearStoredAuth, setStoredAuth } from '../../../../core/utils/auth'
import {
  createBoard,
  getBoardById,
  getBoardState,
  InvalidBoardStateError,
  replaceBoardStateMap,
  BOARD_STATE_SCHEMA_VERSION,
  type BoardSnapshot,
  type BoardState,
} from '../../../board'
import { makeBoardSnapshot, makeBoardState } from '../../../board/ports/testing'
import { createBoardSessionRepository, isBoardSessionRemoteSyncError } from './repository'
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../workspace'
import { resetMediaWorkspaceIdResolver } from '../../../../api/mediaAssetDelivery'
import { queryClient } from '../../../../core/query/queryClient'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  return { request }
})

vi.mock('../../../../api/client', () => ({
  api: {
    request: mocks.request,
  },
}))

const WORKSPACE_ID = 'ws_test123'

type V1BoardDto = {
  id: string
  workspace_id: string
  name: string
  revision: number
  schema_version: number
  archived_at: string | null
  created_at: string
  updated_at: string
}

type V1BoardDocument = {
  board_id: string
  revision: number
  schema_version: number
  document: Record<string, unknown>
  updated_at: string
}

const v1Boards: V1BoardDto[] = []
const v1Documents: Record<string, V1BoardDocument> = {}

function resetV1BoardApi() {
  v1Boards.length = 0
  for (const key of Object.keys(v1Documents)) delete v1Documents[key]
}

function findOrCreateV1BoardDto(boardId: string): V1BoardDto {
  let dto = v1Boards.find((item) => item.id === boardId)
  if (!dto) {
    dto = {
      id: boardId,
      workspace_id: WORKSPACE_ID,
      name: 'Board',
      revision: 1,
      schema_version: BOARD_STATE_SCHEMA_VERSION,
      archived_at: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    v1Boards.push(dto)
  }
  return dto
}

function parseV1JsonBody(body: unknown): {
  data?: { schema_version?: number; payload?: Record<string, unknown>; name?: string }
} {
  if (typeof body === 'string') {
    try {
      return JSON.parse(body) as {
        data?: { schema_version?: number; payload?: Record<string, unknown>; name?: string }
      }
    } catch {
      return {}
    }
  }
  if (body && typeof body === 'object') {
    return body as {
      data?: { schema_version?: number; payload?: Record<string, unknown>; name?: string }
    }
  }
  return {}
}

type V1RequestConfig = { method?: string; body?: unknown }

function readV1Document(boardId: string): Promise<unknown> {
  const document = v1Documents[boardId]
  if (!document) {
    return Promise.reject({ status: 404, code: 'board_not_found', msg: 'board missing' })
  }
  return Promise.resolve({ data: document })
}

function replaceV1Document(boardId: string, config?: V1RequestConfig): Promise<unknown> {
  const payload = parseV1JsonBody(config?.body)
  const previous = v1Documents[boardId]
  const next: V1BoardDocument = {
    board_id: boardId,
    revision: Number(previous?.revision ?? 1) + 1,
    schema_version: Number(payload?.data?.schema_version ?? BOARD_STATE_SCHEMA_VERSION),
    document: payload?.data?.payload ?? {},
    updated_at: new Date().toISOString(),
  }
  v1Documents[boardId] = next
  findOrCreateV1BoardDto(boardId)
  return Promise.resolve({ data: next })
}

function readOrRenameV1Board(boardId: string, config?: V1RequestConfig): Promise<unknown> {
  const dto = findOrCreateV1BoardDto(boardId)
  if (config?.method === 'PATCH') {
    const payload = parseV1JsonBody(config?.body)
    if (typeof payload?.data?.name === 'string') dto.name = payload.data.name
  }
  return Promise.resolve({ data: { ...dto } })
}

function routeV1DocumentRequest(
  boardId: string,
  method: string,
  config: V1RequestConfig | undefined,
): Promise<unknown> | null {
  if (method === 'GET') return readV1Document(boardId)
  if (method === 'PUT') return replaceV1Document(boardId, config)
  return null
}

function routeV1BoardRequest(url: string, config: V1RequestConfig | undefined): Promise<unknown> {
  const method = config?.method ?? 'GET'
  const path = url.split('?')[0] ?? url
  if (path === `/v1/workspaces/${WORKSPACE_ID}/boards` && method === 'GET') {
    return Promise.resolve({
      data: [...v1Boards],
      meta: { has_more: false, next_cursor: null },
    })
  }
  const documentMatch = path.match(/^\/v1\/workspaces\/[^/]+\/boards\/([^/]+)\/document$/)
  const documentResult = documentMatch
    ? routeV1DocumentRequest(documentMatch[1] ?? '', method, config)
    : null
  if (documentResult) return documentResult
  const boardMatch = path.match(/^\/v1\/workspaces\/[^/]+\/boards\/([^/]+)$/)
  if (boardMatch && (method === 'GET' || method === 'PATCH')) {
    return readOrRenameV1Board(boardMatch[1] ?? '', config)
  }
  return Promise.reject(new Error(`Unexpected board API request: ${method} ${url}`))
}

/** Pure v1 workspace boards mock: unknown (legacy) URLs fail closed. */
function createV1BoardApiRequest() {
  return (url: string, config?: V1RequestConfig) => routeV1BoardRequest(url, config)
}

function makeSnapshot(promptText: string): BoardSnapshot {
  return makeBoardSnapshot({
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

function makeState(updatedAt: number, promptText: string): BoardState {
  return makeBoardState({
    ...makeSnapshot(promptText),
    updatedAt,
  })
}

describe('createBoardSessionRepository', () => {
  beforeEach(() => {
    queryClient.clear()
    localStorage.clear()
    clearStoredAuth()
    setStoredAuth({
      public_id: '00000000-0000-0000-0000-000000000001',
      display_name: 'tester',
      avatar: '',
      role: 'user',
    })
    vi.clearAllMocks()
    resetV1BoardApi()
    mocks.request.mockImplementation(createV1BoardApiRequest())
    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().select(WORKSPACE_ID)
  })

  afterEach(() => {
    queryClient.clear()
    resetWorkspaceSelectionStore()
    resetMediaWorkspaceIdResolver()
  })

  it('fails closed when the localStorage write throws', async () => {
    const board = createBoard('Quota Test')
    replaceBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
    const repository = createBoardSessionRepository()
    const setItemSpy = vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded')
    })

    try {
      // Without the retired preview cache there is nothing left to shed on
      // quota failure, so the save rejects before any remote call.
      await expect(repository.save(board.id, makeSnapshot('saved'))).rejects.toThrow(
        'SET_BOARD_STATE_FAILED',
      )
    } finally {
      setItemSpy.mockRestore()
    }

    const savedState = getBoardState(board.id)
    expect(savedState?.promptNodes[0]?.text).toBe('initial')
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('keeps a collaborator rename on ordinary save instead of PATCHing the stale local name', async () => {
    const board = createBoard('Old Local Name')
    replaceBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
    const repository = createBoardSessionRepository()
    findOrCreateV1BoardDto(board.id).name = 'Collaborator New Name'

    await repository.save(board.id, makeSnapshot('saved'))

    expect(patchCallsFor(board.id)).toEqual([])
    expect(findOrCreateV1BoardDto(board.id).name).toBe('Collaborator New Name')
    expect(getBoardById(board.id)?.name).toBe('Old Local Name')
    const documentBody = readPutDocumentBody(board.id)
    expect(documentBody?.payload?.promptNodes[0]?.text).toBe('saved')
  })

  it('keeps the server name on remote sync failure without PATCHing the stale local name', async () => {
    const board = createBoard('Old Local Name')
    replaceBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
    const repository = createBoardSessionRepository()
    findOrCreateV1BoardDto(board.id).name = 'Collaborator New Name'
    mocks.request.mockRejectedValueOnce({ status: 409, code: 'board_revision_conflict' })

    let thrown: unknown
    try {
      await repository.save(board.id, makeSnapshot('saved-after-conflict'))
    } catch (error) {
      thrown = error
    }

    expect(isBoardSessionRemoteSyncError(thrown)).toBe(true)
    expect(patchCallsFor(board.id)).toEqual([])
    expect(findOrCreateV1BoardDto(board.id).name).toBe('Collaborator New Name')
    expect(getBoardState(board.id)?.promptNodes[0]?.text).toBe('saved-after-conflict')
  })

  it('keeps the local state and reports recoverable remote sync failure', async () => {
    const nowSpy = vi.spyOn(Date, 'now').mockReturnValue(1_000)
    try {
      const board = createBoard('Remote Failure')
      replaceBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
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

      const savedState = getBoardState(board.id)
      expect(savedState?.promptNodes[0]?.text).toBe('saved-after-remote-failure')
      expect(savedState).not.toHaveProperty('viewport')
      expect(savedState?.updatedAt).toBe(2_000)
      expect(getBoardById(board.id)?.updatedAt).toBe(2_000)
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

  it('rejects a previous-schema local state instead of migrating it', async () => {
    const board = createBoard('Retired Local Snapshot')
    localStorage.setItem(
      `ideaboard.canvas.boardStates.v2.public_00000000-0000-0000-0000-000000000001.${WORKSPACE_ID}`,
      JSON.stringify({
        [board.id]: {
          schemaVersion: BOARD_STATE_SCHEMA_VERSION - 1,
          imageNodes: [],
          promptNodes: [
            {
              id: 1,
              type: 'prompt',
              x: 0,
              y: 0,
              width: 280,
              text: 'migrated local',
            },
          ],
          fusionNodes: [],
          connections: [],
          groups: [],
          updatedAt: 2_000,
        },
      }),
    )
    const repository = createBoardSessionRepository()

    await expect(repository.load(board.id)).rejects.toBeInstanceOf(InvalidBoardStateError)
  })

  it('keeps saving local board drafts when stored auth is temporarily missing', async () => {
    const board = createBoard('Session Recovery')
    replaceBoardStateMap({ [board.id]: makeState(1_000, 'initial') })
    clearStoredAuth()
    const repository = createBoardSessionRepository()

    const savedState = await repository.saveLocal(
      board.id,
      makeSnapshot('saved-without-stored-auth'),
    )

    expect(repository.exists(board.id)).toBe(true)
    expect(savedState.promptNodes[0]?.text).toBe('saved-without-stored-auth')
    expect(getBoardState(board.id)?.promptNodes[0]?.text).toBe('saved-without-stored-auth')
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('loads a session snapshot without local metadata', async () => {
    const board = createBoard('Load Snapshot')
    replaceBoardStateMap({
      [board.id]: makeBoardState({
        ...makeSnapshot('loaded'),
        updatedAt: 2_000,
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1' }],
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

  it('rejects pre-migration local state instead of normalizing across schema versions', async () => {
    const board = createBoard('Older Local Snapshot')
    localStorage.setItem(
      `ideaboard.canvas.boardStates.v2.public_00000000-0000-0000-0000-000000000001.${WORKSPACE_ID}`,
      JSON.stringify({
        [board.id]: {
          schemaVersion: BOARD_STATE_SCHEMA_VERSION - 2,
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
          groups: [],
          updatedAt: 2_000,
        },
      }),
    )
    const repository = createBoardSessionRepository()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    try {
      await expect(repository.load(board.id)).rejects.toMatchObject({
        code: 'INVALID_BOARD_STATE',
        boundary: 'local-state-read',
        isRecoverable: true,
      })
      expect(warnSpy).toHaveBeenCalledWith(
        '[canvas] invalid board state rejected',
        board.id,
        expect.objectContaining({
          code: 'INVALID_BOARD_STATE',
          boundary: 'local-state-read',
          isRecoverable: true,
        }),
      )
    } finally {
      warnSpy.mockRestore()
    }
  })

  it('rejects invalid local state with recoverable validation context instead of returning an empty session snapshot', async () => {
    const board = createBoard('Invalid Local Snapshot')
    localStorage.setItem(
      `ideaboard.canvas.boardStates.v2.public_00000000-0000-0000-0000-000000000001.${WORKSPACE_ID}`,
      JSON.stringify({
        [board.id]: {
          ...makeBoardState(),
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
    )
    const repository = createBoardSessionRepository()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    try {
      await expect(repository.load(board.id)).rejects.toMatchObject({
        code: 'INVALID_BOARD_STATE',
        boundary: 'local-state-read',
        isRecoverable: true,
        issues: [
          expect.objectContaining({
            path: 'imageNodes.0.sourceImageRatio',
          }),
        ],
      })
    } finally {
      warnSpy.mockRestore()
    }
  })

  it('rejects invalid remote hydration state with recoverable validation context', async () => {
    const board = createBoard('Invalid Remote Snapshot')
    v1Boards.push({
      id: board.id,
      workspace_id: WORKSPACE_ID,
      name: board.name,
      revision: 1,
      schema_version: BOARD_STATE_SCHEMA_VERSION,
      archived_at: null,
      created_at: new Date(board.createdAt).toISOString(),
      updated_at: new Date(board.updatedAt).toISOString(),
    })
    v1Documents[board.id] = {
      board_id: board.id,
      revision: 1,
      schema_version: BOARD_STATE_SCHEMA_VERSION,
      document: {
        ...makeBoardState(),
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
      updated_at: new Date(board.updatedAt).toISOString(),
    }
    const repository = createBoardSessionRepository()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    try {
      await expect(repository.load(board.id)).rejects.toMatchObject({
        code: 'INVALID_BOARD_STATE',
        boundary: 'server-hydration',
        isRecoverable: true,
        issues: [
          expect.objectContaining({
            path: 'imageNodes.0.sourceImageRatio',
          }),
        ],
      })
    } finally {
      warnSpy.mockRestore()
    }
  })

  it('rejects invalid save snapshots with recoverable validation context', async () => {
    const board = createBoard('Invalid Save')
    const repository = createBoardSessionRepository()
    const invalidSnapshot = {
      ...makeBoardSnapshot(),
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
        } as unknown as BoardSnapshot['imageNodes'][number],
      ],
    } as BoardSnapshot

    await expect(repository.save(board.id, invalidSnapshot)).rejects.toMatchObject({
      code: 'INVALID_BOARD_STATE',
      boundary: 'board-save',
      isRecoverable: true,
    })
  })
})

function readPutDocumentBody(boardId: string): {
  schema_version?: number
  payload?: BoardState
} {
  const call = mocks.request.mock.calls.find(
    ([url, config]) =>
      url === `/v1/workspaces/${WORKSPACE_ID}/boards/${boardId}/document` &&
      typeof config === 'object' &&
      config !== null &&
      'method' in config &&
      config.method === 'PUT',
  )
  if (!call) throw new Error(`Missing document PUT call for ${boardId}`)
  const body = call[1]?.body as unknown
  if (typeof body === 'string') {
    const parsed = JSON.parse(body) as {
      data?: { schema_version?: number; payload?: BoardState }
    }
    return parsed.data ?? {}
  }
  if (body && typeof body === 'object') {
    return ((body as { data?: { schema_version?: number; payload?: BoardState } }).data ?? {}) as {
      schema_version?: number
      payload?: BoardState
    }
  }
  throw new Error(`Missing document PUT body for ${boardId}`)
}

function patchCallsFor(boardId: string) {
  return mocks.request.mock.calls.filter(
    ([url, config]) =>
      url === `/v1/workspaces/${WORKSPACE_ID}/boards/${boardId}` &&
      typeof config === 'object' &&
      config !== null &&
      'method' in config &&
      config.method === 'PATCH',
  )
}
