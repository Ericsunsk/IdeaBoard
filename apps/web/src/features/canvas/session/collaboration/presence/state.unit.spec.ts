import { describe, expect, test } from 'vitest'
import {
  buildPresenceServerUrl,
  isPermanentPresenceClose,
  parsePresenceAwarenessState,
  resolvePresenceServerUrl,
  shouldConnectPresence,
  type PresenceAwarenessState,
} from './state'

function buildState(overrides: Partial<PresenceAwarenessState> = {}): PresenceAwarenessState {
  return {
    user: { id: 'usr_ada', name: 'Ada' },
    selection: { nodeIds: [] },
    ...overrides,
  }
}

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
})

describe('presence awareness parsing', () => {
  test('accepts a complete canonical remote state', () => {
    const canonicalFrame: PresenceAwarenessState = {
      user: { id: 'usr_ada', name: 'Ada', avatarUrl: '/a.png' },
      selection: { nodeIds: ['node-1', 'node-2'] },
    }
    const parsed = parsePresenceAwarenessState(canonicalFrame)
    expect(parsed?.user).toEqual({
      id: 'usr_ada',
      name: 'Ada',
      avatarUrl: '/a.png',
    })
    expect(parsed?.selection).toEqual({ nodeIds: ['node-1', 'node-2'] })
  })

  test('rejects awareness states outside the canonical contract', () => {
    expect(
      parsePresenceAwarenessState({
        user: { id: 'usr_ada', name: 'Ada', avatar: '/a.png' },
        selection: ['node-1', 'node-2'],
      }),
    ).toBeNull()
  })

  test('drops states without a usable user', () => {
    expect(parsePresenceAwarenessState(buildState({ user: { id: '', name: '' } }))).toBeNull()
    expect(parsePresenceAwarenessState({ selection: { nodeIds: [] } })).toBeNull()
    expect(parsePresenceAwarenessState(null)).toBeNull()
  })

  test('caps selection ids and filters blanks', () => {
    const selection = Array.from({ length: 60 }, (_, index) => `node-${index}`)
    const parsed = parsePresenceAwarenessState(
      buildState({ selection: { nodeIds: [...selection, ''] } }),
    )
    expect(parsed?.selection.nodeIds).toHaveLength(50)
    expect(parsed?.selection.nodeIds[0]).toBe('node-0')
  })

  test('rejects a missing required selection', () => {
    const parsed = parsePresenceAwarenessState({ ...buildState(), selection: undefined })
    expect(parsed).toBeNull()
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
