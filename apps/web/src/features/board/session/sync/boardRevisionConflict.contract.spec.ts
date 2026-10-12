// P2.5 decision (5) — board revision coexistence.
// If-Match reads the observed revision; 409 keeps the draft + surfaces the
// conflict status while explicit reload converges; archived stays on the
// banner path (never conflict); rename is PATCH-only and advances the
// revision. All halves already exist in the current tree, so this spec stays
// active (real queryFn counting via the api/client mock transport).

import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createMockWorkspaceBoardApiRequest,
  createMockWorkspaceBoardApiState,
  type MockWorkspaceBoardApiState,
} from '../../testing'
import {
  addRemoteBoardFixture,
  makePromptBoardState,
  primeAuthenticatedWorkspace,
} from '../../testing/syncFixtures'
import { clearStoredAuth } from '../../../../core/utils/auth'

import { getBoardState, replaceBoardStateMap } from '../persistence/state'
import { listBoards, replaceBoards } from '../catalog/boards'
import { boardDocumentOptions } from './boardQueries'
import { boardQueryKeys } from './boardQueryKeys'
import { queryClient } from '../../../../core/query/queryClient'
import {
  getBoardSyncConflictKind,
  hydrateBoardsFromServer,
  isBoardArchivedError,
  isBoardSyncConflictError,
  syncBoardRenameToServer,
  syncBoardToServer,
} from './boardSync'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  return { request }
})

vi.mock('../../../../api/client', () => ({
  api: { request: mocks.request },
}))

let apiState: MockWorkspaceBoardApiState

function readIfMatch(boardId: string): unknown {
  const call = putCallsFor(boardId)[0]
  if (!call) throw new Error(`Missing PUT call for ${boardId}/document`)
  return (call[1] as { headers?: Record<string, unknown> }).headers?.['If-Match']
}

function putCallsFor(boardId: string) {
  return mocks.request.mock.calls.filter(
    ([url, config]) =>
      url === `/v1/workspaces/${WORKSPACE_ID}/boards/${boardId}/document` &&
      typeof config === 'object' &&
      config !== null &&
      'method' in config &&
      config.method === 'PUT',
  )
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

describe('board revision conflict', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace(WORKSPACE_ID)
    vi.clearAllMocks()
    queryClient.clear()
    apiState = createMockWorkspaceBoardApiState()
    mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(apiState))
  })

  it('sends the observed revision as If-Match on document saves', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Revision Board', createdAt: 100, updatedAt: 3_000, revision: 7 },
    ])
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Revision Board',
      createdAt: 100,
      updatedAt: 3_000,
      revision: 7,
      state: makePromptBoardState(3_000, 'revision-state'),
    })

    await syncBoardToServer('board-1')

    expect(readIfMatch('board-1')).toBe('7')
    expect(listBoards()[0]?.revision).toBe(8)
  })

  it('keeps the draft and the conflict status on 409, then converges on explicit reload', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Local Draft', createdAt: 100, updatedAt: 2_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-1': makePromptBoardState(2_000, 'local-draft') })
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Local Draft',
      createdAt: 100,
      updatedAt: 2_000,
      revision: 7,
      state: makePromptBoardState(2_000, 'local-draft'),
    })
    apiState.saveError = {
      status: 409,
      code: 'board_revision_conflict',
      msg: 'revision conflict',
    }

    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({ status: 409 })
    expect(getBoardSyncConflictKind(apiState.saveError)).toBe('revision-conflict')
    expect(isBoardSyncConflictError(apiState.saveError)).toBe(true)
    // The draft and its base revision survive the conflict.
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('local-draft')
    expect(listBoards()[0]?.revision).toBe(7)

    delete apiState.saveError
    apiState.boardsResponse.data = []
    apiState.documentById = {}
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Remote Winner',
      createdAt: 100,
      updatedAt: 1_000,
      revision: 8,
      state: makePromptBoardState(1_000, 'remote-winner'),
    })
    await hydrateBoardsFromServer({ preserveLocalNewer: false, rollbackBoardIds: ['board-1'] })

    expect(listBoards()[0]).toMatchObject({ revision: 8, name: 'Remote Winner' })
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('remote-winner')
  })

  it('keeps archived writes on the banner path, never the conflict path', async () => {
    replaceBoards([{ id: 'board-1', name: 'Board', createdAt: 100, updatedAt: 2_000 }])
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Board',
      createdAt: 100,
      updatedAt: 2_000,
      state: makePromptBoardState(2_000, 'board-state'),
    })
    apiState.saveError = { status: 409, code: 'board_archived', msg: 'archived' }

    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({ code: 'board_archived' })
    expect(isBoardArchivedError(apiState.saveError)).toBe(true)
    expect(isBoardSyncConflictError(apiState.saveError)).toBe(false)
    expect(getBoardSyncConflictKind(apiState.saveError)).toBeNull()
  })

  it.each([
    { label: 'never-synced offline creation', revision: undefined },
    { label: 'previously synced deleted or inaccessible board', revision: 7 },
  ])('preserves the $label draft when the document PUT returns 404', async ({ revision }) => {
    replaceBoards([
      {
        id: 'board-1',
        name: 'Local Draft',
        createdAt: 100,
        updatedAt: 2_000,
        ...(revision === undefined ? {} : { revision }),
      },
    ])
    replaceBoardStateMap({ 'board-1': makePromptBoardState(2_000, 'unsaved-draft') })
    apiState.saveError = { status: 404, code: 'board_not_found', msg: 'board unavailable' }

    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({
      status: 404,
      code: 'board_not_found',
    })

    expect(putCallsFor('board-1')).toHaveLength(1)
    expect(readIfMatch('board-1')).toBe(String(revision ?? 1))
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('unsaved-draft')
    expect(listBoards()[0]?.revision).toBe(revision)
    expect(isBoardSyncConflictError(apiState.saveError)).toBe(false)
    expect(isBoardArchivedError(apiState.saveError)).toBe(false)
    expect(apiState.createdBoards).toEqual([])
    expect(apiState.documentById).toEqual({})
  })

  it('preserves drafts on document GET 404 until a complete list confirms remote absence', async () => {
    replaceBoards([
      { id: 'offline-new', name: 'Offline New', createdAt: 100, updatedAt: 2_000 },
      { id: 'board-1', name: 'Local Draft', createdAt: 100, updatedAt: 2_000, revision: 7 },
    ])
    replaceBoardStateMap({
      'offline-new': makePromptBoardState(2_000, 'offline-draft'),
      'board-1': makePromptBoardState(2_000, 'synced-draft'),
    })
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Remote Board',
      createdAt: 100,
      updatedAt: 1_000,
      revision: 7,
      state: makePromptBoardState(1_000, 'remote-state'),
    })
    // The list still contains the board, but access disappears before GET document.
    // This transport error is distinct from absence in a completed catalog list.
    delete apiState.documentById['board-1']

    await expect(hydrateBoardsFromServer()).resolves.toMatchObject({
      ok: false,
      error: { status: 404, code: 'board_not_found' },
    })
    expect(mocks.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/boards/board-1/document`,
      expect.objectContaining({ method: 'GET' }),
    )
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('synced-draft')
    expect(getBoardState('offline-new')?.promptNodes[0]?.text).toBe('offline-draft')
    expect(listBoards().find((board) => board.id === 'board-1')?.revision).toBe(7)

    apiState.boardsResponse.data = []
    await expect(hydrateBoardsFromServer()).resolves.toMatchObject({ ok: true })

    expect(listBoards().map((board) => board.id)).toEqual(['offline-new'])
    expect(getBoardState('offline-new')?.promptNodes[0]?.text).toBe('offline-draft')
    expect(getBoardState('board-1')).toBeNull()
    expect(putCallsFor('board-1')).toEqual([])
    expect(apiState.createdBoards).toEqual([])
  })

  describe('production query revision integration', () => {
    it('seeds the Query snapshot on hydrate, saves with the true local base, and converges on explicit reload after 409', async () => {
      // Local-newer draft built on base 7; the server already moved to 8.
      replaceBoards([
        { id: 'board-1', name: 'Local Draft', createdAt: 100, updatedAt: 2_000, revision: 7 },
      ])
      replaceBoardStateMap({ 'board-1': makePromptBoardState(2_000, 'local-draft') })
      addRemoteBoardFixture(apiState, WORKSPACE_ID, {
        id: 'board-1',
        name: 'Remote Newer Revision',
        createdAt: 100,
        updatedAt: 1_000,
        revision: 8,
        state: makePromptBoardState(1_000, 'remote-v8'),
      })

      // Preserve-local hydrate keeps the draft and its true base in the
      // store while Query takes ownership of the newer server snapshot.
      await hydrateBoardsFromServer()
      expect(listBoards()[0]).toMatchObject({ revision: 7, name: 'Local Draft' })
      expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('local-draft')
      expect(
        queryClient.getQueryData(boardQueryKeys.document(WORKSPACE_ID, 'board-1')),
      ).toMatchObject({
        revision: 8,
        document: { payload: { promptNodes: [{ text: 'remote-v8' }] } },
      })
      expect(queryClient.getQueryData(boardQueryKeys.list(WORKSPACE_ID))).toEqual([
        expect.objectContaining({ id: 'board-1', revision: 8 }),
      ])

      // The save sends the true local base (7), not the newer Query
      // snapshot (8): sending 8 would silently overwrite the unseen remote.
      apiState.saveError = {
        status: 409,
        code: 'board_revision_conflict',
        msg: 'revision conflict',
      }
      await expect(syncBoardToServer('board-1')).rejects.toMatchObject({ status: 409 })
      expect(readIfMatch('board-1')).toBe('7')
      expect(getBoardSyncConflictKind(apiState.saveError)).toBe('revision-conflict')
      // The draft and its base revision survive the conflict.
      expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('local-draft')
      expect(listBoards()[0]?.revision).toBe(7)

      delete apiState.saveError
      apiState.boardsResponse.data = []
      apiState.documentById = {}
      addRemoteBoardFixture(apiState, WORKSPACE_ID, {
        id: 'board-1',
        name: 'Remote Winner',
        createdAt: 100,
        updatedAt: 1_000,
        revision: 8,
        state: makePromptBoardState(1_000, 'remote-winner'),
      })
      // Explicit reload converges through the Query document first, then
      // the store rollback adopts the remote winner.
      await queryClient.invalidateQueries({
        queryKey: boardQueryKeys.document(WORKSPACE_ID, 'board-1'),
        exact: true,
      })
      const reloaded = await queryClient.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-1'))
      expect(reloaded).toMatchObject({
        revision: 8,
        document: { payload: { promptNodes: [{ text: 'remote-winner' }] } },
      })
      await hydrateBoardsFromServer({ preserveLocalNewer: false, rollbackBoardIds: ['board-1'] })

      expect(listBoards()[0]).toMatchObject({ revision: 8, name: 'Remote Winner' })
      expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('remote-winner')
    })
  })

  it('renames PATCH-only without a document write and advances the revision', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Old Name', createdAt: 100, updatedAt: 3_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-1': makePromptBoardState(3_000, 'untouched-state') })
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Old Name',
      createdAt: 100,
      updatedAt: 3_000,
      revision: 7,
      state: makePromptBoardState(3_000, 'untouched-state'),
    })
    // The server advances the revision on rename; the mock echoes the stored
    // row, so bump it here to prove the client adopts the new base.
    const remote = apiState.boardsResponse.data.find((item) => item.id === 'board-1')
    if (remote) remote.revision = 8

    await syncBoardRenameToServer('board-1', 'Explicit New Name')

    expect(patchCallsFor('board-1')).toHaveLength(1)
    expect(putCallsFor('board-1')).toEqual([])
    expect(remote?.name).toBe('Explicit New Name')
    expect(listBoards()[0]?.revision).toBe(8)
  })
})
