import { beforeEach, describe, expect, it } from 'vitest'
import { makeBoardState } from '../../testing'
import { makeSessionUser } from '../../../../test/factories'
import {
  clearStoredAuth,
  forgetStoredAuthScope,
  getAuthStorageUserScope,
  setStoredAuth,
} from '../../../../core/utils/auth'
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../workspace'
import { listBoards, replaceBoards } from './boards'
import { createBoardRepository } from './repository'
import { getBoardState, replaceBoardStateMap } from '../persistence/state'
import {
  boardScopeKey,
  getBoardScope,
  getRecoverableBoardScope,
  subscribeBoardScopeChange,
  type BoardScope,
} from './scope'

const WORKSPACE_A = 'ws_test123'
const WORKSPACE_B = 'ws_workspaceB'
const BOARDS_PREFIX = 'ideaboard.canvas.boards.v2'
const STATES_PREFIX = 'ideaboard.canvas.boardStates.v2'

function primeAuthenticatedWorkspace(workspaceId: string) {
  setStoredAuth({
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'tester',
    avatar: '',
    role: 'user',
  })
  resetWorkspaceSelectionStore()
  getWorkspaceSelectionStore().select(workspaceId)
}

function userScope(): string {
  const scope = getAuthStorageUserScope()
  if (!scope) throw new Error('expected an authenticated scope')
  return scope
}

describe('boardScope', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace(WORKSPACE_A)
  })

  it('isolates catalog and state caches per workspace', () => {
    replaceBoards([{ id: 'board-a', name: 'A Board', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({
      'board-a': makeBoardState({ updatedAt: 2_000 }),
    })

    getWorkspaceSelectionStore().select(WORKSPACE_B)

    expect(listBoards()).toEqual([])
    expect(getBoardState('board-a')).toBeNull()

    replaceBoards([{ id: 'board-b', name: 'B Board', createdAt: 100, updatedAt: 3_000 }])
    replaceBoardStateMap({ 'board-b': makeBoardState({ updatedAt: 3_000 }) })

    getWorkspaceSelectionStore().select(WORKSPACE_A)

    expect(listBoards().map((board) => board.id)).toEqual(['board-a'])
    expect(listBoards()[0]).toMatchObject({ workspaceId: WORKSPACE_A })
    expect(getBoardState('board-a')).not.toBeNull()

    getWorkspaceSelectionStore().select(WORKSPACE_B)

    expect(listBoards().map((board) => board.id)).toEqual(['board-b'])
    expect(getBoardState('board-b')).not.toBeNull()
    expect(getBoardState('board-a')).toBeNull()
  })

  it('keeps legacy user-only caches out of workspace scopes without deleting them', () => {
    const scope = userScope()
    const legacyBoardsKey = `${BOARDS_PREFIX}.${scope}`
    const legacyStatesKey = `${STATES_PREFIX}.${scope}`
    window.localStorage.setItem(
      legacyBoardsKey,
      JSON.stringify([{ id: 'legacy-board', name: 'Legacy', createdAt: 100, updatedAt: 200 }]),
    )
    window.localStorage.setItem(
      legacyStatesKey,
      JSON.stringify({ 'legacy-board': makeBoardState({ updatedAt: 200 }) }),
    )

    expect(listBoards()).toEqual([])
    expect(getBoardState('legacy-board')).toBeNull()
    replaceBoards([{ id: 'board-a', name: 'A Board', createdAt: 100, updatedAt: 2_000 }])

    expect(window.localStorage.getItem(legacyBoardsKey)).not.toBeNull()
    expect(window.localStorage.getItem(legacyStatesKey)).not.toBeNull()
  })

  it('fails closed after logout instead of reading the previous user scope', () => {
    replaceBoards([{ id: 'board-a', name: 'A Board', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({
      'board-a': makeBoardState({ updatedAt: 2_000 }),
    })

    // Explicit logout forgets the remembered device scope (unlike
    // transient auth loss, which keeps it for draft recovery).
    clearStoredAuth()
    forgetStoredAuthScope()

    expect(getBoardScope()).toMatchObject({ userScope: null, workspaceId: WORKSPACE_A })
    expect(listBoards()).toEqual([])
    expect(getBoardState('board-a')).toBeNull()
  })

  it('keeps recovery drafts isolated when a different user signs in after auth expires', () => {
    const originalScope = userScope()
    replaceBoards([{ id: 'board-a', name: 'A Board', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'board-a': makeBoardState({ updatedAt: 2_000 }) })

    clearStoredAuth()
    expect(getBoardScope().userScope).toBeNull()
    expect(getRecoverableBoardScope().userScope).toBe(originalScope)
    expect(getBoardState('board-a')).not.toBeNull()

    setStoredAuth(makeSessionUser({ public_id: '00000000-0000-0000-0000-000000000002' }))
    expect(getRecoverableBoardScope().userScope).not.toBe(originalScope)
    expect(listBoards()).toEqual([])
    expect(getBoardState('board-a')).toBeNull()

    primeAuthenticatedWorkspace(WORKSPACE_A)
    expect(listBoards().map((board) => board.id)).toEqual(['board-a'])
    expect(getBoardState('board-a')).not.toBeNull()
  })

  it('notifies scope subscribers on workspace switches and auth changes', () => {
    const observed: BoardScope[] = []
    const unsubscribe = subscribeBoardScopeChange((scope) => observed.push({ ...scope }))
    try {
      getWorkspaceSelectionStore().select(WORKSPACE_B)
      expect(getBoardScope()).toMatchObject({ userScope: userScope(), workspaceId: WORKSPACE_B })
      expect(observed.at(-1)).toMatchObject({ userScope: userScope(), workspaceId: WORKSPACE_B })

      clearStoredAuth()
      expect(getBoardScope()).toMatchObject({ userScope: null, workspaceId: WORKSPACE_B })
      expect(observed.at(-1)).toMatchObject({ userScope: null, workspaceId: WORKSPACE_B })
    } finally {
      unsubscribe()
    }
  })

  it('binds the repository view model to the User + Workspace scope', () => {
    const repository = createBoardRepository()
    try {
      replaceBoards([{ id: 'board-a', name: 'A Board', createdAt: 100, updatedAt: 2_000 }])
      expect(repository.viewModel.boards.value.map((board) => board.id)).toEqual(['board-a'])
      expect(repository.viewModel.scope.value).toBe(
        boardScopeKey({ userScope: userScope(), workspaceId: WORKSPACE_A }),
      )

      getWorkspaceSelectionStore().select(WORKSPACE_B)

      expect(repository.viewModel.boards.value).toEqual([])
      expect(repository.viewModel.scope.value).toBe(
        boardScopeKey({ userScope: userScope(), workspaceId: WORKSPACE_B }),
      )
    } finally {
      repository.dispose()
    }
  })
})
