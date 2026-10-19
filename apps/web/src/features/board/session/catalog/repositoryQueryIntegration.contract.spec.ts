import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { createBoardSessionRepository } from '../../../canvas/session/board-session'
import { queryClient } from '../../../../core/query/queryClient'
import { clearStoredAuth } from '../../../../core/utils/auth'
import { resetWorkspaceEventStreamsForTests } from '../../../../api/workspaceEventStream'
import { getWorkspaceSelectionStore } from '../../../workspace'
import {
  createMockWorkspaceBoardApiRequest,
  createMockWorkspaceBoardApiState,
  addRemoteBoardFixture,
  makePromptBoardState,
  primeAuthenticatedWorkspace,
} from '../../testing'
import { getBoardState, replaceBoardStateMap, setBoardState } from '../persistence/state'
import { BoardCatalogIncompleteError } from '../adapters/boardsResource'
import { boardQueryKeys } from '../sync/boardQueryKeys'
import {
  hydrateBoardsFromServer,
  observeBoardServerSnapshots,
  syncBoardToServer,
} from '../sync/boardSync'
import { getBoardById, replaceBoards } from './boards'
import { createBoardRepository, type BoardRepository } from './repository'

const WORKSPACE_ID = 'ws_query123'
const OTHER_WORKSPACE_ID = 'ws_other999'
const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('../../../../api/client', () => ({ api: { request: mocks.request } }))

class MockEventSource {
  static instances: MockEventSource[] = []
  onerror: ((event: Event) => void) | null = null
  private listeners = new Map<string, EventListener[]>()
  readonly url: string
  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }
  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const fn = typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn])
  }
  close() {}
  emit(type: string, data: unknown) {
    const event = new MessageEvent(type, { data: JSON.stringify(data) })
    this.listeners.get(type)?.forEach((listener) => listener(event))
  }
}

describe('production Board repository Query integration', () => {
  let apiState: ReturnType<typeof createMockWorkspaceBoardApiState>
  const repositories: BoardRepository[] = []
  const stops: Array<() => void> = []
  const pendingReads: Array<() => void> = []
  function repository() {
    const result = createBoardRepository()
    repositories.push(result)
    return result
  }
  function remote(revision: number, text = 'server', workspaceId = WORKSPACE_ID) {
    apiState.boardsResponse.data = apiState.boardsResponse.data.filter(
      (board) => board.id !== 'board-1',
    )
    addRemoteBoardFixture(apiState, workspaceId, {
      id: 'board-1',
      name: 'Server Board',
      createdAt: 100,
      updatedAt: 1000,
      revision,
      state: makePromptBoardState(1000, text),
    })
  }
  function emit(type: 'board.access_changed' | 'board.document_replaced', revision = 8) {
    MockEventSource.instances.at(-1)!.emit(type, {
      id: `evt_revision${revision}`,
      type,
      version: 1,
      occurred_at: '2026-10-11T00:00:00Z',
      actor: { type: 'user', id: 'usr_test123' },
      resource: { type: 'board', id: 'board-1' },
      workspace_id: WORKSPACE_ID,
      data: { board_id: 'board-1', revision, access_version: revision },
    })
  }
  beforeEach(() => {
    queryClient.clear()
    localStorage.clear()
    clearStoredAuth()
    primeAuthenticatedWorkspace(WORKSPACE_ID)
    vi.clearAllMocks()
    MockEventSource.instances = []
    vi.stubGlobal('EventSource', MockEventSource)
    apiState = createMockWorkspaceBoardApiState()
    remote(7)
    mocks.request.mockImplementation(createMockWorkspaceBoardApiRequest(apiState))
  })
  afterEach(() => {
    pendingReads.splice(0).forEach((resolve) => resolve())
    stops.splice(0).forEach((stop) => stop())
    repositories.splice(0).forEach((entry) => entry.dispose())
    resetWorkspaceEventStreamsForTests()
    queryClient.clear()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('keeps the gallery list-only and updates its production view after member SSE', async () => {
    const catalog = repository()
    expect(
      await catalog.commands.hydrate({
        shouldPreserveLocalNewer: true,
        shouldIncludeDocuments: false,
      }),
    ).toEqual({ isOk: true })
    expect(mocks.request).toHaveBeenCalledTimes(1)
    expect(catalog.viewModel.boards.value[0]?.name).toBe('Server Board')
    apiState.boardsResponse.data[0]!.name = 'Renamed remotely'
    emit('board.access_changed')
    await vi.waitFor(() => expect(catalog.viewModel.boards.value[0]?.name).toBe('Renamed remotely'))
    expect(
      mocks.request.mock.calls.filter(([url]) => String(url).endsWith('/document')),
    ).toHaveLength(0)
  })

  it('refetches and projects a clean document through the production observer after SSE', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('server')
    remote(10, 'latest from HTTP')
    emit('board.document_replaced', 10)
    await vi.advanceTimersByTimeAsync(499)
    expect(mocks.request).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('latest from HTTP')
    expect(getBoardById('board-1')?.revision).toBe(10)
    expect(mocks.request).toHaveBeenCalledTimes(4)
  })

  it('aborts the actual hydrate on workspace navigation and ignores a late document', async () => {
    const catalog = repository()
    let resolveOld!: (value: unknown) => void
    let oldSignal: AbortSignal | undefined
    const oldDocument = apiState.documentById['board-1']
    const newState = createMockWorkspaceBoardApiState()
    addRemoteBoardFixture(newState, OTHER_WORKSPACE_ID, {
      id: 'board-1',
      name: 'Other Board',
      createdAt: 100,
      updatedAt: 1000,
      revision: 9,
      state: makePromptBoardState(1000, 'other workspace'),
    })
    const oldRead = new Promise((resolve) => {
      resolveOld = resolve
    })
    pendingReads.push(() => resolveOld({ data: oldDocument }))
    mocks.request.mockImplementation(
      (url: string, config: { signal?: AbortSignal; method?: string }) => {
        if (url.includes(`/workspaces/${WORKSPACE_ID}/`) && url.endsWith('/document')) {
          oldSignal = config.signal
          return oldRead
        }
        return createMockWorkspaceBoardApiRequest(
          url.includes(OTHER_WORKSPACE_ID) ? newState : apiState,
        )(url, config)
      },
    )
    const hydrate = catalog.commands.hydrate()
    await vi.waitFor(() =>
      expect(mocks.request.mock.calls.some(([url]) => String(url).endsWith('/document'))).toBe(
        true,
      ),
    )
    getWorkspaceSelectionStore().select(OTHER_WORKSPACE_ID)
    expect(oldSignal?.aborted).toBe(true)
    const newHydrate = catalog.commands.hydrate()
    await expect(newHydrate).resolves.toEqual({ isOk: true })
    resolveOld({ data: oldDocument })
    await expect(hydrate).resolves.toMatchObject({ isOk: false })
    expect(
      queryClient.getQueryData(boardQueryKeys.document(WORKSPACE_ID, 'board-1')),
    ).toBeUndefined()
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('other workspace')
    expect(catalog.viewModel.boards.value[0]?.revision).toBe(9)
  })

  it('accepts new remote content after save even when the local clock is newer', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    const saved = setBoardState('board-1', makePromptBoardState(1000, 'saved edit'))!
    await syncBoardToServer('board-1', saved)
    await vi.advanceTimersByTimeAsync(0)
    remote(10, 'new collaborator edit')
    const reads = mocks.request.mock.calls.length
    emit('board.document_replaced', 10)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 2)
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('new collaborator edit')
    expect(getBoardById('board-1')?.revision).toBe(10)
  })

  it('marks a conflicted local draft stale without a document GET or advancing its base', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    setBoardState('board-1', makePromptBoardState(1000, 'unsaved edit'))
    apiState.saveError = { status: 409, code: 'board_revision_conflict' }
    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({ status: 409 })
    remote(10, 'remote edit')
    const reads = mocks.request.mock.calls.length
    emit('board.document_replaced', 10)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 1)
    expect(mocks.request.mock.calls[reads]?.[0]).not.toContain('/document')
    expect(
      queryClient.getQueryState(boardQueryKeys.document(WORKSPACE_ID, 'board-1'))?.isInvalidated,
    ).toBe(true)
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('unsaved edit')
    expect(getBoardById('board-1')?.revision).toBe(7)
  })

  it('does not clear a newer draft when an older in-flight save finishes', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    const submitted = setBoardState('board-1', makePromptBoardState(1000, 'submitted edit'))!
    const request = createMockWorkspaceBoardApiRequest(apiState)
    let finish!: () => void
    mocks.request.mockImplementation(async (url: string, config: { method?: string }) => {
      const result = await request(url, config)
      if (config.method === 'PUT')
        await new Promise<void>((resolve) => {
          finish = resolve
        })
      return result
    })
    const saving = syncBoardToServer('board-1', submitted)
    await vi.advanceTimersByTimeAsync(0)
    setBoardState('board-1', makePromptBoardState(1000, 'edit during save'))
    finish()
    await saving
    await vi.advanceTimersByTimeAsync(0)
    remote(10, 'remote edit')
    const reads = mocks.request.mock.calls.length
    emit('board.document_replaced', 10)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 1)
    expect(mocks.request.mock.calls[reads]?.[0]).not.toContain('/document')
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('edit during save')
    expect(getBoardById('board-1')?.revision).toBe(8)
  })

  it('protects live Canvas edits before the autosave debounce persists them', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    const scope = effectScope()
    stops.push(() => scope.stop())
    let isLiveDirty = true
    scope.run(() => createBoardSessionRepository((boardId) => boardId === 'board-1' && isLiveDirty))
    remote(10, 'remote edit')
    const reads = mocks.request.mock.calls.length
    emit('board.document_replaced', 10)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 1)
    expect(mocks.request.mock.calls[reads]?.[0]).not.toContain('/document')
    expect(getBoardById('board-1')?.revision).toBe(7)
    isLiveDirty = false
    scope.stop()
    emit('board.document_replaced', 11)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 3)
    expect(getBoardById('board-1')?.revision).toBe(10)
  })

  it('reuses ordinary Canvas navigation reads within the document freshness window', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    const session = createBoardSessionRepository()
    await session.load('board-1')
    expect(mocks.request).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(5001)
    await session.load('board-1')
    expect(mocks.request).toHaveBeenCalledTimes(3)
  })

  it('keeps the same live session guard on workspace navigation and releases it on disposal', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    const scope = effectScope()
    stops.push(() => scope.stop())
    scope.run(() => createBoardSessionRepository((boardId) => boardId === 'board-1'))
    getWorkspaceSelectionStore().select(OTHER_WORKSPACE_ID)
    remote(7, 'other workspace', OTHER_WORKSPACE_ID)
    await catalog.commands.hydrate()
    remote(10, 'remote edit', OTHER_WORKSPACE_ID)
    const reads = mocks.request.mock.calls.length
    const source = MockEventSource.instances.at(-1)!
    const event = (revision: number) =>
      source.emit('board.document_replaced', {
        id: `evt_other${revision}`,
        type: 'board.document_replaced',
        version: 1,
        occurred_at: '2026-10-11T00:00:00Z',
        actor: { type: 'user', id: 'usr_test123' },
        resource: { type: 'board', id: 'board-1' },
        workspace_id: OTHER_WORKSPACE_ID,
        data: { board_id: 'board-1', revision, access_version: revision },
      })
    event(10)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 1)
    expect(mocks.request.mock.calls[reads]?.[0]).not.toContain('/document')
    expect(getBoardById('board-1')?.revision).toBe(7)
    scope.stop()
    event(11)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 3)
    expect(getBoardById('board-1')?.revision).toBe(10)
  })

  it('preserves unknown cold-storage content rather than treating timestamps as a clean baseline', async () => {
    vi.useFakeTimers()
    replaceBoards([
      { id: 'board-1', name: 'Offline Draft', createdAt: 100, updatedAt: 2000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-1': makePromptBoardState(2000, 'offline draft') })
    const catalog = repository()
    await catalog.commands.hydrate()
    remote(10, 'remote edit')
    const reads = mocks.request.mock.calls.length
    emit('board.document_replaced', 10)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.request).toHaveBeenCalledTimes(reads + 1)
    expect(mocks.request.mock.calls[reads]?.[0]).not.toContain('/document')
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('offline draft')
    expect(getBoardById('board-1')?.revision).toBe(7)
  })

  it('keeps server catalog separate from conflict drafts and fetches fresh on explicit reload', async () => {
    replaceBoards([
      { id: 'board-1', name: 'Local Rename', createdAt: 100, updatedAt: 2000, revision: 7 },
    ])
    replaceBoardStateMap({ 'board-1': makePromptBoardState(2000, 'local draft') })
    const catalog = repository()
    await catalog.commands.hydrate()
    expect(catalog.viewModel.boards.value[0]?.name).toBe('Server Board')
    expect(getBoardById('board-1')?.name).toBe('Local Rename')
    remote(8, 'remote winner')
    apiState.saveError = { status: 409, code: 'board_revision_conflict' }
    await expect(syncBoardToServer('board-1')).rejects.toMatchObject({ status: 409 })
    expect(getBoardById('board-1')?.revision).toBe(7)
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('local draft')
    expect(await catalog.commands.hydrate({ shouldPreserveLocalNewer: false })).toEqual({
      isOk: true,
    })
    expect(getBoardById('board-1')?.revision).toBe(8)
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('remote winner')
  })

  it('reads the newer catalog document before Canvas navigation despite its cached freshness', async () => {
    vi.useFakeTimers()
    const catalog = repository()
    await catalog.commands.hydrate()
    remote(8, 'revision eight')
    await catalog.commands.hydrate({ shouldIncludeDocuments: false })
    expect(catalog.viewModel.boards.value[0]?.revision).toBe(8)
    expect(getBoardById('board-1')?.revision).toBe(7)
    const session = createBoardSessionRepository()
    const snapshot = await session.load('board-1')
    expect(snapshot?.promptNodes[0]?.text).toBe('revision eight')
    expect(getBoardById('board-1')?.revision).toBe(8)
    await syncBoardToServer('board-1')
    expect(apiState.savedCalls.at(-1)?.headers?.['If-Match']).toBe('8')
    expect(getBoardState('board-1')?.promptNodes[0]?.text).toBe('revision eight')
  })

  it('preserves local drafts and synced metas when the catalog walk is incomplete', async () => {
    replaceBoards([{ id: 'offline-1', name: 'Offline', createdAt: 100, updatedAt: 2000 }])
    const catalog = repository()
    expect(
      await catalog.commands.hydrate({
        shouldPreserveLocalNewer: true,
        shouldIncludeDocuments: false,
      }),
    ).toEqual({ isOk: true })
    expect(catalog.viewModel.boards.value.map((board) => board.id).sort()).toEqual([
      'board-1',
      'offline-1',
    ])

    // A truncated or partially failed walk rejects instead of returning a
    // partial list: absence must never read as deletion or revocation.
    apiState.listError = new BoardCatalogIncompleteError('truncated walk')
    const result = await catalog.commands.hydrate({ shouldIncludeDocuments: false })
    expect(result).toMatchObject({ isOk: false })
    expect(catalog.viewModel.boards.value.map((board) => board.id).sort()).toEqual([
      'board-1',
      'offline-1',
    ])
    expect(getBoardById('offline-1')?.name).toBe('Offline')
    expect(getBoardById('board-1')?.name).toBe('Server Board')
  })

  it.each(['returned', 'thrown'] as const)(
    'restores production list-only observation after a %s hydrate failure',
    async (failure) => {
      vi.useFakeTimers()
      const hydrate = vi.fn(hydrateBoardsFromServer)
      const catalog = createBoardRepository({ hydrate, observeServer: observeBoardServerSnapshots })
      repositories.push(catalog)
      await catalog.commands.hydrate({ shouldIncludeDocuments: false })
      if (failure === 'returned')
        hydrate.mockResolvedValueOnce({ isOk: false, error: new Error('test outage') })
      else hydrate.mockRejectedValueOnce(new Error('test outage'))
      await catalog.commands.hydrate().catch(() => {})
      apiState.boardsResponse.data[0]!.name = 'After failed hydrate'
      emit('board.access_changed')
      await vi.advanceTimersByTimeAsync(0)
      expect(catalog.viewModel.boards.value[0]?.name).toBe('After failed hydrate')
      expect(
        mocks.request.mock.calls.filter(([url]) => String(url).endsWith('/document')),
      ).toHaveLength(0)
    },
  )

  it.each(['dispose', 'scope'] as const)(
    'does not restore a failed observer after a %s transition',
    async (transition) => {
      vi.useFakeTimers()
      const hydrate = vi.fn(hydrateBoardsFromServer)
      const observeServer = vi.fn(observeBoardServerSnapshots)
      const catalog = createBoardRepository({ hydrate, observeServer })
      repositories.push(catalog)
      await catalog.commands.hydrate({ shouldIncludeDocuments: false })
      let fail!: () => void
      hydrate.mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            fail = () => resolve({ isOk: false, error: new Error('test outage') })
          }),
      )
      const pending = catalog.commands.hydrate()
      await vi.advanceTimersByTimeAsync(0)
      if (transition === 'dispose') catalog.dispose()
      else {
        getWorkspaceSelectionStore().select(OTHER_WORKSPACE_ID)
        remote(9, 'other workspace', OTHER_WORKSPACE_ID)
        await catalog.commands.hydrate({ shouldIncludeDocuments: false })
      }
      const observations = observeServer.mock.calls.length
      fail()
      await pending
      expect(observeServer).toHaveBeenCalledTimes(observations)
      if (transition === 'scope') expect(catalog.viewModel.boards.value[0]?.revision).toBe(9)
    },
  )
})
