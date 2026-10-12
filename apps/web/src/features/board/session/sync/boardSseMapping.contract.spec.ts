// P3 production binding: SSE carries invalidation signals only; the
// production invalidator maps them onto the Board Query keys.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  isWorkspaceEventType,
  resetWorkspaceEventStreamsForTests,
  subscribeWorkspaceEvent,
  WORKSPACE_EVENT_TYPES,
  type WorkspaceEventEnvelope,
} from '../../../../api/workspaceEventStream'

import { QueryClient, QueryObserver } from '@tanstack/vue-query'
import { queryClient } from '../../../../core/query/queryClient'
import { clearStoredAuth } from '../../../../core/utils/auth'
import {
  addRemoteBoardFixture,
  makePromptBoardState,
  primeAuthenticatedWorkspace,
  createMockWorkspaceBoardApiRequest,
  createMockWorkspaceBoardApiState,
} from '../../testing'
import { getBoardState, replaceBoardStateMap } from '../persistence/state'
import { getBoardById, replaceBoards } from '../catalog/boards'
import { boardDocumentOptions, boardListOptions } from './boardQueries'
import { boardQueryKeys } from './boardQueryKeys'
import { subscribeBoardQueryInvalidation } from './boardSseInvalidation'
import { hydrateBoardsFromServer } from './boardSync'

// Isolated client carrying the production singleton defaults; see
// boardSourceOfTruth.contract.spec.ts for the isolation rationale.
function newIsolatedBoardQueryClient() {
  return new QueryClient({ defaultOptions: queryClient.getDefaultOptions() })
}

const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('../../../../api/client', () => ({ api: { request: mocks.request } }))

const WORKSPACE_ID = 'ws_abc123'
const OTHER_WORKSPACE_ID = 'ws_other999'

class MockEventSource {
  static instances: MockEventSource[] = []
  onerror: ((event: Event) => void) | null = null
  closed = false
  readonly url: string
  private listeners: Record<string, EventListener[]> = {}

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const fn: EventListener =
      typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
    this.listeners[type] = [...(this.listeners[type] ?? []), fn]
  }

  close() {
    this.closed = true
  }

  emit(type: string, data: unknown) {
    const event = new MessageEvent(type, {
      data: typeof data === 'string' ? data : JSON.stringify(data),
    })
    ;(this.listeners[type] ?? []).forEach((listener) => listener(event))
  }
}

function stubEventSource() {
  vi.stubGlobal('EventSource', MockEventSource)
}

function envelopeFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'evt_abc123',
    type: 'board.document_replaced',
    version: 1,
    occurred_at: '2026-10-11T00:00:00Z',
    actor: { type: 'user', id: 'usr_abc123' },
    resource: { type: 'board', id: 'board-1' },
    workspace_id: WORKSPACE_ID,
    data: { board_id: 'board-1', revision: 8 },
    ...overrides,
  }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-11T00:00:00Z'))
  vi.unstubAllGlobals()
  MockEventSource.instances = []
  resetWorkspaceEventStreamsForTests()
})

afterEach(() => {
  resetWorkspaceEventStreamsForTests()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('board SSE mapping', () => {
  let client: ReturnType<typeof newIsolatedBoardQueryClient>
  let apiState: ReturnType<typeof createMockWorkspaceBoardApiState>
  const stops: Array<() => void> = []
  function remoteRevision(revision: number, text = 'server') {
    addRemoteBoardFixture(apiState, WORKSPACE_ID, {
      id: 'board-1',
      name: 'Server Board',
      createdAt: 100,
      updatedAt: 1000,
      revision,
      state: makePromptBoardState(1000, text),
    })
  }
  async function observeDocument() {
    const observer = new QueryObserver(client, boardDocumentOptions(WORKSPACE_ID, 'board-1'))
    stops.push(observer.subscribe(() => undefined))
    await vi.advanceTimersByTimeAsync(0)
    return observer
  }
  async function observeList() {
    const observer = new QueryObserver(client, boardListOptions(WORKSPACE_ID))
    stops.push(observer.subscribe(() => undefined))
    await vi.advanceTimersByTimeAsync(0)
    return observer
  }
  function connect(isDraftDirty?: (boardId: string) => boolean) {
    stubEventSource()
    stops.push(subscribeBoardQueryInvalidation(client, WORKSPACE_ID, isDraftDirty))
    const transport = MockEventSource.instances.at(-1)!
    return (revision: number, id = `evt_revision${revision}`) => {
      transport.emit(
        'board.document_replaced',
        envelopeFixture({
          id,
          data: { board_id: 'board-1', revision, document: { text: 'untrusted event content' } },
        }),
      )
    }
  }
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace(WORKSPACE_ID)
    vi.clearAllMocks()
    apiState = createMockWorkspaceBoardApiState()
    remoteRevision(7)
    mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(apiState))
    client = newIsolatedBoardQueryClient()
  })
  afterEach(() => {
    stops.splice(0).forEach((stop) => stop())
    client.clear()
    vi.restoreAllMocks()
  })
  it('freezes the board event types in the workspace registry', () => {
    expect(WORKSPACE_EVENT_TYPES).toContain('board.document_replaced')
    expect(WORKSPACE_EVENT_TYPES).toContain('board.member_changed')
    expect(WORKSPACE_EVENT_TYPES).toContain('board.share_revoked')
    expect(isWorkspaceEventType('board.document_replaced')).toBe(true)
    expect(isWorkspaceEventType('board.member_changed')).toBe(true)
    expect(isWorkspaceEventType('board.share_revoked')).toBe(true)
  })

  it('fans out board envelopes per workspace and drops cross-workspace frames', () => {
    stubEventSource()
    const seen: WorkspaceEventEnvelope[] = []
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, (envelope) => seen.push(envelope), {
      types: ['board.document_replaced', 'board.member_changed', 'board.share_revoked'],
    })
    expect(MockEventSource.instances).toHaveLength(1)

    MockEventSource.instances[0]?.emit('board.document_replaced', envelopeFixture())
    expect(seen).toHaveLength(1)

    MockEventSource.instances[0]?.emit(
      'board.document_replaced',
      envelopeFixture({ id: 'evt_other999', workspace_id: OTHER_WORKSPACE_ID }),
    )
    expect(seen).toHaveLength(1)
    stop()
  })

  it('surfaces gap notices so board clients refetch state', () => {
    stubEventSource()
    const gaps: { lastEventId: string }[] = []
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined, {
      types: ['board.document_replaced'],
      onGap: (info) => gaps.push(info),
    })
    MockEventSource.instances[0]?.emit('board.document_replaced', envelopeFixture())
    MockEventSource.instances[0]?.emit('gap', { truncated: true })
    expect(gaps).toEqual([{ lastEventId: 'evt_abc123' }])
    stop()
  })

  describe('query invalidation mapping ownership', () => {
    it('invalidates the document on document_replaced with 500ms coalesce', async () => {
      const observer = await observeDocument()
      const emit = connect()
      const write = vi.spyOn(client, 'setQueryData')
      remoteRevision(10, 'latest from HTTP')
      emit(8)
      await vi.advanceTimersByTimeAsync(200)
      emit(9)
      emit(10)
      await vi.advanceTimersByTimeAsync(299)
      expect(mocks.request).toHaveBeenCalledTimes(1)
      expect(observer.getCurrentResult().data?.revision).toBe(7)
      await vi.advanceTimersByTimeAsync(1)

      expect(mocks.request).toHaveBeenCalledTimes(2)
      expect(observer.getCurrentResult().data).toMatchObject({
        revision: 10,
        document: { payload: { promptNodes: [{ text: 'latest from HTTP' }] } },
      })
      expect(write).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(500)
      expect(mocks.request).toHaveBeenCalledTimes(2)
    })

    it('drops document_replaced frames at or below the local revision', async () => {
      const observer = await observeDocument()
      const emit = connect()
      emit(7)
      emit(6)
      await vi.advanceTimersByTimeAsync(500)
      expect(mocks.request).toHaveBeenCalledTimes(1)
      expect(
        client.getQueryState(boardQueryKeys.document(WORKSPACE_ID, 'board-1'))?.isInvalidated,
      ).toBe(false)

      remoteRevision(8, 'advanced')
      emit(8)
      emit(8, 'evt_duplicate8')
      emit(7, 'evt_outoforder7')
      await vi.advanceTimersByTimeAsync(500)
      emit(8, 'evt_afterfetch8')
      emit(6, 'evt_outoforder6')
      await vi.advanceTimersByTimeAsync(500)
      expect(mocks.request).toHaveBeenCalledTimes(2)
      expect(observer.getCurrentResult().data?.revision).toBe(8)
    })

    it('marks draft-dirty boards stale without refetching or overwriting the draft', async () => {
      const observer = await observeDocument()
      replaceBoardStateMap({ 'board-1': makePromptBoardState(2000, 'unsaved local draft') })
      const emit = connect((boardId) => getBoardState(boardId) !== null)
      const write = vi.spyOn(client, 'setQueryData')
      remoteRevision(8, 'remote replacement')
      emit(8)
      await vi.advanceTimersByTimeAsync(500)

      expect(
        client.getQueryState(boardQueryKeys.document(WORKSPACE_ID, 'board-1'))?.isInvalidated,
      ).toBe(true)
      expect(observer.getCurrentResult().isStale).toBe(true)
      expect(mocks.request).toHaveBeenCalledTimes(1)
      expect(observer.getCurrentResult().data).toMatchObject({
        revision: 7,
        document: { payload: { promptNodes: [{ text: 'server' }] } },
      })
      expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('unsaved local draft')
      expect(write).not.toHaveBeenCalled()
    })

    it('invalidates the list on member_changed without fetching the document', async () => {
      await observeList()
      await client.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-1'))
      connect()
      MockEventSource.instances[0]!.emit(
        'board.member_changed',
        envelopeFixture({
          id: 'evt_member8',
          type: 'board.member_changed',
          data: { board_id: 'board-1' },
        }),
      )
      await vi.advanceTimersByTimeAsync(0)

      expect(mocks.request).toHaveBeenCalledTimes(3)
      expect(
        mocks.request.mock.calls.filter(([url]) => String(url).endsWith('/document')),
      ).toHaveLength(1)
      expect(
        client.getQueryState(boardQueryKeys.document(WORKSPACE_ID, 'board-1'))?.isInvalidated,
      ).toBe(false)
    })

    it('invalidates the list and removes the document on share_revoked', async () => {
      await observeList()
      await client.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-1'))
      const emit = connect()
      emit(8)
      MockEventSource.instances[0]!.emit(
        'board.share_revoked',
        envelopeFixture({
          id: 'evt_revoked8',
          type: 'board.share_revoked',
          data: { board_id: 'board-1' },
        }),
      )
      await vi.advanceTimersByTimeAsync(500)

      expect(mocks.request).toHaveBeenCalledTimes(3)
      expect(
        client
          .getQueryCache()
          .find({ queryKey: boardQueryKeys.document(WORKSPACE_ID, 'board-1'), exact: true }),
      ).toBeUndefined()
      expect(client.getQueryData(boardQueryKeys.list(WORKSPACE_ID))).toBeDefined()
    })

    describe('share_revoked gallery convergence', () => {
      function connectRevoked(
        convergence: {
          isBoardVisible?: (boardId: string) => boolean
          requestCatalogHydrate?: () => void | Promise<unknown>
        } = {},
      ) {
        stubEventSource()
        stops.push(subscribeBoardQueryInvalidation(client, WORKSPACE_ID, () => false, convergence))
      }

      function emitShareRevoked(id = 'evt_revoked8') {
        MockEventSource.instances[0]!.emit(
          'board.share_revoked',
          envelopeFixture({
            id,
            type: 'board.share_revoked',
            data: { board_id: 'board-1' },
          }),
        )
      }

      it('requests one hydrate convergence when the revoked board is still shown', async () => {
        await observeList()
        await client.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-1'))
        const isBoardVisible = vi.fn((boardId: string) => boardId === 'board-1')
        const requestCatalogHydrate = vi.fn()
        connectRevoked({ isBoardVisible, requestCatalogHydrate })
        emitShareRevoked()
        await vi.advanceTimersByTimeAsync(0)

        expect(isBoardVisible).toHaveBeenCalledWith('board-1')
        expect(requestCatalogHydrate).toHaveBeenCalledTimes(1)
        expect(
          client
            .getQueryCache()
            .find({ queryKey: boardQueryKeys.document(WORKSPACE_ID, 'board-1'), exact: true }),
        ).toBeUndefined()
      })

      it('coalesces duplicate revoked frames into the single in-flight convergence', async () => {
        await observeList()
        const resolvers: Array<() => void> = []
        const requestCatalogHydrate = vi.fn(
          () =>
            new Promise<void>((resolve) => {
              resolvers.push(resolve)
            }),
        )
        connectRevoked({ isBoardVisible: () => true, requestCatalogHydrate })
        emitShareRevoked('evt_revoked8')
        emitShareRevoked('evt_revoked8-duplicate')
        await vi.advanceTimersByTimeAsync(0)
        expect(requestCatalogHydrate).toHaveBeenCalledTimes(1)

        resolvers.splice(0).forEach((resolve) => resolve())
        await vi.advanceTimersByTimeAsync(0)
        emitShareRevoked('evt_revoked8-retry')
        await vi.advanceTimersByTimeAsync(0)
        expect(requestCatalogHydrate).toHaveBeenCalledTimes(2)
      })

      it('skips the convergence when the revoked board is not shown', async () => {
        await observeList()
        await client.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-1'))
        const requestCatalogHydrate = vi.fn()
        connectRevoked({ isBoardVisible: () => false, requestCatalogHydrate })
        emitShareRevoked()
        await vi.advanceTimersByTimeAsync(500)

        expect(requestCatalogHydrate).not.toHaveBeenCalled()
        expect(
          client
            .getQueryCache()
            .find({ queryKey: boardQueryKeys.document(WORKSPACE_ID, 'board-1'), exact: true }),
        ).toBeUndefined()
        expect(client.getQueryData(boardQueryKeys.list(WORKSPACE_ID))).toBeDefined()
      })

      it('converges the catalog without refetching the removed board document', async () => {
        // The local catalog still shows the synced entry while the server no
        // longer lists board-1 (revoked elsewhere), so the gallery is stale.
        replaceBoards([
          {
            id: 'board-1',
            name: 'Server Board',
            createdAt: 100,
            updatedAt: 1000,
            revision: 7,
            workspaceId: WORKSPACE_ID,
          },
        ])
        await observeList()
        await client.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-1'))
        const documentReads = () =>
          mocks.request.mock.calls.filter(([url]) => String(url).endsWith('/document'))
        expect(documentReads()).toHaveLength(1)
        apiState.boardsResponse.data = apiState.boardsResponse.data.filter(
          (board) => board.id !== 'board-1',
        )
        delete apiState.documentById['board-1']

        stubEventSource()
        stops.push(
          subscribeBoardQueryInvalidation(client, WORKSPACE_ID, undefined, {
            requestCatalogHydrate: () =>
              hydrateBoardsFromServer({ includeDocuments: false, force: true }),
          }),
        )
        emitShareRevoked()
        for (let i = 0; i < 10 && getBoardById('board-1') !== null; i += 1) {
          await vi.advanceTimersByTimeAsync(10)
        }

        expect(getBoardById('board-1')).toBeNull()
        expect(documentReads()).toHaveLength(1)
        queryClient.clear()
      })
    })

    it('invalidates the scoped boards root on gap and never setQueryData document content', async () => {
      await client.fetchQuery(boardListOptions(WORKSPACE_ID))
      await client.fetchQuery(boardDocumentOptions(WORKSPACE_ID, 'board-1'))
      client.setQueryData(boardQueryKeys.list(OTHER_WORKSPACE_ID), [{ id: 'foreign-board' }])
      client.setQueryData(['tasks', WORKSPACE_ID], [{ id: 'task-1' }])
      const previous = client.getQueryData(boardQueryKeys.document(WORKSPACE_ID, 'board-1'))
      connect()
      const write = vi.spyOn(client, 'setQueryData')
      MockEventSource.instances[0]!.emit('gap', { truncated: true })
      await vi.advanceTimersByTimeAsync(0)

      expect(client.getQueryState(boardQueryKeys.list(WORKSPACE_ID))?.isInvalidated).toBe(true)
      expect(
        client.getQueryState(boardQueryKeys.document(WORKSPACE_ID, 'board-1'))?.isInvalidated,
      ).toBe(true)
      expect(client.getQueryState(boardQueryKeys.list(OTHER_WORKSPACE_ID))?.isInvalidated).toBe(
        false,
      )
      expect(client.getQueryState(['tasks', WORKSPACE_ID])?.isInvalidated).toBe(false)
      expect(client.getQueryData(boardQueryKeys.document(WORKSPACE_ID, 'board-1'))).toEqual(
        previous,
      )
      expect(mocks.request).toHaveBeenCalledTimes(2)
      expect(write).not.toHaveBeenCalled()
    })
  })
})
