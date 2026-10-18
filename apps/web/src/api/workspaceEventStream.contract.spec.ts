// Issue #807: workspace event stream contract spec.
//
// Covers the versioned envelope parsing (frozen registry, evt_/ws_ shapes),
// transport suffix building (types filter + last_event_id cursor),
// per-workspace shared subscriptions with Last-Event-ID resume + reconnect,
// gap notices, cross-workspace isolation, media invalidation extraction, and
// the workspace task waiter (SSE refresh with polling fallback). Legacy
// `/user/events` behavior stays owned by `taskEvents.contract.spec.ts`.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../test/msw'
import {
  isWorkspaceEventType,
  normalizeWorkspaceEventTypesFilter,
  parseWorkspaceEventEnvelope,
  resetWorkspaceEventStreamsForTests,
  subscribeWorkspaceEvent,
  subscribeWorkspaceMediaInvalidation,
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
  it('freezes the 10-type version-1 registry', () => {
    expect(WORKSPACE_EVENT_TYPES).toHaveLength(10)
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
    ]) {
      expect(isWorkspaceEventType(type)).toBe(true)
    }
    expect(isWorkspaceEventType('task.updated')).toBe(false)
    expect(isWorkspaceEventType('audit.recorded')).toBe(false)
    expect(isWorkspaceEventType(null)).toBe(false)
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

  it('refreshes on the matching task envelope and resolves terminal', async () => {
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
