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
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../workspace'
import {
  BoardPayloadError,
  BoardWorkspaceUnavailableError,
} from '../../infrastructure/api/boardsResource'
import { getBoardState, replaceBoardStateMap } from '../../infrastructure/persistence'
import { listBoards, replaceBoards, subscribeBoardChanges } from '../catalog/boards'
import {
  hydrateBoardsFromServer,
  getBoardSyncConflictKind,
  isBoardArchivedError,
  isBoardSyncConflictError,
  syncBoardRenameToServer,
  syncBoardToServer,
  syncDeleteBoardToServer,
  syncRestoreBoardToServer,
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
  return { id, name, createdAt: 100, updatedAt, workspaceId: WORKSPACE_ID }
}

function getPromptText(boardId: string): string | undefined {
  return getBoardState(boardId)?.promptNodes[0]?.text
}

function boardDto(params: {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  revision?: number | undefined
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

function documentData(params: { id: string; revision?: number | undefined; state: BoardState }) {
  // Backend wire shape: the stored snapshot nests nodes under `payload`
  // beside `schema_version` (see `saveWorkspaceBoard` + `V1InitialDocument`).
  const { schemaVersion, ...payload } = params.state
  return {
    board_id: params.id,
    revision: params.revision ?? 1,
    schema_version: schemaVersion,
    document: { schema_version: schemaVersion, payload },
    updated_at: new Date(params.state.updatedAt).toISOString(),
  }
}

function addRemoteBoard(params: {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  revision?: number | undefined
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
    avatar: '',
    role: 'user',
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

  it('drops a local-only board with revision deleted elsewhere (meta and draft cleared)', async () => {
    replaceBoards([
      { id: 'deleted-elsewhere', name: 'Deleted Elsewhere', createdAt: 100, updatedAt: 3_000, revision: 5 },
    ])
    replaceBoardStateMap({ 'deleted-elsewhere': makeState(3_000, 'stale-local-edit') })
    addRemoteBoard({
      id: 'remote-only',
      name: 'Remote Board',
      createdAt: 100,
      updatedAt: 1_500,
      state: makeState(1_500, 'remote-only-state'),
    })

    await hydrateBoardsFromServer()

    expect(listBoards().map((board) => board.id)).toEqual(['remote-only'])
    // The draft rides only on the merged set, so dropping the meta clears it too.
    expect(getBoardState('deleted-elsewhere')).toBeNull()
    // Hydrate never pushes the dropped board back to the server.
    expect(
      mocks.request.mock.calls.filter(
        ([url, config]) =>
          typeof url === 'string' &&
          url.includes('deleted-elsewhere') &&
          typeof config === 'object' &&
          config !== null &&
          'method' in config &&
          config.method === 'PUT',
      ),
    ).toEqual([])
  })

  it('keeps a never-synced local-only board without revision during hydrate', async () => {
    replaceBoards([{ id: 'offline-new', name: 'Offline New', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'offline-new': makeState(2_000, 'offline-draft') })
    addRemoteBoard({
      id: 'remote-only',
      name: 'Remote Board',
      createdAt: 100,
      updatedAt: 1_500,
      state: makeState(1_500, 'remote-only-state'),
    })

    await hydrateBoardsFromServer()

    expect(listBoards().map((board) => board.id)).toEqual(['offline-new', 'remote-only'])
    expect(getPromptText('offline-new')).toBe('offline-draft')
  })

  it('still drops a never-synced local-only board marked for rollback', async () => {
    replaceBoards([{ id: 'local-only', name: 'Local Only', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'local-only': makeState(2_000, 'local-draft') })
    addRemoteBoard({
      id: 'remote-only',
      name: 'Remote Board',
      createdAt: 100,
      updatedAt: 1_500,
      state: makeState(1_500, 'remote-only-state'),
    })

    await hydrateBoardsFromServer({ preserveLocalNewer: false, rollbackBoardIds: ['local-only'] })

    expect(listBoards().map((board) => board.id)).toEqual(['remote-only'])
    expect(getBoardState('local-only')).toBeNull()
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
    if (result.ok) throw new Error('expected hydrate failure')
    expect(result.error).toBeInstanceOf(BoardWorkspaceUnavailableError)
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('waits for a late workspace selection when explicitly requested', async () => {
    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().clear()
    addRemoteBoard({
      id: 'late-board',
      name: 'Late Board',
      createdAt: 100,
      updatedAt: 2_000,
      state: makeState(2_000, 'late-ready'),
    })

    const hydrateTask = hydrateBoardsFromServer({ waitForWorkspaceTimeoutMs: 500 })
    setTimeout(() => getWorkspaceSelectionStore().select(WORKSPACE_ID), 10)
    const result = await hydrateTask

    expect(result.ok).toBe(true)
    expect(listBoards().map((board) => board.id)).toEqual(['late-board'])
    expect(getPromptText('late-board')).toBe('late-ready')
  })

  it('times out to fail-closed when the workspace selection never arrives', async () => {
    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().clear()

    const result = await hydrateBoardsFromServer({ waitForWorkspaceTimeoutMs: 20 })

    expect(result.ok).toBe(false)
    if (result.ok) throw new Error('expected hydrate failure')
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
      avatar: '',
      role: 'user',
    })
    replaceBoards([boardMeta('board-b', 'User B Board', 3_000)])
    resolveRequest(apiState.boardsResponse)

    await expect(hydrateTask).resolves.toMatchObject({ ok: false })
    expect(listBoards()).toEqual([boardMeta('board-b', 'User B Board', 3_000)])
    expect(getBoardState('board-a')).toBeNull()
  })

  it('drops a delayed hydrate response after switching workspaces', async () => {
    const workspaceB = 'ws_workspaceB'
    replaceBoards([
      { id: 'board-a', name: 'Workspace A Board', createdAt: 100, updatedAt: 2_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-a': makeState(2_000, 'workspace-a-draft') })
    let resolveRequest!: (value: unknown) => void
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )
    addRemoteBoard({
      id: 'board-a',
      name: 'Late Workspace A Board',
      createdAt: 100,
      updatedAt: 1_000,
      revision: 8,
      state: makeState(1_000, 'late-workspace-a'),
    })
    const hydrateTask = hydrateBoardsFromServer()

    getWorkspaceSelectionStore().select(workspaceB)
    resolveRequest(apiState.boardsResponse)

    await expect(hydrateTask).resolves.toMatchObject({ ok: false })
    expect(listBoards()).toEqual([])
    expect(getBoardState('board-a')).toBeNull()

    getWorkspaceSelectionStore().select(WORKSPACE_ID)

    expect(listBoards()[0]).toMatchObject({ revision: 7, name: 'Workspace A Board' })
    expect(getPromptText('board-a')).toBe('workspace-a-draft')
  })

  it('ignores a delayed save response after switching workspaces', async () => {
    const workspaceB = 'ws_workspaceB'
    replaceBoards([
      { id: 'board-a', name: 'Workspace A Board', createdAt: 100, updatedAt: 2_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-a': makeState(2_000, 'workspace-a-draft') })
    addRemoteBoard({
      id: 'board-a',
      name: 'Workspace A Board',
      createdAt: 100,
      updatedAt: 2_000,
      revision: 7,
      state: makeState(2_000, 'workspace-a-draft'),
    })
    let resolvePut!: (value: unknown) => void
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePut = resolve
      }),
    )
    const saveTask = syncBoardToServer('board-a')

    getWorkspaceSelectionStore().select(workspaceB)
    resolvePut({
      data: {
        board_id: 'board-a',
        revision: 8,
        schema_version: BOARD_STATE_SCHEMA_VERSION,
        document: { schema_version: BOARD_STATE_SCHEMA_VERSION, payload: {} },
        updated_at: new Date(2_001).toISOString(),
      },
    })
    await saveTask

    expect(listBoards()).toEqual([])
    getWorkspaceSelectionStore().select(WORKSPACE_ID)
    expect(listBoards()[0]).toMatchObject({ revision: 7, name: 'Workspace A Board' })
    expect(getPromptText('board-a')).toBe('workspace-a-draft')
  })

  it('sends document saves to the newly selected workspace', async () => {
    const workspaceB = 'ws_workspaceB'
    getWorkspaceSelectionStore().select(workspaceB)
    replaceBoards([{ id: 'board-b', name: 'Workspace B Board', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({ 'board-b': makeState(3_000, 'workspace-b-draft') })
    addRemoteBoard({
      id: 'board-b',
      name: 'Workspace B Board',
      createdAt: 100,
      updatedAt: 3_000,
      revision: 3,
      state: makeState(3_000, 'workspace-b-draft'),
    })

    await syncBoardToServer('board-b')

    const putCall = mocks.request.mock.calls.find(
      ([url, config]) =>
        url === `/v1/workspaces/${workspaceB}/boards/board-b/document` &&
        typeof config === 'object' &&
        config !== null &&
        'method' in config &&
        config.method === 'PUT',
    )
    expect(putCall).toBeTruthy()
    expect(listBoards()[0]).toMatchObject({ revision: 4 })
  })

  it('sends explicit renames to the newly selected workspace', async () => {
    const workspaceB = 'ws_workspaceB'
    getWorkspaceSelectionStore().select(workspaceB)
    replaceBoards([
      { id: 'board-b', name: 'Old Name', createdAt: 100, updatedAt: 3_000, revision: 3 },
    ])
    addRemoteBoard({
      id: 'board-b',
      name: 'Old Name',
      createdAt: 100,
      updatedAt: 3_000,
      revision: 3,
      state: makeState(3_000, 'workspace-b-draft'),
    })

    await syncBoardRenameToServer('board-b', 'New Name')

    const patchCall = mocks.request.mock.calls.find(
      ([url, config]) =>
        url === `/v1/workspaces/${workspaceB}/boards/board-b` &&
        typeof config === 'object' &&
        config !== null &&
        'method' in config &&
        config.method === 'PATCH',
    )
    expect(patchCall).toBeTruthy()
    expect(listBoards()[0]).toMatchObject({ revision: 3 })
  })

  it('returns hydrate failure for a rejected list read', async () => {
    apiState.listError = { status: 503, code: 'service_unavailable', msg: '服务暂不可用' }

    const result = await hydrateBoardsFromServer()

    expect(result).toMatchObject({ ok: false, error: expect.anything() })
  })

  it('rejects a remote board with a broken timestamp instead of fabricating now()', async () => {
    replaceBoards([{ id: 'board-1', name: 'Local Board', createdAt: 100, updatedAt: 2_000 }])
    apiState.boardsResponse.data.push({
      ...boardDto({ id: 'board-broken', name: 'Broken Remote', createdAt: 100, updatedAt: 1_500 }),
      updated_at: 'not-a-timestamp',
    })

    const result = await hydrateBoardsFromServer()

    expect(result).toMatchObject({ ok: false, error: expect.any(BoardPayloadError) })
    expect(listBoards().map((board) => board.id)).toEqual(['board-1'])
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

  it('saves the document without PATCHing the board name', async () => {
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

    expect(patchCallsFor('board-1')).toEqual([])
    const payload = readReplaceDocumentPayload('board-1')
    expect(payload.data).toEqual(
      expect.objectContaining({
        schema_version: BOARD_STATE_SCHEMA_VERSION,
      }),
    )
    expect((payload.data?.payload as Record<string, unknown>).updatedAt).toBe(1_000)
  })

  it('keeps a collaborator rename on the server when the local name is stale', async () => {
    replaceBoards([{ id: 'board-1', name: 'Old Local Name', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({ 'board-1': makeState(3_000, 'local-edits') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Collaborator New Name',
      createdAt: 100,
      updatedAt: 2_500,
      state: makeState(2_500, 'remote-state'),
    })

    await syncBoardToServer('board-1')

    expect(patchCallsFor('board-1')).toEqual([])
    const remote = apiState.boardsResponse.data.find((item) => item.id === 'board-1')
    expect(remote?.name).toBe('Collaborator New Name')
    expect(listBoards()[0]?.name).toBe('Old Local Name')
  })

  it('leaves the server name untouched when a document save hits a revision conflict', async () => {
    replaceBoards([{ id: 'board-1', name: 'Old Local Name', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({ 'board-1': makeState(3_000, 'local-edits') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Collaborator New Name',
      createdAt: 100,
      updatedAt: 2_500,
      state: makeState(2_500, 'remote-state'),
    })
    apiState.saveError = {
      status: 409,
      code: 'board_revision_conflict',
      msg: '看板版本已变更，请刷新后重试',
    }

    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({
      code: 'board_revision_conflict',
    })

    expect(patchCallsFor('board-1')).toEqual([])
    const remote = apiState.boardsResponse.data.find((item) => item.id === 'board-1')
    expect(remote?.name).toBe('Collaborator New Name')
  })

  it('sends no PATCH when a document save fails on the network', async () => {
    replaceBoards([{ id: 'board-1', name: 'Old Local Name', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({ 'board-1': makeState(3_000, 'local-edits') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Collaborator New Name',
      createdAt: 100,
      updatedAt: 2_500,
      state: makeState(2_500, 'remote-state'),
    })
    apiState.saveError = new Error('network down')

    await expect(syncBoardToServer('board-1')).rejects.toThrow('network down')

    expect(patchCallsFor('board-1')).toEqual([])
    expect(apiState.savedCalls.filter((call) => call.method === 'PUT')).toHaveLength(1)
    const remote = apiState.boardsResponse.data.find((item) => item.id === 'board-1')
    expect(remote?.name).toBe('Collaborator New Name')
  })

  it('renames via PATCH without writing the document', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Old Name', createdAt: 100, updatedAt: 3_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-1': makeState(3_000, 'untouched-state') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Old Name',
      createdAt: 100,
      updatedAt: 3_000,
      revision: 7,
      state: makeState(3_000, 'untouched-state'),
    })

    await syncBoardRenameToServer('board-1', 'Explicit New Name')

    expect(readRenamePayload('board-1')).toEqual({ data: { name: 'Explicit New Name' } })
    expect(apiState.savedCalls.filter((call) => call.method === 'PUT')).toEqual([])
    const remote = apiState.boardsResponse.data.find((item) => item.id === 'board-1')
    expect(remote?.name).toBe('Explicit New Name')
    expect(listBoards()[0]?.revision).toBe(7)
  })

  it('surfaces rename PATCH failures without a document write', async () => {
    replaceBoards([{ id: 'board-1', name: 'Old Name', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({ 'board-1': makeState(3_000, 'untouched-state') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Old Name',
      createdAt: 100,
      updatedAt: 3_000,
      state: makeState(3_000, 'untouched-state'),
    })
    const renameError = { status: 409, code: 'board_archived', msg: '看板已归档' }
    mocks.request.mockRejectedValueOnce(renameError)

    await expect(syncBoardRenameToServer('board-1', 'Explicit New Name')).rejects.toMatchObject({
      code: 'board_archived',
    })

    expect(apiState.savedCalls.filter((call) => call.method === 'PUT')).toEqual([])
    expect(isBoardArchivedError(renameError)).toBe(true)
    expect(isBoardSyncConflictError(renameError)).toBe(false)
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

  it('keeps the true local base revision on preserve-local hydrate so the next save conflicts', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Local Draft', createdAt: 100, updatedAt: 2_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-1': makeState(2_000, 'local-draft') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Remote Newer Revision',
      createdAt: 100,
      updatedAt: 1_000,
      revision: 8,
      state: makeState(1_000, 'remote-v8'),
    })
    apiState.saveError = {
      status: 409,
      code: 'board_revision_conflict',
      msg: '看板版本已变更，请刷新后重试',
    }

    await hydrateBoardsFromServer()

    // The newer local timestamp keeps the draft, but must not upgrade its
    // concurrency base to the unseen server revision.
    expect(listBoards()[0]).toMatchObject({ revision: 7, name: 'Local Draft' })
    expect(getPromptText('board-1')).toBe('local-draft')

    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({ status: 409 })
    expect(readIfMatch('board-1')).toBe('7')
    // Conflict recovery keeps the local draft and its base revision.
    expect(getPromptText('board-1')).toBe('local-draft')
    expect(listBoards()[0]?.revision).toBe(7)
  })

  it('advances the base revision after accepting the remote snapshot and saves cleanly', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Local Draft', createdAt: 100, updatedAt: 2_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-1': makeState(2_000, 'local-draft') })
    addRemoteBoard({
      id: 'board-1',
      name: 'Remote Newer Revision',
      createdAt: 100,
      updatedAt: 1_000,
      revision: 8,
      state: makeState(1_000, 'remote-v8'),
    })

    await hydrateBoardsFromServer({ preserveLocalNewer: false, rollbackBoardIds: ['board-1'] })

    expect(listBoards()[0]).toMatchObject({ revision: 8, name: 'Remote Newer Revision' })
    expect(getPromptText('board-1')).toBe('remote-v8')

    await syncBoardToServer('board-1')

    expect(readIfMatch('board-1')).toBe('8')
    expect(listBoards()[0]?.revision).toBe(9)
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

  it('classifies 409 board_archived as archived, never as a name conflict', async () => {
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
      code: 'board_archived',
      msg: '看板已归档',
    }

    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({
      code: 'board_archived',
    })
    expect(isBoardArchivedError(apiState.saveError)).toBe(true)
    expect(isBoardSyncConflictError(apiState.saveError)).toBe(false)
  })

  it('restores an archived board on the v1 restore path', async () => {
    addRemoteBoard({
      id: 'board-1',
      name: 'Board',
      createdAt: 100,
      updatedAt: 2_000,
      state: makeState(2_000, 'restored'),
    })
    apiState.archivedBoardIds.push('board-1')

    const meta = await syncRestoreBoardToServer('board-1')

    const call = mocks.request.mock.calls.find(
      ([url, config]) =>
        url === `/v1/workspaces/${WORKSPACE_ID}/boards/board-1/restore` &&
        typeof config === 'object' &&
        config !== null &&
        'method' in config &&
        config.method === 'POST',
    )
    expect(call).toBeTruthy()
    expect(apiState.archivedBoardIds).toEqual([])
    expect(meta.archivedAt).toBeUndefined()
  })
})

describe('board sync conflict classifier', () => {
  it('distinguishes revision conflicts from ordinary sync failures', () => {
    const error = { status: 409, code: 'board_revision_conflict', msg: '看板版本已变更' }

    expect(getBoardSyncConflictKind(error)).toBe('revision-conflict')
    expect(isBoardSyncConflictError(error)).toBe(true)
    expect(isBoardArchivedError(error)).toBe(false)
  })

  it('distinguishes idempotency conflicts from ordinary sync failures', () => {
    const error = { status: 409, code: 'board_idempotency_conflict', msg: '幂等冲突' }

    expect(getBoardSyncConflictKind(error)).toBe('idempotency-conflict')
    expect(isBoardSyncConflictError(error)).toBe(true)
    expect(isBoardArchivedError(error)).toBe(false)
  })

  it('resolves canonical codes through wrapped causes', () => {
    const revisionCause = { status: 409, code: 'board_revision_conflict' }
    const idempotencyCause = { status: 409, code: 'board_idempotency_conflict' }

    expect(getBoardSyncConflictKind(new Error('save failed', { cause: revisionCause }))).toBe(
      'revision-conflict',
    )
    expect(getBoardSyncConflictKind(new Error('create failed', { cause: idempotencyCause }))).toBe(
      'idempotency-conflict',
    )
    expect(isBoardSyncConflictError(new Error('wrapped', { cause: revisionCause }))).toBe(true)
  })

  it('falls back to revision-conflict for a code-less 409', () => {
    expect(getBoardSyncConflictKind({ status: 409 })).toBe('revision-conflict')
    expect(isBoardSyncConflictError({ status: 409 })).toBe(true)
    expect(isBoardSyncConflictError(new Error('transport 409', { cause: { status: 409 } }))).toBe(
      true,
    )
  })

  it('never classifies archived writes or ordinary failures as conflicts', () => {
    expect(getBoardSyncConflictKind({ status: 409, code: 'board_archived' })).toBeNull()
    expect(isBoardSyncConflictError({ status: 409, code: 'board_archived' })).toBe(false)
    expect(getBoardSyncConflictKind(new Error('network down'))).toBeNull()
    expect(isBoardSyncConflictError(new Error('network down'))).toBe(false)
    expect(getBoardSyncConflictKind({ status: 500, code: 'service_unavailable' })).toBeNull()
    expect(isBoardSyncConflictError({ status: 500 })).toBe(false)
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
