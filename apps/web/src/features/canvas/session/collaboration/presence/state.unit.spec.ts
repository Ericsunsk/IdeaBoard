import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import {
  PRESENCE_CURSOR_BROADCAST_INTERVAL_MS,
  PRESENCE_DRAG_BROADCAST_INTERVAL_MS,
  buildPresenceServerUrl,
  createBroadcastThrottle,
  flowPointToScreenPoint,
  isPermanentPresenceClose,
  parsePresenceAwarenessState,
  presencePeerColor,
  resolvePresenceServerUrl,
  shouldConnectPresence,
  type PresenceAwarenessState,
} from './state'

function buildState(overrides: Partial<PresenceAwarenessState> = {}): PresenceAwarenessState {
  return {
    user: { id: 'usr_ada', name: 'Ada' },
    cursor: { position: { x: 10, y: 20 }, dragging: false },
    selection: { nodeIds: [], marquee: null },
    ...overrides,
  }
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('presence connect gate', () => {
  test('connects for a concrete board with workspace posture', () => {
    expect(shouldConnectPresence('board-1', 'workspace')).toBe(true)
  })

  test('never connects private boards', () => {
    expect(shouldConnectPresence('board-1', 'private')).toBe(false)
  })

  test('never connects drafts without a board id', () => {
    expect(shouldConnectPresence(null, 'workspace')).toBe(false)
    expect(shouldConnectPresence('', 'workspace')).toBe(false)
    expect(shouldConnectPresence('   ', 'workspace')).toBe(false)
  })

  test('never connects without a known posture', () => {
    expect(shouldConnectPresence('board-1', undefined)).toBe(false)
    expect(shouldConnectPresence('board-1', null)).toBe(false)
  })

  test('broadcasts with dual-speed cadence (cursor ~30ms, drag ~100ms)', () => {
    expect(PRESENCE_CURSOR_BROADCAST_INTERVAL_MS).toBe(30)
    expect(PRESENCE_DRAG_BROADCAST_INTERVAL_MS).toBe(100)
    expect(1000 / PRESENCE_CURSOR_BROADCAST_INTERVAL_MS).toBeCloseTo(33.3, 0)
  })
})

describe('presence broadcast throttle', () => {
  test('sends the first push immediately and coalesces bursts to one trailing send', () => {
    const published: PresenceAwarenessState[] = []
    const throttle = createBroadcastThrottle(66, (state) => published.push(state))

    for (let index = 0; index < 10; index += 1) {
      throttle.push(buildState({ cursor: { position: { x: index, y: 0 }, dragging: false } }))
    }
    expect(published).toHaveLength(1)
    expect(published[0]?.cursor?.position).toEqual({ x: 0, y: 0 })
    expect(published[0]?.cursor?.dragging).toBe(false)

    vi.advanceTimersByTime(66)
    expect(published).toHaveLength(2)
    expect(published[1]?.cursor?.position).toEqual({ x: 9, y: 0 })
    expect(published[1]?.cursor?.dragging).toBe(false)
  })

  test('flush sends the pending state immediately', () => {
    const published: PresenceAwarenessState[] = []
    const throttle = createBroadcastThrottle(66, (state) => published.push(state))

    throttle.push(buildState({ cursor: { position: { x: 1, y: 1 }, dragging: false } }))
    throttle.push(buildState({ cursor: { position: { x: 2, y: 2 }, dragging: true } }))
    throttle.flush()
    expect(published).toHaveLength(2)
    expect(published[1]?.cursor?.position).toEqual({ x: 2, y: 2 })
    expect(published[1]?.cursor?.dragging).toBe(true)
  })

  test('cancel drops the pending state', () => {
    const published: PresenceAwarenessState[] = []
    const throttle = createBroadcastThrottle(66, (state) => published.push(state))

    throttle.push(buildState())
    throttle.push(buildState({ cursor: { position: { x: 5, y: 5 }, dragging: false } }))
    throttle.cancel()
    vi.advanceTimersByTime(1000)
    expect(published).toHaveLength(1)
  })
})

describe('presence awareness parsing', () => {
  test('accepts a complete canonical remote state', () => {
    const canonicalFrame: PresenceAwarenessState = {
      user: { id: 'usr_ada', name: 'Ada', color: 'hsl(10 70% 45%)', avatarUrl: '/a.png' },
      cursor: { position: { x: 1.5, y: -2 }, dragging: true },
      selection: { nodeIds: ['node-1', 'node-2'], marquee: null },
      followingDeviceId: 7,
      isSpotlighting: true,
      viewport: { x: 1, y: 2, zoom: 1.5 },
    }
    const parsed = parsePresenceAwarenessState(canonicalFrame)
    expect(parsed?.user).toEqual({
      id: 'usr_ada',
      name: 'Ada',
      color: 'hsl(10 70% 45%)',
      avatarUrl: '/a.png',
    })
    expect(parsed?.cursor?.position).toEqual({ x: 1.5, y: -2 })
    expect(parsed?.cursor?.dragging).toBe(true)
    expect(parsed?.selection).toEqual({ nodeIds: ['node-1', 'node-2'], marquee: null })
    expect(parsed?.followingDeviceId).toBe(7)
    expect(parsed?.isSpotlighting).toBe(true)
    expect(parsed?.viewport).toEqual({ x: 1, y: 2, zoom: 1.5 })
  })

  test('rejects awareness states outside the canonical contract', () => {
    expect(
      parsePresenceAwarenessState({
        user: { id: 'usr_ada', name: 'Ada', avatar: '/a.png' },
        cursor: { x: 1.5, y: -2 },
        selection: ['node-1', 'node-2'],
      }),
    ).toBeNull()
  })

  test('accepts a null cursor as pointer-left', () => {
    expect(parsePresenceAwarenessState(buildState({ cursor: null }))?.cursor).toBeNull()
  })

  test('drops states without a usable user', () => {
    expect(parsePresenceAwarenessState(buildState({ user: { id: '', name: '' } }))).toBeNull()
    expect(
      parsePresenceAwarenessState({ cursor: null, selection: { nodeIds: [], marquee: null } }),
    ).toBeNull()
    expect(parsePresenceAwarenessState(null)).toBeNull()
  })

  test('drops states with a malformed cursor', () => {
    const malformed = { x: 'far', y: 1 } as unknown as { x: number; y: number }
    expect(
      parsePresenceAwarenessState(
        buildState({ cursor: malformed } as unknown as Partial<PresenceAwarenessState>),
      ),
    ).toBeNull()
  })

  test('caps selection ids and filters blanks', () => {
    const selection = Array.from({ length: 60 }, (_, index) => `node-${index}`)
    const parsed = parsePresenceAwarenessState(
      buildState({ selection: { nodeIds: [...selection, ''], marquee: null } }),
    )
    expect(parsed?.selection.nodeIds).toHaveLength(50)
    expect(parsed?.selection.nodeIds[0]).toBe('node-0')
  })

  test('rejects a missing required selection', () => {
    const parsed = parsePresenceAwarenessState({ ...buildState(), selection: undefined })
    expect(parsed).toBeNull()
  })

  test('reads follow, spotlight, and viewport collaboration fields', () => {
    const parsed = parsePresenceAwarenessState(
      buildState({ followingDeviceId: 9, isSpotlighting: true, viewport: { x: 3, y: 4, zoom: 2 } }),
    )
    expect(parsed?.followingDeviceId).toBe(9)
    expect(parsed?.isSpotlighting).toBe(true)
    expect(parsed?.viewport).toEqual({ x: 3, y: 4, zoom: 2 })
  })

  test('fails closed on malformed collaboration fields', () => {
    const parsed = parsePresenceAwarenessState(
      buildState({
        followingDeviceId: 'far' as unknown as number,
        isSpotlighting: 'yes' as unknown as boolean,
        viewport: { x: 1, y: 2, zoom: -1 },
      }),
    )
    expect(parsed?.followingDeviceId).toBeUndefined()
    expect(parsed?.isSpotlighting).toBeUndefined()
    expect(parsed?.viewport).toBeUndefined()
  })
})

describe('presence coordinates', () => {
  test('projects flow points through the viewport transform', () => {
    expect(flowPointToScreenPoint({ x: 10, y: 20 }, { x: 100, y: 50, zoom: 2 })).toEqual({
      x: 120,
      y: 90,
    })
  })

  test('derives a stable color per user id', () => {
    expect(presencePeerColor('usr_ada')).toBe(presencePeerColor('usr_ada'))
    expect(presencePeerColor('usr_ada')).toMatch(/^hsl\(\d+ 70% 45%\)$/)
    expect(presencePeerColor('usr_ada')).not.toBe(presencePeerColor('usr_bob'))
  })
})

describe('presence server url', () => {
  test('upgrades https origins to wss', () => {
    expect(buildPresenceServerUrl('https:', 'app.example')).toBe('wss://app.example/realtime')
    expect(buildPresenceServerUrl('http:', 'localhost:5173')).toBe('ws://localhost:5173/realtime')
  })

  test('resolves the same-origin realtime base in the browser', () => {
    expect(resolvePresenceServerUrl()?.endsWith('/realtime')).toBe(true)
  })
})

describe('presence permanent closes', () => {
  test.each([4400, 4401, 4403, 4499])('treats %i as permanent', (code) => {
    expect(isPermanentPresenceClose(code)).toBe(true)
  })

  test.each([4399, 4500, 1000, 1006, 1012])('retries %i', (code) => {
    expect(isPermanentPresenceClose(code)).toBe(false)
  })

  test('retries missing or malformed codes', () => {
    expect(isPermanentPresenceClose(null)).toBe(false)
    expect(isPermanentPresenceClose(undefined)).toBe(false)
    expect(isPermanentPresenceClose('4403')).toBe(false)
    expect(isPermanentPresenceClose(4401.5)).toBe(false)
  })
})
