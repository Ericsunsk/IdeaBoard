// Board sync contract (6) — board draft overlay.
// Local-newer wins; rollback → remote wins; never-synced offline creations
// are kept while deleted-elsewhere boards are dropped; the overlay is frozen
// to the captured scope. All halves already exist in the current tree
// (hydrate merge + scope guards), so this spec stays active.

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
import { getWorkspaceSelectionStore } from '../../../workspace'
import { getBoardState, replaceBoardStateMap } from '../persistence/state'
import { listBoards, replaceBoards } from '../catalog/boards'
import { hydrateBoardsFromServer } from './boardSync'

const WORKSPACE_ID = 'ws_test123'
const WORKSPACE_B = 'ws_workspaceB'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  return { request }
})

vi.mock('../../../../api/client', () => ({
  api: { request: mocks.request },
}))

let apiState: MockWorkspaceBoardApiState

describe('board draft overlay', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace(WORKSPACE_ID)
    vi.clearAllMocks()
    apiState = createMockWorkspaceBoardApiState()
    mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(apiState))
  })

  it('lets the local-newer draft win over an older remote snapshot', async () => {
    replaceBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'board-1': makePromptBoardState(2_000, 'local-newer') })
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Remote Old',
      createdAt: 100,
      updatedAt: 1_000,
      state: makePromptBoardState(1_000, 'remote-older'),
    })

    await hydrateBoardsFromServer()

    expect(listBoards()[0]?.name).toBe('Local Rename')
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('local-newer')
  })

  it('lets an explicit rollback converge to the remote snapshot', async () => {
    replaceBoards([{ id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'board-1': makePromptBoardState(2_000, 'local-newer') })
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Remote Winner',
      createdAt: 100,
      updatedAt: 1_000,
      state: makePromptBoardState(1_000, 'remote-winner'),
    })

    await hydrateBoardsFromServer({ preserveLocalNewer: false })

    expect(listBoards()[0]?.name).toBe('Remote Winner')
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('remote-winner')
  })

  it('keeps never-synced offline creations and drops deleted-elsewhere boards', async () => {
    replaceBoards([
      { id: 'offline-new', name: 'Offline New', createdAt: 100, updatedAt: 2_000 },
      {
        id: 'deleted-elsewhere',
        name: 'Deleted Elsewhere',
        createdAt: 100,
        updatedAt: 3_000,
        revision: 5,
      },
    ])
    replaceBoardStateMap({
      'offline-new': makePromptBoardState(2_000, 'offline-draft'),
      'deleted-elsewhere': makePromptBoardState(3_000, 'stale-local-edit'),
    })
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'remote-only',
      name: 'Remote Board',
      createdAt: 100,
      updatedAt: 1_500,
      state: makePromptBoardState(1_500, 'remote-only-state'),
    })

    await hydrateBoardsFromServer()

    expect(
      listBoards()
        .map((board) => board.id)
        .sort(),
    ).toEqual(['offline-new', 'remote-only'])
    expect(getBoardState('offline-new')?.promptNodes[0]?.text).toBe('offline-draft')
    expect(getBoardState('deleted-elsewhere')).toBeNull()
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

  it('freezes the overlay to the captured scope: a workspace switch drops the late response', async () => {
    replaceBoards([
      { id: 'board-a', name: 'Workspace A Board', createdAt: 100, updatedAt: 2_000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-a': makePromptBoardState(2_000, 'workspace-a-draft') })
    let resolveRequest!: (value: unknown) => void
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-a',
      name: 'Late Workspace A Board',
      createdAt: 100,
      updatedAt: 1_000,
      revision: 8,
      state: makePromptBoardState(1_000, 'late-workspace-a'),
    })
    const hydrateTask = hydrateBoardsFromServer()

    getWorkspaceSelectionStore().select(WORKSPACE_B)
    resolveRequest(apiState.boardsResponse)

    await expect(hydrateTask).resolves.toMatchObject({ ok: false })
    expect(listBoards()).toEqual([])
    expect(getBoardState('board-a')).toBeNull()
  })
})
