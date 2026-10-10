import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../../../api/sessionUserType'
import { queryClient } from '../../../core/query/queryClient'
import { clearStoredAuthState, getStoredUser, setStoredAuth } from '../../../core/utils/auth'
import {
  ensureWorkspaceMediaInvalidation,
  getWorkspaceMediaAssetMetadata,
  getWorkspaceMediaAssetMetadataBatch,
  invalidateMediaAssetMetadata,
} from '../../../api/mediaAssetDelivery'
import {
  resetWorkspaceEventStreamsForTests,
  subscribeWorkspaceEvent,
} from '../../../api/workspaceEventStream'
import { makeSessionUser } from '../../../test/factories'
import { createBrowserSessionTransition } from './browserSessionTransition'
import { SESSION_TRUST_WINDOW_MS } from './freshness'

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../api/client', () => ({
  api: { request: mocks.request },
}))

const WORKSPACE_ID = 'ws_test123'
const ASSET_ID = 'asset_uid42'

function assetEnvelope() {
  return {
    data: {
      id: ASSET_ID,
      workspace_id: WORKSPACE_ID,
      status: 'ready',
      media_kind: 'image',
      mime_type: 'image/jpeg',
      width: 2048,
      height: 1024,
      size_bytes: 100,
      checksum_sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
      content_url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
      derivatives: {
        thumb: { status: 'ready', url: '/api/v1/thumb.webp' },
        preview: { status: 'ready', url: '/api/v1/preview.webp' },
      },
      created_at: '2026-09-25T00:00:00.000Z',
      updated_at: '2026-09-25T00:00:00.000Z',
    },
  }
}

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
    const eventListener: EventListener =
      typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
    this.listeners[type] = [...(this.listeners[type] ?? []), eventListener]
  }

  removeEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const eventListener: EventListener =
      typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
    this.listeners[type] = (this.listeners[type] ?? []).filter((entry) => entry !== eventListener)
  }

  close() {
    this.closed = true
  }

  emit(type: string, data: unknown) {
    const event = new MessageEvent(type, { data: JSON.stringify(data) })
    for (const listener of this.listeners[type] ?? []) listener(event)
  }
}

function stubEventSource() {
  vi.stubGlobal('EventSource', MockEventSource)
}

const PROBE_KEY = ['identity', 'cache-reset-probe'] as const

function createHarness(
  overrides: Partial<{
    user: User | null
    isLoaded: boolean
    validatedAt: number
  }> = {},
) {
  let user = overrides.user ?? null
  let isLoaded = overrides.isLoaded ?? false
  let validatedAt = overrides.validatedAt ?? 0
  const transport = {
    hasSession: vi.fn().mockResolvedValue(true),
    logout: vi.fn().mockResolvedValue(true),
    stopImpersonating: vi.fn().mockResolvedValue(undefined),
  }
  const loadUser = vi.fn()
  const state = {
    getUser: () => user,
    isLoaded: () => isLoaded,
    getValidatedAt: () => validatedAt,
    commit: vi.fn((nextUser: User) => {
      user = nextUser
      isLoaded = true
      validatedAt = 0
    }),
    restore: vi.fn((storedUser: User) => {
      user = storedUser
    }),
    markUnloaded: vi.fn(() => {
      isLoaded = false
    }),
    clear: vi.fn(() => {
      user = null
      isLoaded = false
      validatedAt = 0
    }),
  }
  const transition = createBrowserSessionTransition(state, {
    transport,
    loadUser,
    now: () => SESSION_TRUST_WINDOW_MS + 1,
  })
  return { transition, transport, loadUser, state, getUser: () => user }
}

function seedQueryCache(owner: string) {
  queryClient.setQueryData([...PROBE_KEY], { owner })
}

function cachedOwner(): string | undefined {
  return (queryClient.getQueryData([...PROBE_KEY]) as { owner: string } | undefined)?.owner
}

describe('identity cache reset (query/identity-cache-reset)', () => {
  beforeEach(() => {
    queryClient.clear()
    localStorage.clear()
    vi.clearAllMocks()
    vi.unstubAllGlobals()
    MockEventSource.instances = []
    mocks.request.mockResolvedValue(assetEnvelope())
    invalidateMediaAssetMetadata()
    resetWorkspaceEventStreamsForTests()
  })

  it('clears the single Query cache when the authenticated identity changes', () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    seedQueryCache('user-1')
    expect(cachedOwner()).toBe('user-1')

    harness.transition.setAuth(makeSessionUser({ public_id: 'user-2' }))

    expect(harness.getUser()?.public_id).toBe('user-2')
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
    expect(cachedOwner()).toBeUndefined()
  })

  it('keeps cached data when the same identity recommits', () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    seedQueryCache('user-1')

    harness.transition.setAuth(makeSessionUser({ public_id: 'user-1' }))

    expect(harness.getUser()?.public_id).toBe('user-1')
    expect(cachedOwner()).toBe('user-1')
  })

  it('clears the cache and the persisted snapshot when the session expires', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    setStoredAuth(makeSessionUser({ public_id: 'user-1' }))
    seedQueryCache('user-1')
    harness.loadUser.mockRejectedValue({ status: 401 })

    await expect(harness.transition.fetchUserInfo(true)).resolves.toBeNull()

    expect(harness.getUser()).toBeNull()
    expect(getStoredUser()).toBeNull()
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
  })

  it('clears stored auth and the Query cache on silent auth-state reset', () => {
    setStoredAuth(makeSessionUser({ public_id: 'user-1' }))
    seedQueryCache('user-1')

    clearStoredAuthState()

    expect(getStoredUser()).toBeNull()
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
  })

  it('logout clears identity, persisted snapshot, remembered scope, and the Query cache', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    setStoredAuth(makeSessionUser({ public_id: 'user-1' }))
    seedQueryCache('user-1')

    await expect(harness.transition.logout()).resolves.toBe(true)

    expect(harness.getUser()).toBeNull()
    expect(getStoredUser()).toBeNull()
    expect(localStorage.getItem('ideaboard.auth.lastStorageScope.v1')).toBeNull()
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
  })
})

describe('identity switch resets workspace delivery and streams', () => {
  beforeEach(() => {
    queryClient.clear()
    localStorage.clear()
    vi.clearAllMocks()
    vi.unstubAllGlobals()
    MockEventSource.instances = []
    mocks.request.mockResolvedValue(assetEnvelope())
    invalidateMediaAssetMetadata()
    resetWorkspaceEventStreamsForTests()
  })

  it('drops cached workspace media metadata when the authenticated identity changes', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })

    const first = await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    expect(mocks.request).toHaveBeenCalledTimes(1)

    harness.transition.setAuth(makeSessionUser({ public_id: 'user-2' }))

    const second = await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    expect(mocks.request).toHaveBeenCalledTimes(2)
    expect(second).not.toBe(first)
  })

  it('closes workspace event streams when the authenticated identity changes', () => {
    stubEventSource()
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined)
    expect(MockEventSource.instances).toHaveLength(1)

    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.transition.setAuth(makeSessionUser({ public_id: 'user-2' }))

    expect(MockEventSource.instances[0]?.closed).toBe(true)
    stop()
  })

  it('keeps the replacement shared stream registered when an old disposer runs', () => {
    stubEventSource()
    const stopOld = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined)
    clearStoredAuthState()
    const seen = vi.fn()
    const stopNew = subscribeWorkspaceEvent(WORKSPACE_ID, seen)

    stopOld()
    const seenJoined = vi.fn()
    const stopJoined = subscribeWorkspaceEvent(WORKSPACE_ID, seenJoined)
    expect(MockEventSource.instances).toHaveLength(2)
    MockEventSource.instances[1]?.emit('task.created', {
      id: 'evt_new123',
      type: 'task.created',
      version: 1,
      occurred_at: '2026-10-10T00:00:00Z',
      actor: { type: 'user', id: 'usr_new123' },
      resource: { type: 'task', id: 'task_new123' },
      workspace_id: WORKSPACE_ID,
      data: {},
    })
    expect(seen).toHaveBeenCalledTimes(1)
    expect(seenJoined).toHaveBeenCalledTimes(1)
    stopNew()
    expect(MockEventSource.instances[1]?.closed).toBe(false)
    stopJoined()
    expect(MockEventSource.instances[1]?.closed).toBe(true)
  })

  it('keeps new media invalidation subscribed after both old refcount disposers run', async () => {
    stubEventSource()
    const stopOld = ensureWorkspaceMediaInvalidation(WORKSPACE_ID)
    const stopOldJoined = ensureWorkspaceMediaInvalidation(WORKSPACE_ID)
    clearStoredAuthState()
    const stopNew = ensureWorkspaceMediaInvalidation(WORKSPACE_ID)
    await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)

    stopOld()
    stopOldJoined()
    expect(MockEventSource.instances[1]?.closed).toBe(false)
    MockEventSource.instances[1]?.emit('media.asset_ready', {
      id: 'evt_media123',
      type: 'media.asset_ready',
      version: 1,
      occurred_at: '2026-10-10T00:00:00Z',
      actor: { type: 'user', id: 'usr_new123' },
      resource: { type: 'media_asset', id: ASSET_ID },
      workspace_id: WORKSPACE_ID,
      data: { asset_id: ASSET_ID },
    })
    await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    expect(mocks.request).toHaveBeenCalledTimes(2)
    stopNew()
    expect(MockEventSource.instances[1]?.closed).toBe(true)
  })

  it('aborts old batch reads and rejects late results without replacing new metadata', async () => {
    let resolveOld!: (value: {
      data: { items: ReturnType<typeof assetEnvelope>['data'][] }
    }) => void
    mocks.request.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve
        }),
    )
    const oldBatch = getWorkspaceMediaAssetMetadataBatch(WORKSPACE_ID, [ASSET_ID])
    const oldSignal = (mocks.request.mock.calls[0]?.[1] as { signal: AbortSignal }).signal

    clearStoredAuthState()
    expect(oldSignal.aborted).toBe(true)
    const fresh = await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    const rejected = expect(oldBatch).rejects.toMatchObject({ name: 'AbortError' })
    resolveOld({ data: { items: [{ ...assetEnvelope().data, width: 999 }] } })
    await rejected

    expect(await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)).toBe(fresh)
    expect(mocks.request).toHaveBeenCalledTimes(2)
  })

  it('drops media cache and closes streams on silent auth-state reset', async () => {
    stubEventSource()
    const first = await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    expect(first.asset_id).toBe(ASSET_ID)
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined)
    expect(MockEventSource.instances).toHaveLength(1)

    clearStoredAuthState()

    expect(MockEventSource.instances[0]?.closed).toBe(true)
    await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    expect(mocks.request).toHaveBeenCalledTimes(2)
    stop()
  })
})
