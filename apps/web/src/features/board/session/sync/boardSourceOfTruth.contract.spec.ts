// P3 production binding: Query owns the catalog list and per-board server
// document snapshots; the feature store owns the draft overlay.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryObserver } from '@tanstack/vue-query'
import { queryClient } from '../../../../core/query/queryClient'
import { makeBoardState } from '../../testing'
import { clearStoredAuth } from '../../../../core/utils/auth'
import {
  addRemoteBoardFixture,
  makePromptBoardState,
  primeAuthenticatedWorkspace,
} from '../../testing/syncFixtures'
import {
  createMockWorkspaceBoardApiRequest,
  createMockWorkspaceBoardApiState,
} from '../../testing/factories'
import { boardDocumentOptions, boardListOptions } from './boardQueries'
import { boardQueryKeys } from './boardQueryKeys'

// Isolated client carrying the production singleton defaults (stale 5s,
// gc 10m, retry 1, no refocus): the behavior under test (keys, options,
// invalidation) is production code; only the cache instance is per-test.
function newIsolatedBoardQueryClient() {
  return new QueryClient({ defaultOptions: queryClient.getDefaultOptions() })
}
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../workspace'
import { BoardWorkspaceUnavailableError } from '../adapters/boardsResource'
import { requireBoardWorkspaceId } from '../adapters/boardsResource'
import { getBoardState, replaceBoardStateMap } from '../persistence'
import { getBoardById, listBoards, replaceBoards } from '../catalog/boards'
import { getBoardScope } from '../catalog/scope'

const WORKSPACE_ID = 'ws_test123'
const OTHER_WORKSPACE_ID = 'ws_other999'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  return { request }
})

vi.mock('../../../../api/client', () => ({
  api: { request: mocks.request },
}))

describe('board source of truth', () => {
  const clients: ReturnType<typeof newIsolatedBoardQueryClient>[] = []
  const observers: Array<() => void> = []
  function client() {
    const result = newIsolatedBoardQueryClient()
    clients.push(result)
    return result
  }
  function remoteFixture(workspaceId = WORKSPACE_ID, revision = 8, text = 'server') {
    const state = createMockWorkspaceBoardApiState()
    addRemoteBoardFixture(state, workspaceId, {
      id: 'board-a',
      name: 'Server Board',
      createdAt: 100,
      updatedAt: 1000,
      revision,
      state: makePromptBoardState(1000, text),
    })
    return state
  }
  afterEach(() => {
    observers.splice(0).forEach((stop) => stop())
    clients.splice(0).forEach((entry) => entry.clear())
  })
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace(WORKSPACE_ID)
    vi.clearAllMocks()
    mocks.request.mockResolvedValue({ data: [], meta: { next_cursor: null, has_more: false } })
  })

  it('selection port owns the scope: resolution fails closed before any network call', () => {
    expect(getBoardScope()).toEqual({
      userScope: expect.any(String),
      workspaceId: WORKSPACE_ID,
    })
    expect(requireBoardWorkspaceId()).toBe(WORKSPACE_ID)

    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().clear()
    expect(() => requireBoardWorkspaceId()).toThrow(BoardWorkspaceUnavailableError)
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('feature store owns the draft overlay: local drafts survive without server writes', () => {
    replaceBoards([{ id: 'draft-1', name: 'Draft', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'draft-1': makeBoardState({ updatedAt: 2_000 }) })

    expect(getBoardById('draft-1')?.name).toBe('Draft')
    expect(getBoardState('draft-1')).not.toBeNull()
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('feature store isolates drafts per workspace scope', () => {
    replaceBoards([{ id: 'board-a', name: 'A Board', createdAt: 100, updatedAt: 2_000 }])
    replaceBoardStateMap({ 'board-a': makeBoardState({ updatedAt: 2_000 }) })

    getWorkspaceSelectionStore().select(OTHER_WORKSPACE_ID)

    expect(listBoards()).toEqual([])
    expect(getBoardState('board-a')).toBeNull()

    getWorkspaceSelectionStore().select(WORKSPACE_ID)
    expect(listBoards().map((board) => board.id)).toEqual(['board-a'])
  })

  describe('query server snapshot ownership', () => {
    it('caches the catalog list under the flat key ["boards", workspaceId, "list"]', async () => {
      const query = client()
      mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(remoteFixture()))
      const list = await query.fetchQuery(boardListOptions(WORKSPACE_ID))

      expect(
        query
          .getQueryCache()
          .getAll()
          .map((entry) => entry.queryKey),
      ).toEqual([['boards', WORKSPACE_ID, 'list']])
      expect(list).toEqual([
        expect.objectContaining({ id: 'board-a', name: 'Server Board', revision: 8 }),
      ])
      expect(query.getQueryData(boardQueryKeys.list(WORKSPACE_ID))).toEqual(list)
      expect(mocks.request).toHaveBeenCalledWith(
        expect.stringContaining(`/workspaces/${WORKSPACE_ID}/boards`),
        expect.objectContaining({ method: 'GET', signal: expect.any(AbortSignal) }),
      )
    })

    it('caches each board document under ["boards", workspaceId, "board", boardId, "document"]', async () => {
      const query = client()
      mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(remoteFixture()))
      const document = await query.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-a'))

      expect(
        query
          .getQueryCache()
          .getAll()
          .map((entry) => entry.queryKey),
      ).toEqual([['boards', WORKSPACE_ID, 'board', 'board-a', 'document']])
      expect(document).toMatchObject({ boardId: 'board-a', revision: 8 })
      expect(mocks.request).toHaveBeenCalledWith(
        `/v1/workspaces/${WORKSPACE_ID}/boards/board-a/document`,
        expect.objectContaining({ method: 'GET', signal: expect.any(AbortSignal) }),
      )
    })

    it('keeps local draft edits separate from the cached server document', async () => {
      const query = client()
      mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(remoteFixture()))
      const document = await query.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-a'))
      replaceBoardStateMap({ 'board-a': makePromptBoardState(2000, 'local draft') })

      expect(getBoardState('board-a')?.promptNodes[0]?.text).toBe('local draft')
      expect(query.getQueryData(boardQueryKeys.document(WORKSPACE_ID, 'board-a'))).toEqual(document)
      expect(document.document).toMatchObject({ payload: { promptNodes: [{ text: 'server' }] } })
      expect(mocks.request).toHaveBeenCalledTimes(1)
    })

    it('never regresses the cached document revision on an older server read', async () => {
      const query = client()
      const options = boardDocumentOptions(WORKSPACE_ID, 'board-a')
      mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(remoteFixture()))
      await query.fetchQuery(options)
      mocks.request.mockImplementation(
        createMockWorkspaceBoardApiRequest(remoteFixture(WORKSPACE_ID, 7, 'older')),
      )
      await query.invalidateQueries({ queryKey: options.queryKey })
      await query.fetchQuery(options)

      expect(mocks.request).toHaveBeenCalledTimes(2)
      expect(query.getQueryData(options.queryKey)).toMatchObject({
        revision: 8,
        document: { payload: { promptNodes: [{ text: 'server' }] } },
      })
    })

    it('cancels the old workspace read and ignores its late response after observer navigation', async () => {
      const query = client()
      const oldFixture = remoteFixture()
      const newFixture = remoteFixture(OTHER_WORKSPACE_ID, 9, 'other workspace')
      let resolveOld!: (value: unknown) => void
      let oldSignal: AbortSignal | undefined
      const oldResponse = new Promise<unknown>((resolve) => {
        resolveOld = resolve
      })
      mocks.request.mockImplementation(
        (url: string, config: { method?: string; signal?: AbortSignal }) => {
          if (url.includes(`/workspaces/${WORKSPACE_ID}/`)) {
            oldSignal = config.signal
            return oldResponse
          }
          return createMockWorkspaceBoardApiRequest(newFixture)(url, config)
        },
      )
      const observer = new QueryObserver(query, boardDocumentOptions(WORKSPACE_ID, 'board-a'))
      observers.push(observer.subscribe(() => undefined))
      await vi.waitFor(() => expect(oldSignal).toBeInstanceOf(AbortSignal))

      getWorkspaceSelectionStore().select(OTHER_WORKSPACE_ID)
      observer.setOptions(boardDocumentOptions(OTHER_WORKSPACE_ID, 'board-a'))
      await vi.waitFor(() => expect(observer.getCurrentResult().data?.revision).toBe(9))
      expect(oldSignal?.aborted).toBe(true)
      resolveOld({ data: oldFixture.documentById['board-a'] })
      await oldResponse
      await new Promise((resolve) => setTimeout(resolve, 0))

      expect(query.getQueryData(boardQueryKeys.document(WORKSPACE_ID, 'board-a'))).toBeUndefined()
      expect(observer.getCurrentResult().data).toMatchObject({
        revision: 9,
        document: { payload: { promptNodes: [{ text: 'other workspace' }] } },
      })
      expect(mocks.request).toHaveBeenCalledTimes(2)
    })
  })
})
