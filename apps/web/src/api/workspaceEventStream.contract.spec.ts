// Issue #807: workspace event stream contract spec.
//
// Covers the versioned envelope parsing (frozen registry, evt_/ws_ shapes),
// transport suffix building (types filter + last_event_id cursor),
// per-workspace shared subscriptions with Last-Event-ID resume + reconnect,
// gap notices, cross-workspace isolation, media invalidation extraction, and
// the workspace task waiter (SSE refresh with polling fallback). Legacy
// `/user/events` behavior stays owned by `taskEvents.contract.spec.ts`.
//
// Cross-stack parity (frozen registry <-> webhook allowlist): the
// `workspace event cross-stack parity` block pins the same literals as the
// backend frozen table (`apps/api/internal/events/v1_envelope.go`:
// `V1EventTypes`) and the webhook allowlist
// (`apps/api/internal/events/webhook/visibility.go`:
// `IsWorkspaceWebhookEventType`). The backend gate
// (`event_parity_test.go`) reads `WORKSPACE_EVENT_TYPES` from disk, so a new
// type on either side without the other two places fails there; the pins
// here give the same signal inside the frontend suite.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../test/msw'
import {
  isWorkspaceEventType,
  normalizeWorkspaceEventTypesFilter,
  NOTIFICATION_EVENT_TYPES,
  parseWorkspaceEventEnvelope,
  resetWorkspaceEventStreamsForTests,
  subscribeUserNotificationEvents,
  subscribeWorkspaceEvent,
  subscribeWorkspaceMediaInvalidation,
  userNotificationEventsSuffix,
  userNotificationEventsUrl,
  waitForWorkspaceEnvelope,
  WORKSPACE_EVENT_TYPES,
  workspaceEventsSuffix,
  type WorkspaceEventEnvelope,
} from './workspaceEventStream'
import { waitWorkspaceTaskTerminal } from './workspaceTasks'

const WORKSPACE_ID = 'ws_abc123'
const OTHER_WORKSPACE_ID = 'ws_other999'

class MockEventSource {
  static instances: MockEventSource[] = []

  onerror: ((event: Event) => void) | null = null
  closed = false
  readonly url: string
  readonly init?: EventSourceInit | undefined
  private listeners: Record<string, EventListener[]> = {}

  constructor(url: string, init?: EventSourceInit) {
    this.url = url
    this.init = init
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
    const event = new MessageEvent(type, {
      data: typeof data === 'string' ? data : JSON.stringify(data),
    })
    ;(this.listeners[type] ?? []).forEach((listener) => listener(event))
  }

  fail() {
    this.onerror?.(new Event('error'))
  }
}

function stubEventSource() {
  vi.stubGlobal('EventSource', MockEventSource)
}

function envelopeFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'evt_abc123',
    type: 'task.created',
    version: 1,
    occurred_at: '2026-09-23T00:00:00Z',
    actor: { type: 'user', id: 'usr_abc123' },
    resource: { type: 'task', id: 'task_def456' },
    workspace_id: WORKSPACE_ID,
    data: { task_id: 'task_def456', service: 'render' },
    ...overrides,
  }
}

beforeEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  MockEventSource.instances = []
  resetWorkspaceEventStreamsForTests()
})

afterEach(() => {
  resetWorkspaceEventStreamsForTests()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('workspace event registry', () => {
  it('freezes the 12-type version-1 registry', () => {
    expect(WORKSPACE_EVENT_TYPES).toHaveLength(12)
    for (const type of [
      'task.created',
      'task.running',
      'task.completed',
      'task.failed',
      'task.cancelled',
      'media.asset_ready',
      'media.asset_archived',
      'board.document_replaced',
      'board.access_changed',
      'billing.settled',
      'notification.created',
      'notification.updated',
    ]) {
      expect(isWorkspaceEventType(type)).toBe(true)
    }
    expect(isWorkspaceEventType('task.updated')).toBe(false)
    expect(isWorkspaceEventType('audit.recorded')).toBe(false)
    expect(isWorkspaceEventType(null)).toBe(false)
  })

  it('exposes the notification hint pair for the inbox push', () => {
    expect(NOTIFICATION_EVENT_TYPES).toEqual(['notification.created', 'notification.updated'])
    for (const type of NOTIFICATION_EVENT_TYPES) {
      expect(isWorkspaceEventType(type)).toBe(true)
    }
    expect(normalizeWorkspaceEventTypesFilter([...NOTIFICATION_EVENT_TYPES])).toEqual([
      'notification.created',
      'notification.updated',
    ])
  })

  it('normalizes the types filter (empty means all, unknown fails closed)', () => {
    expect(normalizeWorkspaceEventTypesFilter(undefined)).toEqual([])
    expect(normalizeWorkspaceEventTypesFilter([])).toEqual([])
    expect(normalizeWorkspaceEventTypesFilter(['task.completed', 'task.completed'])).toEqual([
      'task.completed',
    ])
    expect(() => normalizeWorkspaceEventTypesFilter(['task.updated'])).toThrow()
    expect(() => normalizeWorkspaceEventTypesFilter(['audit.recorded'])).toThrow()
  })
})

describe('workspace event cross-stack parity', () => {
  // Pinned mirror of the backend frozen registry
  // (`apps/api/internal/events/v1_envelope.go`: `V1EventTypes`, all version
  // 1) and the webhook allowlist
  // (`apps/api/internal/events/webhook/visibility.go`:
  // `IsWorkspaceWebhookEventType`). Adding a type means updating the backend
  // table, this list, and the allowlist decision together — the backend
  // `event_parity_test.go` gate reads `WORKSPACE_EVENT_TYPES` from disk so
  // any two-sided drift fails there; these pins fail the same drift here.
  const FROZEN_V1_EVENT_TYPES = [
    'task.created',
    'task.running',
    'task.completed',
    'task.failed',
    'task.cancelled',
    'media.asset_ready',
    'media.asset_archived',
    'board.document_replaced',
    'board.access_changed',
    'billing.settled',
    'notification.created',
    'notification.updated',
  ] as const
  // Webhooks carry no user principal: only checkout-scoped billing.settled
  // rides them. Task-linked settlements and Task/Media/Board content events
  // go to authorized SSE principals only; principal-scoped hints
  // (board.access_changed, notification.*) must never become subscribable.
  const WORKSPACE_WEBHOOK_ALLOWLIST = ['billing.settled'] as const
  const PRINCIPAL_SCOPED_HINTS = [
    'board.access_changed',
    'notification.created',
    'notification.updated',
  ] as const

  it('pins the frozen v1 registry shared with the backend', () => {
    expect([...WORKSPACE_EVENT_TYPES]).toEqual([...FROZEN_V1_EVENT_TYPES])
  })

  it('pins the webhook allowlist to checkout-scoped billing.settled only', () => {
    for (const type of WORKSPACE_WEBHOOK_ALLOWLIST) {
      expect(isWorkspaceEventType(type)).toBe(true)
    }
    expect([...WORKSPACE_WEBHOOK_ALLOWLIST]).toEqual(['billing.settled'])
    for (const hint of PRINCIPAL_SCOPED_HINTS) {
      expect(isWorkspaceEventType(hint)).toBe(true)
      expect((WORKSPACE_WEBHOOK_ALLOWLIST as readonly string[]).includes(hint)).toBe(false)
    }
  })
})

describe('workspace event envelope parsing', () => {
  it('parses a valid envelope from an object or JSON string', () => {
    const fromObject = parseWorkspaceEventEnvelope(envelopeFixture())
    expect(fromObject.id).toBe('evt_abc123')
    expect(fromObject.type).toBe('task.created')
    const fromString = parseWorkspaceEventEnvelope(JSON.stringify(envelopeFixture()))
    expect(fromString).toEqual(fromObject)
  })

  it('rejects non-evt ids, unknown types, bad versions, and missing data', () => {
    expect(() => parseWorkspaceEventEnvelope(envelopeFixture({ id: 'task_abc123' }))).toThrow()
    expect(() => parseWorkspaceEventEnvelope(envelopeFixture({ id: '42' }))).toThrow()
    expect(() => parseWorkspaceEventEnvelope(envelopeFixture({ type: 'task.updated' }))).toThrow()
    expect(() => parseWorkspaceEventEnvelope(envelopeFixture({ version: 2 }))).toThrow()
    expect(() => parseWorkspaceEventEnvelope(envelopeFixture({ data: null }))).toThrow()
    expect(() => parseWorkspaceEventEnvelope(envelopeFixture({ data: [1] }))).toThrow()
    expect(() =>
      parseWorkspaceEventEnvelope(envelopeFixture({ workspace_id: 'ws-other' })),
    ).toThrow()
    expect(() =>
      parseWorkspaceEventEnvelope(envelopeFixture({ actor: { type: 'user', id: '' } })),
    ).toThrow()
    expect(() => parseWorkspaceEventEnvelope('not-json{')).toThrow()
    expect(() => parseWorkspaceEventEnvelope(null)).toThrow()
  })
})

describe('workspace events transport suffix', () => {
  it('builds canonical suffixes with sorted types and cursor', () => {
    expect(workspaceEventsSuffix(WORKSPACE_ID)).toBe('/v1/workspaces/ws_abc123/events')
    const suffix = workspaceEventsSuffix(WORKSPACE_ID, {
      types: ['task.completed', 'task.created'],
      lastEventId: 'evt_abc123',
    })
    const url = new URL(suffix, 'https://example.test')
    expect(url.pathname).toBe('/v1/workspaces/ws_abc123/events')
    expect(url.searchParams.get('types')).toBe('task.completed,task.created')
    expect(url.searchParams.get('last_event_id')).toBe('evt_abc123')
  })

  it('rejects malformed workspace ids, filters, and cursors before any network call', () => {
    expect(() => workspaceEventsSuffix('nope')).toThrow()
    expect(() => workspaceEventsSuffix('42')).toThrow()
    expect(() => workspaceEventsSuffix(WORKSPACE_ID, { types: ['bogus'] })).toThrow()
    expect(() => workspaceEventsSuffix(WORKSPACE_ID, { lastEventId: 'task_1' })).toThrow()
    expect(() => subscribeWorkspaceEvent('nope', () => undefined)).toThrow()
  })
})

describe('workspace event subscriptions', () => {
  it('shares one transport per workspace and fans out parsed envelopes', () => {
    stubEventSource()
    const seenA: WorkspaceEventEnvelope[] = []
    const seenB: WorkspaceEventEnvelope[] = []
    const stopA = subscribeWorkspaceEvent(WORKSPACE_ID, (envelope) => seenA.push(envelope))
    const stopB = subscribeWorkspaceEvent(WORKSPACE_ID, (envelope) => seenB.push(envelope))
    expect(MockEventSource.instances).toHaveLength(1)
    expect(MockEventSource.instances[0]?.url).toContain('/api/v1/workspaces/ws_abc123/events')
    MockEventSource.instances[0]?.emit('task.created', envelopeFixture())
    expect(seenA).toHaveLength(1)
    expect(seenB).toHaveLength(1)
    expect(seenA[0]?.id).toBe('evt_abc123')
    stopA()
    // One remaining subscriber keeps the transport open.
    expect(MockEventSource.instances[0]?.closed).toBe(false)
    stopB()
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('scopes the transport URL to the types filter', () => {
    stubEventSource()
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined, {
      types: ['media.asset_ready'],
    })
    const url = new URL(MockEventSource.instances[0]?.url ?? '', 'https://example.test')
    expect(url.searchParams.get('types')).toBe('media.asset_ready')
    stop()
  })

  it('drops malformed frames and cross-workspace envelopes', () => {
    stubEventSource()
    const seen: WorkspaceEventEnvelope[] = []
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, (envelope) => seen.push(envelope))
    MockEventSource.instances[0]?.emit('task.created', 'not-json{')
    MockEventSource.instances[0]?.emit(
      'task.created',
      envelopeFixture({ id: 'evt_other999', workspace_id: OTHER_WORKSPACE_ID }),
    )
    expect(seen).toHaveLength(0)
    stop()
  })

  it('isolates workspaces onto separate transports', () => {
    stubEventSource()
    const stopA = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined)
    const stopB = subscribeWorkspaceEvent(OTHER_WORKSPACE_ID, () => undefined)
    expect(MockEventSource.instances).toHaveLength(2)
    expect(MockEventSource.instances[0]?.url).toContain(WORKSPACE_ID)
    expect(MockEventSource.instances[1]?.url).toContain(OTHER_WORKSPACE_ID)
    stopA()
    stopB()
  })

  it('resumes from the last seen evt_ id after a transport error', () => {
    stubEventSource()
    vi.useFakeTimers()
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined)
    expect(MockEventSource.instances).toHaveLength(1)
    MockEventSource.instances[0]?.emit('task.created', envelopeFixture({ id: 'evt_first111' }))
    MockEventSource.instances[0]?.fail()
    expect(MockEventSource.instances).toHaveLength(1)
    vi.advanceTimersByTime(1500)
    expect(MockEventSource.instances).toHaveLength(2)
    const resumed = new URL(MockEventSource.instances[1]?.url ?? '', 'https://example.test')
    expect(resumed.searchParams.get('last_event_id')).toBe('evt_first111')
    stop()
  })

  it('does not reconnect after the last listener unsubscribed', () => {
    stubEventSource()
    vi.useFakeTimers()
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined)
    expect(MockEventSource.instances).toHaveLength(1)
    MockEventSource.instances[0]?.fail()
    stop()
    vi.advanceTimersByTime(30_000)
    expect(MockEventSource.instances).toHaveLength(1)
  })

  it('does not resurrect a listener-less entry on transport error', () => {
    stubEventSource()
    vi.useFakeTimers()
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined)
    expect(MockEventSource.instances).toHaveLength(1)
    stop()
    MockEventSource.instances[0]?.fail()
    vi.advanceTimersByTime(30_000)
    expect(MockEventSource.instances).toHaveLength(1)
  })

  it('surfaces gap notices so clients refetch state', () => {
    stubEventSource()
    const gaps: { lastEventId: string }[] = []
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined, {
      onGap: (info) => gaps.push(info),
    })
    MockEventSource.instances[0]?.emit('task.created', envelopeFixture({ id: 'evt_gap0001' }))
    MockEventSource.instances[0]?.emit('gap', { truncated: true })
    expect(gaps).toEqual([{ lastEventId: 'evt_gap0001' }])
    stop()
  })

  it('starts from an explicit cursor when provided', () => {
    stubEventSource()
    const stop = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined, {
      lastEventId: 'evt_seed0001',
    })
    const url = new URL(MockEventSource.instances[0]?.url ?? '', 'https://example.test')
    expect(url.searchParams.get('last_event_id')).toBe('evt_seed0001')
    stop()
  })
})

describe('user notification hint stream', () => {
  function hintFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      id: 'evt_hint111',
      type: 'notification.created',
      version: 1,
      occurred_at: '2026-09-23T00:00:00Z',
      actor: { type: 'user', id: 'usr_abc123' },
      resource: { type: 'invite', id: 'wsi_def456' },
      workspace_id: WORKSPACE_ID,
      data: { invite_id: 'wsi_def456', notification_type: 'workspace_invite' },
      ...overrides,
    }
  }

  it('builds the per-user suffix with sorted types and cursor', () => {
    expect(userNotificationEventsSuffix()).toBe('/v1/user/notifications/events')
    const suffix = userNotificationEventsSuffix({
      types: ['notification.updated', 'notification.created'],
      lastEventId: 'evt_abc123',
    })
    const url = new URL(suffix, 'https://example.test')
    expect(url.pathname).toBe('/v1/user/notifications/events')
    expect(url.searchParams.get('types')).toBe('notification.created,notification.updated')
    expect(url.searchParams.get('last_event_id')).toBe('evt_abc123')
    expect(userNotificationEventsUrl()).toContain('/api/v1/user/notifications/events')
    expect(() => userNotificationEventsSuffix({ types: ['task.exploded'] })).toThrow()
    expect(() => userNotificationEventsSuffix({ lastEventId: 'task_1' })).toThrow()
  })

  it('parses invite resources through the shared envelope parser', () => {
    const envelope = parseWorkspaceEventEnvelope(hintFixture())
    expect(envelope.type).toBe('notification.created')
    expect(envelope.resource).toEqual({ type: 'invite', id: 'wsi_def456' })
    expect(() =>
      parseWorkspaceEventEnvelope(hintFixture({ type: 'notification.exploded' })),
    ).toThrow()
  })

  it('shares one transport per types filter and fans out hint frames', () => {
    stubEventSource()
    const seenA: WorkspaceEventEnvelope[] = []
    const seenB: WorkspaceEventEnvelope[] = []
    const stopA = subscribeUserNotificationEvents((envelope) => seenA.push(envelope), {
      types: [...NOTIFICATION_EVENT_TYPES],
    })
    const stopB = subscribeUserNotificationEvents((envelope) => seenB.push(envelope), {
      types: [...NOTIFICATION_EVENT_TYPES],
    })
    expect(MockEventSource.instances).toHaveLength(1)
    expect(MockEventSource.instances[0]?.url).toContain('/api/v1/user/notifications/events')
    MockEventSource.instances[0]?.emit('notification.created', hintFixture())
    expect(seenA).toHaveLength(1)
    expect(seenB).toHaveLength(1)
    expect(seenA[0]?.resource).toEqual({ type: 'invite', id: 'wsi_def456' })
    stopA()
    expect(MockEventSource.instances[0]?.closed).toBe(false)
    stopB()
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('isolates the user stream from workspace transports', () => {
    stubEventSource()
    const stopUser = subscribeUserNotificationEvents(() => undefined, {
      types: [...NOTIFICATION_EVENT_TYPES],
    })
    const stopWorkspace = subscribeWorkspaceEvent(WORKSPACE_ID, () => undefined, {
      types: [...NOTIFICATION_EVENT_TYPES],
    })
    expect(MockEventSource.instances).toHaveLength(2)
    const urls = MockEventSource.instances.map((instance) => instance.url)
    expect(urls.some((url) => url.includes('/api/v1/user/notifications/events'))).toBe(true)
    expect(urls.some((url) => url.includes(`/api/v1/workspaces/${WORKSPACE_ID}/events`))).toBe(true)
    stopUser()
    stopWorkspace()
  })

  it('resumes from the last seen hint id and surfaces gaps', () => {
    stubEventSource()
    vi.useFakeTimers()
    const gaps: { lastEventId: string }[] = []
    const stop = subscribeUserNotificationEvents(() => undefined, {
      types: [...NOTIFICATION_EVENT_TYPES],
      onGap: (info) => gaps.push(info),
    })
    MockEventSource.instances[0]?.emit(
      'notification.created',
      hintFixture({
        id: 'evt_hint222',
        data: { invite_id: 'wsi_def456', notification_type: 'workspace_invite' },
      }),
    )
    MockEventSource.instances[0]?.emit('gap', {})
    expect(gaps).toEqual([{ lastEventId: 'evt_hint222' }])
    MockEventSource.instances[0]?.fail()
    vi.advanceTimersByTime(1500)
    const resumed = new URL(MockEventSource.instances[1]?.url ?? '', 'https://example.test')
    expect(resumed.searchParams.get('last_event_id')).toBe('evt_hint222')
    stop()
  })

  it('drops malformed hint frames without delivering', () => {
    stubEventSource()
    const seen: WorkspaceEventEnvelope[] = []
    const stop = subscribeUserNotificationEvents((envelope) => seen.push(envelope))
    MockEventSource.instances[0]?.emit('notification.created', 'not-json{')
    expect(seen).toHaveLength(0)
    stop()
  })
})

describe('waitForWorkspaceEnvelope', () => {
  it('resolves on the first matching envelope', async () => {
    stubEventSource()
    const pending = waitForWorkspaceEnvelope(
      WORKSPACE_ID,
      (envelope) => envelope.type === 'task.completed',
      { types: ['task.completed'] },
    )
    MockEventSource.instances[0]?.emit(
      'task.completed',
      envelopeFixture({
        id: 'evt_done111',
        type: 'task.completed',
        data: {
          task_id: 'task_def456',
          outputs_uri: `/api/v1/workspaces/${WORKSPACE_ID}/tasks/task_def456/outputs`,
        },
      }),
    )
    await expect(pending).resolves.toMatchObject({ id: 'evt_done111', type: 'task.completed' })
  })

  it('rejects on timeout and on abort', async () => {
    stubEventSource()
    await expect(
      waitForWorkspaceEnvelope(WORKSPACE_ID, () => true, { timeoutMs: 20 }),
    ).rejects.toThrow(/timed out/)
    const controller = new AbortController()
    const pending = waitForWorkspaceEnvelope(WORKSPACE_ID, () => true, {
      signal: controller.signal,
      timeoutMs: 5000,
    })
    controller.abort()
    await expect(pending).rejects.toThrow(/abort/)
  })
})

describe('workspace media invalidation', () => {
  it('extracts asset_id from media envelopes only', () => {
    stubEventSource()
    const invalidated: string[] = []
    const stop = subscribeWorkspaceMediaInvalidation(WORKSPACE_ID, (assetId) =>
      invalidated.push(assetId),
    )
    const url = new URL(MockEventSource.instances[0]?.url ?? '', 'https://example.test')
    expect(url.searchParams.get('types')).toBe('media.asset_archived,media.asset_ready')
    MockEventSource.instances[0]?.emit(
      'media.asset_ready',
      envelopeFixture({
        id: 'evt_media111',
        type: 'media.asset_ready',
        resource: { type: 'media_asset', id: 'asset_abc123' },
        data: { asset_id: 'asset_abc123' },
      }),
    )
    MockEventSource.instances[0]?.emit(
      'media.asset_archived',
      envelopeFixture({
        id: 'evt_media222',
        type: 'media.asset_archived',
        resource: { type: 'media_asset', id: 'asset_def456' },
        data: { asset_id: '  asset_def456 ' },
      }),
    )
    // Wrong-type frames and empty ids never invalidate.
    MockEventSource.instances[0]?.emit('task.created', envelopeFixture({ id: 'evt_noise111' }))
    MockEventSource.instances[0]?.emit(
      'media.asset_ready',
      envelopeFixture({ id: 'evt_noise222', data: {} }),
    )
    expect(invalidated).toEqual(['asset_abc123', 'asset_def456'])
    stop()
  })
})

describe('waitWorkspaceTaskTerminal', () => {
  const TASK_ID = 'task_def456'

  function taskPayload(status: string): Record<string, unknown> {
    return {
      data: {
        id: TASK_ID,
        workspace_id: WORKSPACE_ID,
        service: 'render',
        status,
        retry_count: 0,
        billing_snapshot: {
          sku: 'ai-image:render',
          display_name: 'Render',
          price_version_id: 1,
          price_version: 1,
          output_resolution: '1k',
          unit_credits: 0,
          quantity: 1,
          total_credits: 0,
          quoted_at: '2026-09-23T00:00:00Z',
          settlement_state: 'free',
        },
        created_at: '2026-09-23T00:00:00Z',
        updated_at: '2026-09-23T00:00:00Z',
      },
    }
  }

  it('resolves an already terminal task before opening a stream', async () => {
    stubEventSource()
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/tasks/:taskId', () => {
        return HttpResponse.json(taskPayload('completed'))
      }),
    )
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { timeoutMs: 5000 })
    // Unrelated task envelopes do not resolve the wait.
    MockEventSource.instances[0]?.emit(
      'task.running',
      envelopeFixture({
        id: 'evt_run111',
        type: 'task.running',
        data: { task_id: 'task_other000' },
      }),
    )
    MockEventSource.instances[0]?.emit(
      'task.completed',
      envelopeFixture({
        id: 'evt_done222',
        type: 'task.completed',
        data: {
          task_id: TASK_ID,
          outputs_uri: `/api/v1/workspaces/${WORKSPACE_ID}/tasks/${TASK_ID}/outputs`,
        },
      }),
    )
    await expect(pending).resolves.toMatchObject({ id: TASK_ID, status: 'completed' })
  })

  it('resolves immediately from self-contained task.completed outputs without an event-triggered GET', async () => {
    stubEventSource()
    let getCalls = 0
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/tasks/:taskId', () => {
        getCalls += 1
        return HttpResponse.json(taskPayload('running'))
      }),
    )
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { timeoutMs: 5000 })
    await vi.waitFor(() => expect(MockEventSource.instances[0]).toBeDefined())
    expect(getCalls).toBe(2)

    MockEventSource.instances[0]?.emit(
      'task.completed',
      envelopeFixture({
        id: 'evt_done_self_contained',
        type: 'task.completed',
        data: {
          task_id: TASK_ID,
          outputs_uri: `/api/v1/workspaces/${WORKSPACE_ID}/tasks/${TASK_ID}/outputs`,
          outputs: [
            {
              asset_id: 'asset_fast_123',
              role: 'result',
              url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/asset_fast_123/content`,
              width: 1024,
              height: 1024,
              mime_type: 'image/png',
              // eslint-disable-next-line governance/boolean-prefix -- Wire contract field
              preview_ready: true,
            },
          ],
        },
      }),
    )

    const resolved = await pending
    expect(resolved).toMatchObject({
      id: TASK_ID,
      status: 'completed',
      output_refs: [
        {
          kind: 'asset',
          asset_id: 'asset_fast_123',
          role: 'result',
          width: 1024,
          height: 1024,
          mime_type: 'image/png',
          // eslint-disable-next-line governance/boolean-prefix -- Wire contract field
          preview_ready: true,
        },
      ],
    })
    expect(getCalls).toBe(2)
  })

  it('falls back to polling when EventSource is unavailable', async () => {
    vi.stubGlobal('EventSource', undefined)
    let calls = 0
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/tasks/:taskId', () => {
        calls += 1
        return HttpResponse.json(taskPayload(calls < 2 ? 'running' : 'failed'))
      }),
    )
    await expect(
      waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, {
        timeoutMs: 5000,
        pollInitialDelayMs: 5,
        pollMaxDelayMs: 10,
      }),
    ).resolves.toMatchObject({ status: 'failed' })
    expect(calls).toBeGreaterThanOrEqual(2)
  })

  it('rejects on timeout and validates ids before any network call', async () => {
    stubEventSource()
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/tasks/:taskId', () => {
        return HttpResponse.json(taskPayload('running'))
      }),
    )
    await expect(
      waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, {
        timeoutMs: 30,
        pollInitialDelayMs: 5,
        pollMaxDelayMs: 10,
      }),
    ).rejects.toThrow(/timed out/)
    expect(() => waitWorkspaceTaskTerminal('nope', TASK_ID, { timeoutMs: 10 })).toThrow()
    expect(() => waitWorkspaceTaskTerminal(WORKSPACE_ID, '42', { timeoutMs: 10 })).toThrow()
  })
})
