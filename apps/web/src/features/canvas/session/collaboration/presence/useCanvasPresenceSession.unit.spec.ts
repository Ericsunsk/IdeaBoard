import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import type * as Y from 'yjs'
import {
  EVENT_STREAM_RECONNECT_BACKOFF_FACTOR,
  EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS,
} from '../../../../../core/config/timing'
import type { PresenceAwarenessState, PresenceUser } from './state'
import { useCanvasPresenceSession } from './useCanvasPresenceSession'
import type {
  PresenceCloseEvent,
  PresenceProviderFactory,
  PresenceProviderHandle,
  PresenceProviderStatus,
} from './provider'

type FakePresence = {
  handle: PresenceProviderHandle
  localPublishes: Array<PresenceAwarenessState | null>
  getIsDisposed: () => boolean
  emitRemote: (clientId: number, raw: unknown) => void
  emitClose: (code?: number | undefined) => void
  emitStatus: (status: PresenceProviderStatus) => void
  emitSync: (isSynced: boolean) => void
}

function buildFakePresence(): FakePresence {
  const states = new Map<number, unknown>()
  const changeListeners = new Set<() => void>()
  const closeListeners = new Set<(event: PresenceCloseEvent) => void>()
  const statusListeners = new Set<(status: PresenceProviderStatus) => void>()
  const syncListeners = new Set<(isSynced: boolean) => void>()
  const localPublishes: Array<PresenceAwarenessState | null> = []
  let isDisposed = false
  const handle: PresenceProviderHandle = {
    provider: {
      isSynced: true,
      onSync: (listener) => {
        syncListeners.add(listener)
        return () => {
          syncListeners.delete(listener)
        }
      },
      awareness: {
        clientID: 41,
        setLocalState: (state) => {
          localPublishes.push(state)
          if (state === null) states.delete(41)
          else states.set(41, state)
        },
        getStates: () => new Map(states),
        on: (_event, listener) => {
          changeListeners.add(listener)
        },
        off: (_event, listener) => {
          changeListeners.delete(listener)
        },
      },
      onConnectionClose: (listener) => {
        closeListeners.add(listener)
        return () => {
          closeListeners.delete(listener)
        }
      },
      onStatus: (listener) => {
        statusListeners.add(listener)
        return () => {
          statusListeners.delete(listener)
        }
      },
    },
    doc: null as unknown as Y.Doc,
    dispose: () => {
      isDisposed = true
    },
  }
  return {
    handle,
    localPublishes,
    emitSync: (isSynced) => {
      for (const listener of syncListeners) listener(isSynced)
    },
    getIsDisposed: () => isDisposed,
    emitRemote: (clientId, raw) => {
      states.set(clientId, raw)
      for (const listener of [...changeListeners]) listener()
    },
    emitClose: (code) => {
      const event: PresenceCloseEvent = code === undefined ? null : { code, reason: 'test' }
      for (const listener of [...closeListeners]) listener(event)
    },
    emitStatus: (status) => {
      for (const listener of [...statusListeners]) listener(status)
    },
  }
}

function buildLocalUser(): PresenceUser {
  return { id: 'usr_ada', name: 'Ada' }
}

function buildRemoteState(overrides: Partial<PresenceAwarenessState> = {}): PresenceAwarenessState {
  return {
    user: { id: 'usr_bob', name: 'Bob' },
    selection: { nodeIds: ['node-1'] },
    ...overrides,
  }
}

async function flushAsync(rounds = 12): Promise<void> {
  for (let index = 0; index < rounds; index += 1) {
    await Promise.resolve()
  }
}

function mountPresence(input?: {
  boardId?: string | null
  accessState?: string | undefined
  canWrite?: boolean | undefined
}) {
  const ticketFn = vi.fn(async (_workspaceId: string, _boardId: string) => 'ticket-1')
  const fakes: FakePresence[] = []
  const providerFn: PresenceProviderFactory = vi.fn(async () => {
    const fake = buildFakePresence()
    fakes.push(fake)
    return fake.handle
  })
  const boardId = ref<string | null | undefined>(
    input && 'boardId' in input ? input.boardId : 'board-1',
  )
  const accessState = ref<string | null | undefined>(
    input && 'accessState' in input ? input.accessState : 'workspace',
  )
  const workspaceId = ref<string | null>('ws-1')
  const localUser = ref<PresenceUser | null>(buildLocalUser())
  const canWrite = ref<boolean>(input?.canWrite ?? true)
  const selectedIds = ref<string[]>([])
  const scope = effectScope()
  const presence = scope.run(() =>
    useCanvasPresenceSession({
      boardId,
      accessState,
      workspaceId,
      localUser,
      canWrite,
      selectedIds,
      deps: { requestTicket: ticketFn, createProvider: providerFn },
    }),
  )
  if (!presence) throw new Error('presence failed to mount')
  return {
    scope,
    presence,
    ticketFn,
    providerFn,
    fakes,
    boardId,
    accessState,
    localUser,
    canWrite,
    selectedIds,
  }
}

type MountedPresence = ReturnType<typeof mountPresence>

async function publishSelection(mounted: MountedPresence, nodeIds: string[]): Promise<void> {
  mounted.selectedIds.value = [...nodeIds]
  await flushAsync()
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('canvas presence connection gate', () => {
  test('tracks document sync readiness and clears it after disconnect', async () => {
    const mounted = mountPresence()
    await flushAsync()
    expect(mounted.presence.isDocSynced.value).toBe(true)
    mounted.fakes[0]?.emitSync(false)
    expect(mounted.presence.isDocSynced.value).toBe(false)
    mounted.fakes[0]?.emitSync(true)
    expect(mounted.presence.isDocSynced.value).toBe(true)
    mounted.fakes[0]?.emitClose(1006)
    expect(mounted.presence.isDocSynced.value).toBe(false)
    mounted.scope.stop()
  })
  test('signs a ticket and joins the board room on mount', async () => {
    const mounted = mountPresence()
    await flushAsync()

    expect(mounted.ticketFn).toHaveBeenCalledTimes(1)
    expect(mounted.ticketFn).toHaveBeenCalledWith('ws-1', 'board-1')
    expect(mounted.providerFn).toHaveBeenCalledTimes(1)
    const room = vi.mocked(mounted.providerFn).mock.calls[0]?.[0]
    expect(room?.room).toBe('board-1')
    expect(room?.ticket).toBe('ticket-1')
    expect(room?.serverBaseUrl.endsWith('/realtime')).toBe(true)
  })

  test('never connects private boards, and connects once shared', async () => {
    const mounted = mountPresence({ accessState: 'private' })
    await flushAsync()
    expect(mounted.ticketFn).not.toHaveBeenCalled()

    mounted.accessState.value = 'workspace'
    await flushAsync()
    expect(mounted.ticketFn).toHaveBeenCalledTimes(1)
    expect(mounted.providerFn).toHaveBeenCalledTimes(1)
  })

  test('never connects drafts, and connects once the board id lands', async () => {
    const mounted = mountPresence({ boardId: null })
    await flushAsync()
    expect(mounted.ticketFn).not.toHaveBeenCalled()

    mounted.boardId.value = 'board-1'
    await flushAsync()
    expect(mounted.ticketFn).toHaveBeenCalledTimes(1)
  })

  test('switching boards tears down the old room and signs the new one', async () => {
    const mounted = mountPresence()
    await flushAsync()
    expect(mounted.fakes).toHaveLength(1)

    mounted.boardId.value = 'board-2'
    await flushAsync()
    expect(mounted.fakes[0]?.getIsDisposed()).toBe(true)
    expect(mounted.ticketFn).toHaveBeenLastCalledWith('ws-1', 'board-2')
    expect(mounted.fakes).toHaveLength(2)
    await publishSelection(mounted, ['node-7'])
    expect(mounted.fakes[1]?.localPublishes.at(-1)?.selection).toEqual({
      nodeIds: ['node-7'],
    })
  })
})

describe('canvas presence broadcast', () => {
  test('publishes selection changes immediately', async () => {
    const mounted = mountPresence()
    await flushAsync()
    mounted.fakes[0]?.emitStatus('connected')

    await publishSelection(mounted, ['node-9'])
    const published = mounted.fakes[0]?.localPublishes ?? []
    const last = published[published.length - 1]
    expect(last?.user).toEqual({ id: 'usr_ada', name: 'Ada' })
    expect(last?.selection).toEqual({ nodeIds: ['node-9'] })
  })

  test('viewers broadcast selection-free and never claim selections', async () => {
    const mounted = mountPresence({ canWrite: false })
    await flushAsync()
    vi.advanceTimersByTime(200)
    await publishSelection(mounted, ['node-9'])
    const published = mounted.fakes[0]?.localPublishes ?? []
    const last = published[published.length - 1]
    expect(last?.selection).toEqual({ nodeIds: [] })
  })

  test('clears lock claims when write capability is revoked without changing selection', async () => {
    const mounted = mountPresence()
    await flushAsync()
    await publishSelection(mounted, ['node-9'])
    expect(mounted.fakes[0]?.localPublishes.at(-1)?.selection.nodeIds).toEqual(['node-9'])

    mounted.canWrite.value = false
    await flushAsync()
    expect(mounted.fakes[0]?.localPublishes.at(-1)?.selection.nodeIds).toEqual([])
    mounted.scope.stop()
  })

  test('syncs remote peers and drops malformed or self states', async () => {
    const mounted = mountPresence()
    await flushAsync()
    const fake = mounted.fakes[0]
    if (!fake) throw new Error('expected a provider')

    fake.emitRemote(7, buildRemoteState())
    fake.emitRemote(
      41,
      buildRemoteState({
        user: { id: 'usr_ada', name: 'Ada' },
        selection: { nodeIds: [] },
      }),
    )
    fake.emitRemote(9, {
      user: { id: '' },
      selection: { nodeIds: [] },
    })
    expect(mounted.presence.remotePeers.value).toHaveLength(1)
    const peer = mounted.presence.remotePeers.value[0]
    expect(peer?.clientId).toBe(7)
    expect(peer?.state.user).toEqual({ id: 'usr_bob', name: 'Bob' })
    expect(peer?.state.selection).toEqual({ nodeIds: ['node-1'] })
  })

  test('keeps every remote peer so the roster renders the full collaborator set', async () => {
    const mounted = mountPresence()
    await flushAsync()
    const fake = mounted.fakes[0]
    if (!fake) throw new Error('expected a provider')

    fake.emitRemote(7, buildRemoteState())
    fake.emitRemote(
      8,
      buildRemoteState({
        user: { id: 'usr_cid', name: 'Cid' },
        selection: { nodeIds: [] },
      }),
    )
    fake.emitRemote(
      41,
      buildRemoteState({
        user: { id: 'usr_ada', name: 'Ada' },
        selection: { nodeIds: [] },
      }),
    )
    const peers = mounted.presence.remotePeers.value
    expect(peers.map((peer) => peer.clientId).sort((a, b) => a - b)).toEqual([7, 8])
    expect(peers.find((peer) => peer.clientId === 8)?.state.user).toEqual({
      id: 'usr_cid',
      name: 'Cid',
    })
    mounted.scope.stop()
  })
})

describe('canvas presence teardown', () => {
  test('disconnects on unmount and stops publishing', async () => {
    const mounted = mountPresence()
    await flushAsync()
    expect(mounted.fakes).toHaveLength(1)

    mounted.scope.stop()
    expect(mounted.fakes[0]?.getIsDisposed()).toBe(true)
    const before = (mounted.fakes[0]?.localPublishes ?? []).length
    await publishSelection(mounted, ['node-after-disposal'])
    vi.advanceTimersByTime(1000)
    expect((mounted.fakes[0]?.localPublishes ?? []).length).toBe(before)
    expect(mounted.ticketFn).toHaveBeenCalledTimes(1)
  })

  test('disconnects when the board goes private', async () => {
    const mounted = mountPresence()
    await flushAsync()
    mounted.fakes[0]?.emitStatus('connected')
    mounted.fakes[0]?.emitRemote(7, {
      user: { id: 'usr_bob', name: 'Bob' },
      selection: { nodeIds: [] },
    })
    expect(mounted.presence.remotePeers.value).toHaveLength(1)

    mounted.accessState.value = 'private'
    await flushAsync()
    expect(mounted.fakes[0]?.getIsDisposed()).toBe(true)
    expect(mounted.presence.remotePeers.value).toEqual([])
  })

  test('ignores a late connect after unmount', async () => {
    let resolveTicket!: (ticket: string) => void
    const ticketFn = vi.fn(
      () =>
        new Promise<string>((resolve) => {
          resolveTicket = resolve
        }),
    )
    const providerFn: PresenceProviderFactory = vi.fn(async () => buildFakePresence().handle)
    const scope = effectScope()
    scope.run(() =>
      useCanvasPresenceSession({
        boardId: ref('board-1'),
        accessState: ref('workspace'),
        workspaceId: ref('ws-1'),
        localUser: ref(buildLocalUser()),
        selectedIds: ref([]),
        deps: { requestTicket: ticketFn, createProvider: providerFn },
      }),
    )
    scope.stop()
    resolveTicket('late-ticket')
    await flushAsync()
    expect(providerFn).not.toHaveBeenCalled()
  })
})

describe('canvas presence reconnect', () => {
  test('re-signs the ticket with event-stream backoff after a drop', async () => {
    const mounted = mountPresence()
    await flushAsync()
    expect(mounted.fakes).toHaveLength(1)

    mounted.fakes[0]?.emitClose()
    vi.advanceTimersByTime(EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS - 1)
    expect(mounted.ticketFn).toHaveBeenCalledTimes(1)

    vi.advanceTimersByTime(1)
    await flushAsync()
    expect(mounted.ticketFn).toHaveBeenCalledTimes(2)
    expect(mounted.fakes).toHaveLength(2)
    expect(mounted.fakes[0]?.getIsDisposed()).toBe(true)
  })

  test('never reconnects permanent sidecar refusals', async () => {
    const mounted = mountPresence()
    await flushAsync()
    mounted.fakes[0]?.emitStatus('connected')
    mounted.fakes[0]?.emitRemote(7, buildRemoteState())
    expect(mounted.presence.remotePeers.value).toHaveLength(1)

    mounted.fakes[0]?.emitClose(4403)
    vi.advanceTimersByTime(60_000)
    await flushAsync()
    expect(mounted.ticketFn).toHaveBeenCalledTimes(1)
    expect(mounted.fakes).toHaveLength(1)
    expect(mounted.presence.remotePeers.value).toEqual([])
  })

  test('reconnects on abnormal closes', async () => {
    const mounted = mountPresence()
    await flushAsync()

    mounted.fakes[0]?.emitClose(1006)
    vi.advanceTimersByTime(EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS)
    await flushAsync()
    expect(mounted.ticketFn).toHaveBeenCalledTimes(2)
    expect(mounted.fakes).toHaveLength(2)
  })

  test('backs off across consecutive drops', async () => {
    const mounted = mountPresence()
    await flushAsync()
    mounted.fakes[0]?.emitClose()
    vi.advanceTimersByTime(EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS)
    await flushAsync()
    expect(mounted.fakes).toHaveLength(2)

    const secondDelay = Math.round(
      EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS * EVENT_STREAM_RECONNECT_BACKOFF_FACTOR,
    )
    mounted.fakes[1]?.emitClose()
    vi.advanceTimersByTime(secondDelay - 1)
    await flushAsync()
    expect(mounted.fakes).toHaveLength(2)
    vi.advanceTimersByTime(1)
    await flushAsync()
    expect(mounted.fakes).toHaveLength(3)
  })

  test('retries signing when the ticket request fails', async () => {
    const mounted = mountPresence({ boardId: null })
    await flushAsync()
    expect(mounted.ticketFn).not.toHaveBeenCalled()

    mounted.ticketFn.mockRejectedValueOnce(new Error('offline'))
    mounted.boardId.value = 'board-1'
    await flushAsync()
    expect(mounted.ticketFn).toHaveBeenCalledTimes(1)
    expect(mounted.providerFn).not.toHaveBeenCalled()

    vi.advanceTimersByTime(EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS)
    await flushAsync()
    expect(mounted.ticketFn).toHaveBeenCalledTimes(2)
    expect(mounted.providerFn).toHaveBeenCalledTimes(1)
  })
})

describe('canvas presence collaboration fields', () => {
  test('exposes the local device id for presence tracking', async () => {
    const mounted = mountPresence()
    await flushAsync()
    expect(mounted.presence.localClientId.value).toBe(41)
  })
})
