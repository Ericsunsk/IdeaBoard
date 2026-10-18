import { beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient } from '@tanstack/vue-query'
import { queryClient } from '../../../../core/query/queryClient'
import { clearStoredAuth } from '../../../../core/utils/auth'
import { primeAuthenticatedWorkspace, selectWorkspace } from '../../testing'
import { getBoardById, replaceBoards } from '../catalog/boards'
import { boardQueryKeys } from '../sync/boardQueryKeys'
import { isBoardAccessLost } from './accessLoss'
import { revalidateAndConvergeBoardAccess, subscribeBoardAccessLost } from './convergeAccessLoss'

const WORKSPACE = 'ws_converge1'
const OTHER_WORKSPACE = 'ws_converge2'
const BOARD = 'board_converge1'

function newIsolatedBoardQueryClient() {
  return new QueryClient({ defaultOptions: queryClient.getDefaultOptions() })
}

function seedCatalog() {
  replaceBoards([
    {
      id: BOARD,
      name: 'Converge',
      createdAt: 1,
      updatedAt: 2,
      revision: 3,
    },
  ])
}

describe('confirmed access loss convergence', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    replaceBoards([])
  })

  test('purges protected queries and drops local copies without copying content', async () => {
    primeAuthenticatedWorkspace(WORKSPACE)
    seedCatalog()
    const client = newIsolatedBoardQueryClient()
    client.setQueryData(boardQueryKeys.document(WORKSPACE, BOARD), { revision: 3 })
    client.setQueryData(boardQueryKeys.access(WORKSPACE, BOARD), { accessVersion: 5 })
    client.setQueryData(boardQueryKeys.list(WORKSPACE), [{ id: BOARD }])
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const notified: string[] = []
    const stop = subscribeBoardAccessLost((boardId) => notified.push(boardId))
    try {
      await revalidateAndConvergeBoardAccess(BOARD, {
        workspaceId: WORKSPACE,
        client,
        revalidate: async () => 'revoked',
      })

      expect(isBoardAccessLost(BOARD)).toBe(true)
      expect(client.getQueryData(boardQueryKeys.document(WORKSPACE, BOARD))).toBeUndefined()
      expect(client.getQueryData(boardQueryKeys.access(WORKSPACE, BOARD))).toBeUndefined()
      expect(invalidate).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: boardQueryKeys.list(WORKSPACE) }),
      )
      // Local copies are dropped, never copied elsewhere.
      expect(getBoardById(BOARD)).toBeNull()
      expect(notified).toEqual([BOARD])
    } finally {
      stop()
    }
  })

  test('faulty subscribers never break convergence', async () => {
    primeAuthenticatedWorkspace(WORKSPACE)
    const client = newIsolatedBoardQueryClient()
    const stopBad = subscribeBoardAccessLost(() => {
      throw new Error('surface blew up')
    })
    const notified: string[] = []
    const stop = subscribeBoardAccessLost((boardId) => notified.push(boardId))
    try {
      await expect(
        revalidateAndConvergeBoardAccess(BOARD, {
          workspaceId: WORKSPACE,
          client,
          revalidate: async () => 'revoked',
        }),
      ).resolves.toBe('revoked')
      expect(isBoardAccessLost(BOARD)).toBe(true)
      expect(notified).toEqual([BOARD])
    } finally {
      stopBad()
      stop()
    }
  })

  test('unsubscribing stops surface notifications', async () => {
    primeAuthenticatedWorkspace(WORKSPACE)
    const client = newIsolatedBoardQueryClient()
    const notified: string[] = []
    const stop = subscribeBoardAccessLost((boardId) => notified.push(boardId))
    stop()
    await revalidateAndConvergeBoardAccess(BOARD, {
      workspaceId: WORKSPACE,
      client,
      revalidate: async () => 'revoked',
    })
    expect(notified).toEqual([])
  })

  test('without a scope nothing converges', async () => {
    const client = newIsolatedBoardQueryClient()
    const notified: string[] = []
    const stop = subscribeBoardAccessLost((boardId) => notified.push(boardId))
    try {
      await revalidateAndConvergeBoardAccess(BOARD, { client, revalidate: async () => 'revoked' })
      expect(isBoardAccessLost(BOARD)).toBe(false)
      expect(notified).toEqual([])
    } finally {
      stop()
    }
  })

  test('marks stay isolated across workspace switches', async () => {
    primeAuthenticatedWorkspace(WORKSPACE)
    const client = newIsolatedBoardQueryClient()
    await revalidateAndConvergeBoardAccess(BOARD, {
      workspaceId: WORKSPACE,
      client,
      revalidate: async () => 'revoked',
    })
    expect(isBoardAccessLost(BOARD)).toBe(true)
    selectWorkspace(OTHER_WORKSPACE)
    expect(isBoardAccessLost(BOARD)).toBe(false)
  })
})

describe('access-loss revalidation and convergence', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    replaceBoards([])
    primeAuthenticatedWorkspace(WORKSPACE)
    seedCatalog()
  })

  test('only a confirmed metadata read purges the protected draft and notifies surfaces', async () => {
    const client = newIsolatedBoardQueryClient()
    client.setQueryData(boardQueryKeys.document(WORKSPACE, BOARD), { draft: 'unsaved' })
    const notified: string[] = []
    const stop = subscribeBoardAccessLost((boardId) => notified.push(boardId))
    try {
      const verdict = await revalidateAndConvergeBoardAccess(BOARD, {
        client,
        fetchMeta: async () => {
          throw Object.assign(new Error('board_forbidden'), { code: 'board_forbidden' })
        },
      })
      expect(verdict).toBe('revoked')
      expect(isBoardAccessLost(BOARD)).toBe(true)
      expect(client.getQueryData(boardQueryKeys.document(WORKSPACE, BOARD))).toBeUndefined()
      expect(getBoardById(BOARD)).toBeNull()
      expect(notified).toEqual([BOARD])
    } finally {
      stop()
    }
  })

  test.each([
    new Error('offline'),
    Object.assign(new Error('unavailable'), { status: 503 }),
    Object.assign(new Error('conflict'), { code: 'board_revision_conflict', status: 409 }),
  ])('unconfirmed metadata failures preserve the draft and retry state: %s', async (error) => {
    const client = newIsolatedBoardQueryClient()
    client.setQueryData(boardQueryKeys.document(WORKSPACE, BOARD), { draft: 'unsaved' })
    const verdict = await revalidateAndConvergeBoardAccess(BOARD, {
      client,
      fetchMeta: async () => {
        throw error
      },
    })
    expect(verdict).toBe('preserved')
    expect(isBoardAccessLost(BOARD)).toBe(false)
    expect(client.getQueryData(boardQueryKeys.document(WORKSPACE, BOARD))).toEqual({
      draft: 'unsaved',
    })
    expect(getBoardById(BOARD)).not.toBeNull()
  })

  test('A→B→A drops an old revoked verdict without purging the new residency', async () => {
    const client = newIsolatedBoardQueryClient()
    let reject!: (error: Error) => void
    const pending = revalidateAndConvergeBoardAccess(BOARD, {
      client,
      fetchMeta: () =>
        new Promise((_resolve, rejectRead) => {
          reject = rejectRead
        }),
    })
    selectWorkspace(OTHER_WORKSPACE)
    selectWorkspace(WORKSPACE)
    seedCatalog()
    client.setQueryData(boardQueryKeys.document(WORKSPACE, BOARD), { draft: 'new residency' })
    reject(Object.assign(new Error('denied'), { code: 'board_forbidden' }))
    expect(await pending).toBe('preserved')
    expect(isBoardAccessLost(BOARD)).toBe(false)
    expect(getBoardById(BOARD)).not.toBeNull()
    expect(client.getQueryData(boardQueryKeys.document(WORKSPACE, BOARD))).toEqual({
      draft: 'new residency',
    })
  })

  test('an observer that leaves its board does not purge a late response', async () => {
    const client = newIsolatedBoardQueryClient()
    let isCurrent = true
    let reject!: (error: Error) => void
    const pending = revalidateAndConvergeBoardAccess(BOARD, {
      client,
      isCurrent: () => isCurrent,
      fetchMeta: () =>
        new Promise((_resolve, rejectRead) => {
          reject = rejectRead
        }),
    })
    isCurrent = false
    reject(Object.assign(new Error('denied'), { code: 'board_forbidden' }))
    expect(await pending).toBe('preserved')
    expect(getBoardById(BOARD)).not.toBeNull()
  })

  test('an inactive observer starts no metadata read', async () => {
    const fetchMeta = vi.fn()
    expect(
      await revalidateAndConvergeBoardAccess(BOARD, {
        isCurrent: () => false,
        fetchMeta,
      }),
    ).toBe('preserved')
    expect(fetchMeta).not.toHaveBeenCalled()
  })
})
