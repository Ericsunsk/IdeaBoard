import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { BoardState } from '../../domain'
import { BOARD_STATE_SCHEMA_VERSION } from '../../domain'
import {
  createMockWorkspaceBoardApiRequest,
  createMockWorkspaceBoardApiState,
  makeBoardState,
  type MockWorkspaceBoardApiState,
} from '../../testing'
import { clearStoredAuth, setStoredAuth } from '../../../../shared/utils/auth'
import {
  getWorkspaceSelectionStore,
  resetWorkspaceSelectionStore,
} from '../../../workspace'
import { BoardWorkspaceUnavailableError } from '../../infrastructure/api/boardsResource'
import { getBoardState, replaceBoardStateMap } from '../../infrastructure/persistence'
import { listBoards, replaceBoards, subscribeBoardChanges } from '../catalog/boards'
import {
  hydrateBoardsFromServer,
  syncBoardToServer,
  syncDeleteBoardToServer,
} from './boardSync'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  return { request }
})

vi.mock('../../../../shared/api/client', () => ({
  api: {
    request: mocks.request,
  },
}))

let apiState: MockWorkspaceBoardApiState

function makeState(updatedAt: number, promptText: string): BoardState {
  return makeBoardState({
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
  return getBoardState(boardId)?.promptNodes[0]?.text
}

function boardDto(params: {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  revision?: number
}): Record<string, unknown> {
  return {
    id: params.id,
    workspace_id: WORKSPACE_ID,
    name: params.name,
    revision: params.revision ?? 1,
    schema_version: BOARD_STATE_SCHEMA_VERSION,
    archived_at: null,
    created_at: new Date(params.createdAt).toISOString(),
    updated_at: new Date(params.updatedAt).toISOString(),
  }
}

function documentData(params: { id: string; revision?: number; state: BoardState }) {
  const { schemaVersion, ...payload } = params.state
  return {
    board_id: params.id,
    revision: params.revision ?? 1,
    schema_version: schemaVersion,
    document: payload,
    updated_at: new Date(params.state.updatedAt).toISOString(),
  }
}

function addRemoteBoard(params: {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  revision?: number
  state: BoardState
}) {
  apiState.boardsResponse.data.push(
    boardDto({
      id: params.id,
      name: params.name,
      createdAt: params.createdAt,
      updatedAt: params.updatedAt,
      revision: params.revision,
    }),
  )
  apiState.documentById[params.id] = documentData({
    id: params.id,
    revision: params.revision,
    state: params.state,
  })
}

function primeAuthenticatedWorkspace() {
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
  resetWorkspaceSelectionStore()
  getWorkspaceSelectionStore().select(WORKSPACE_ID)
}

describe('boardSync', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace()
    vi.clearAllMocks()
    apiState = createMockWorkspaceBoardApiState()
    mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(apiState))
  })

  it('preserves newer local board meta and state during hydrate', async () => {
    replaceBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'board-1': makeState(2_000, 'local-newer') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Remote Old',
      createdAt: 100,
      updatedAt: 1_000,
      state: makeState(1_000, 'remote-older'),
    })

    await hydrateBoardsFromServer()

    expect(listBoards()[0]?.name).toBe('Local Rename')
    expect(getPromptText('board-1')).toBe('local-newer')
  })

  it('publishes remote hydrate only after metadata and board state are both committed', async () => {
    addRemoteBoard({
      id: 'remote-board',
      name: 'Remote Board',
      createdAt: 100,
      updatedAt: 2_000,
      state: makeState(2_000, 'remote-ready'),
    })
    const observedPromptTexts: Array<string | undefined> = []
    const unsubscribe = subscribeBoardChanges(() => {
      observedPromptTexts.push(getPromptText('remote-board'))
    })

    await hydrateBoardsFromServer()
    unsubscribe()

    expect(observedPromptTexts).toEqual(['remote-ready'])
  })

  it('preserves local-only boards while adding remote boards during hydrate', async () => {
    replaceBoards([{ id: 'local-only', name: 'Unsynced Local', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'local-only': makeState(2_000, 'local-only-state') })
    addRemoteBoard({
      id: 'remote-only',
      name: 'Remote Board',
      createdAt: 100,
      updatedAt: 1_500,
      state: makeState(1_500, 'remote-only-state'),
    })

    await hydrateBoardsFromServer()

    expect(listBoards().map((board) => board.id)).toEqual(['local-only', 'remote-only'])
    expect(getPromptText('local-only')).toBe('local-only-state')
    expect(getPromptText('remote-only')).toBe('remote-only-state')
  })

  it('rolls back only the conflict target while preserving other local newer and local-only boards', async () => {
    replaceBoards([
      { id: 'board-a', name: 'Conflicting Local', createdAt: 100, updatedAt: 4_000 },
      { id: 'board-b', name: 'Local Newer', createdAt: 100, updatedAt: 5_000 },
      { id: 'board-c', name: 'Local Only', createdAt: 100, updatedAt: 6_000 },
    ])
    replaceBoardStateMap({
      'board-a': makeState(4_000, 'local-conflict'),
      'board-b': makeState(5_000, 'local-newer'),
      'board-c': makeState(6_000, 'local-only'),
    })
    apiState.saveError = { status: 409, code: 'board_revision_conflict', msg: '看板版本已变更' }
    addRemoteBoard({
      id: 'board-a',
      name: 'Conflicting Local',
      createdAt: 100,
      updatedAt: 4_000,
      state: makeState(4_000, 'local-conflict'),
    })

    await expect(syncBoardToServer('board-a')).rejects.toMatchObject({ status: 409 })
    delete apiState.saveError

    apiState.boardsResponse.data = []
    apiState.documentById = {}
    addRemoteBoard({
      id: 'board-a',
      name: 'Remote Winner',
      createdAt: 100,
      updatedAt: 1_000,
      state: makeState(1_000, 'remote-winner'),
    })
    addRemoteBoard({
      id: 'board-b',
      name: 'Remote Older',
      createdAt: 100,
      updatedAt: 1_000,
      state: makeState(1_000, 'remote-older'),
    })

    await hydrateBoardsFromServer({ preserveLocalNewer: false })

    const boardNames = new Map(listBoards().map((board) => [board.id, board.name]))
    expect(boardNames.get('board-a')).toBe('Remote Winner')
    expect(boardNames.get('board-b')).toBe('Local Newer')
    expect(boardNames.get('board-c')).toBe('Local Only')
    expect(getPromptText('board-a')).toBe('remote-winner')
    expect(getPromptText('board-b')).toBe('local-newer')
    expect(getPromptText('board-c')).toBe('local-only')
  })

  it('can replace local newer boards when explicitly requested for conflict recovery', async () => {
    replaceBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'board-1': makeState(2_000, 'local-newer') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Remote Winner',
      createdAt: 100,
      updatedAt: 1_000,
      state: makeState(1_000, 'remote-winner'),
    })

    await hydrateBoardsFromServer({ preserveLocalNewer: false })

    expect(listBoards()[0]?.name).toBe('Remote Winner')
    expect(getPromptText('board-1')).toBe('remote-winner')
  })

  it('returns hydrate failure without clearing local boards', async () => {
    replaceBoards([{ id: 'board-1', name: 'Local Board', createdAt: 100, updatedAt: 2_000 }])
    apiState.listError = new Error('network failed')

    const result = await hydrateBoardsFromServer()

    expect(result.ok).toBe(false)
    expect(listBoards()[0]?.name).toBe('Local Board')
  })

  it('fails hydrate closed without touching the network when no workspace is selected', async () => {
    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().clear()

    const result = await hydrateBoardsFromServer()

    expect(result.ok).toBe(false)
    expect(result.error).toBeInstanceOf(BoardWorkspaceUnavailableError)
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('rejects a board save before any network call when no workspace is selected', async () => {
    replaceBoards([{ id: 'board-1', name: 'Board', createdAt: 100, updatedAt: 2_000 }])
    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().clear()

    await expect(syncBoardToServer('board-1')).rejects.toBeInstanceOf(
      BoardWorkspaceUnavailableError,
    )
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('rejects an in-flight hydrate response after the auth scope changes', async () => {
    let resolveRequest!: (value: unknown) => void
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )
    addRemoteBoard({
      id: 'board-a',
      name: 'Late User A Board',
      createdAt: 100,
      updatedAt: 2_000,
      state: makeState(2_000, 'late-user-a'),
    })
    const hydrateTask = hydrateBoardsFromServer()

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
    replaceBoards([boardMeta('board-b', 'User B Board', 3_000)])
    resolveRequest(apiState.boardsResponse)

    await expect(hydrateTask).resolves.toMatchObject({ ok: false })
    expect(listBoards()).toEqual([boardMeta('board-b', 'User B Board', 3_000)])
    expect(getBoardState('board-a')).toBeNull()
  })

  it('returns hydrate failure for a rejected list read', async () => {
    apiState.listError = { status: 503, code: 'service_unavailable', msg: '服务暂不可用' }

    const result = await hydrateBoardsFromServer()

    expect(result).toMatchObject({ ok: false, error: expect.anything() })
  })

  it('rejects a board save with the v1 revision conflict', async () => {
    replaceBoards([{ id: 'board-1', name: 'Board', createdAt: 100, updatedAt: 2_000 }])
    addRemoteBoard({
      id: 'board-1',
      name: 'Board',
      createdAt: 100,
      updatedAt: 2_000,
      state: makeState(2_000, 'board-state'),
    })
    apiState.saveError = {
      status: 409,
      code: 'board_revision_conflict',
      msg: '看板版本已变更，请刷新后重试',
    }

    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({
      code: 'board_revision_conflict',
    })
  })

  it('returns hydrate failure for pre-migration remote board state without upgrading it', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      addRemoteBoard({
        id: 'board-older',
        name: 'Older Remote',
        createdAt: 100,
        updatedAt: 1_500,
        state: makeState(1_500, 'older remote prompt'),
      })
      apiState.documentById['board-older'] = {
        board_id: 'board-older',
        revision: 1,
        schema_version: BOARD_STATE_SCHEMA_VERSION - 2,
        document: {
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
        },
        updated_at: new Date(1_500).toISOString(),
      }

      const result = await hydrateBoardsFromServer()

      expect(result).toMatchObject({
        ok: false,
        error: expect.objectContaining({
          code: 'INVALID_BOARD_STATE',
          boundary: 'server-hydration',
          recoverable: true,
        }),
      })
      expect(getBoardState('board-older')).toBeNull()
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
    replaceBoards([{ id: 'local-board', name: 'Local Board', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'local-board': makeState(2_000, 'local-state') })
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      addRemoteBoard({
        id: 'board-invalid',
        name: 'Invalid Remote',
        createdAt: 100,
        updatedAt: 1_500,
        state: makeState(1_500, 'invalid remote'),
      })
      const invalid = makeState(1_500, 'invalid remote') as unknown as Record<string, unknown>
      apiState.documentById['board-invalid'] = {
        board_id: 'board-invalid',
        revision: 1,
        schema_version: BOARD_STATE_SCHEMA_VERSION,
        document: {
          ...(invalid as object),
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
        updated_at: new Date(1_500).toISOString(),
      }

      const result = await hydrateBoardsFromServer()

      expect(result).toMatchObject({
        ok: false,
        error: expect.objectContaining({
          code: 'INVALID_BOARD_STATE',
          boundary: 'server-hydration',
          recoverable: true,
        }),
      })
      expect(getBoardState('board-invalid')).toBeNull()
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

  it('renames via PATCH while the document keeps the state timestamp', async () => {
    replaceBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({ 'board-1': makeState(1_000, 'remote-older') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Board',
      createdAt: 100,
      updatedAt: 1_000,
      state: makeState(1_000, 'remote-older'),
    })

    await syncBoardToServer('board-1')

    expect(readRenamePayload('board-1')).toEqual({ data: { name: 'Local Rename' } })
    const payload = readReplaceDocumentPayload('board-1')
    expect(payload.data).toEqual(
      expect.objectContaining({
        schema_version: BOARD_STATE_SCHEMA_VERSION,
      }),
    )
    expect((payload.data?.payload as Record<string, unknown>).updatedAt).toBe(1_000)
  })

  it('sends the observed revision as If-Match and stores the server revision', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Revision Board', createdAt: 100, updatedAt: 3_000, revision: 7 },
    ])
    addRemoteBoard({
      id: 'board-1',
      name: 'Revision Board',
      createdAt: 100,
      updatedAt: 3_000,
      revision: 7,
      state: makeState(3_000, 'revision-state'),
    })

    await syncBoardToServer('board-1')

    expect(readIfMatch('board-1')).toBe('7')
    expect(listBoards()[0]?.revision).toBe(8)
  })

  it('syncs the shared remote envelope shape without viewport', async () => {
    replaceBoards([{ id: 'board-1', name: 'Envelope Board', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({
      'board-1': makeBoardState({
        updatedAt: 2_000,
        mediaRefs: [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }],
        previewSnapshotDataUrl: 'data:image/webp;base64,preview',
        previewSnapshotUpdatedAt: 2_001,
        previewSnapshotVersion: 4,
      }),
    })
    addRemoteBoard({
      id: 'board-1',
      name: 'Envelope Board',
      createdAt: 100,
      updatedAt: 3_000,
      state: makeState(3_000, 'envelope'),
    })

    await syncBoardToServer('board-1')

    const payload = readReplaceDocumentPayload('board-1')
    expect(payload.data?.payload).toEqual(
      expect.objectContaining({
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
    expect(payload.data?.payload).not.toHaveProperty('viewport')
  })

  it('archives the board on the v1 delete path', async () => {
    addRemoteBoard({
      id: 'board-1',
      name: 'Board',
      createdAt: 100,
      updatedAt: 2_000,
      state: makeState(2_000, 'archived'),
    })

    await syncDeleteBoardToServer('board-1')

    const call = mocks.request.mock.calls.find(
      ([url, config]) =>
        url === `/v1/workspaces/${WORKSPACE_ID}/boards/board-1/archive` &&
        typeof config === 'object' &&
        config !== null &&
        'method' in config &&
        config.method === 'POST',
    )
    expect(call).toBeTruthy()
    expect(apiState.archivedBoardIds).toEqual(['board-1'])
  })
})

function findCall(boardId: string, method: string, suffix: string) {
  const call = mocks.request.mock.calls.find(
    ([url, config]) =>
      url === `/v1/workspaces/${WORKSPACE_ID}/boards/${boardId}${suffix}` &&
      typeof config === 'object' &&
      config !== null &&
      'method' in config &&
      config.method === method,
  )
  if (!call) throw new Error(`Missing ${method} call for ${boardId}${suffix}`)
  return call[1] as { body?: unknown; headers?: Record<string, unknown> }
}

function readReplaceDocumentPayload(boardId: string): { data?: Record<string, unknown> } {
  const config = findCall(boardId, 'PUT', '/document')
  const body = config.body
  if (typeof body === 'string') return JSON.parse(body) as { data?: Record<string, unknown> }
  if (body && typeof body === 'object') return body as { data?: Record<string, unknown> }
  throw new Error(`Missing replace body for ${boardId}`)
}

function readRenamePayload(boardId: string): Record<string, unknown> {
  const config = findCall(boardId, 'PATCH', '')
  const body = config.body
  if (typeof body === 'string') return JSON.parse(body) as Record<string, unknown>
  if (body && typeof body === 'object') return body as Record<string, unknown>
  throw new Error(`Missing rename body for ${boardId}`)
}

function readIfMatch(boardId: string): unknown {
  return findCall(boardId, 'PUT', '/document').headers?.['If-Match']
}
