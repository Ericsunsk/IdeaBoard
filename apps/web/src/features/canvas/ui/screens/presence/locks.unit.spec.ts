// Soft-lock specs: single-select intent, viewer exclusion, tie-break.

import { describe, expect, test } from 'vitest'
import {
  filterEditableNodeIds,
  getSingleSelectionLockIntent,
  resolveNodeLocks,
  toSoftLockPeer,
  type SoftLockPeer,
} from './locks'
import type { RemotePresencePeer } from '../../../session/collaboration'

function buildRemotePeer(overrides: Partial<RemotePresencePeer> = {}): RemotePresencePeer {
  return {
    clientId: 41,
    state: {
      user: { id: 'usr_ada', name: 'Ada' },
      selection: { nodeIds: ['node-1'] },
    },
    ...overrides,
  }
}

function buildPeer(overrides: Partial<SoftLockPeer> = {}): SoftLockPeer {
  return {
    deviceId: 41,
    selection: ['node-1'],
    canWrite: true,
    ...overrides,
  }
}

describe('soft lock selection intent', () => {
  test('claims only a single selected node', () => {
    expect(getSingleSelectionLockIntent(['node-1'])).toBe('node-1')
    expect(getSingleSelectionLockIntent([])).toBeNull()
    expect(getSingleSelectionLockIntent(['a', 'b'])).toBeNull()
    expect(getSingleSelectionLockIntent(['  '])).toBeNull()
  })
})

describe('soft lock resolution', () => {
  test('smaller deviceId strictly wins concurrent claims', () => {
    const locks = resolveNodeLocks([buildPeer({ deviceId: 9 }), buildPeer({ deviceId: 3 })])
    expect(locks.get('node-1')?.deviceId).toBe(3)
  })

  test('keeps the existing owner on ties and larger incoming ids', () => {
    const locks = resolveNodeLocks([
      buildPeer({ deviceId: 5 }),
      buildPeer({ deviceId: 5 }),
      buildPeer({ deviceId: 9 }),
    ])
    expect(locks.get('node-1')?.deviceId).toBe(5)
  })

  test('ignores empty selections and multi-select peers', () => {
    const locks = resolveNodeLocks([
      buildPeer({ deviceId: 1, selection: [] }),
      buildPeer({ deviceId: 2, selection: ['a', 'b'] }),
    ])
    expect(locks.has('node-9')).toBe(false)
    expect(locks.size).toBe(0)
  })

  test('gates edits per node for other owners only', () => {
    const locks = resolveNodeLocks([buildPeer({ deviceId: 7, selection: ['node-1'] })])
    expect(filterEditableNodeIds(['node-1', 'node-2'], locks, 7)).toEqual(['node-1', 'node-2'])
    expect(filterEditableNodeIds(['node-1', 'node-2'], locks, 8)).toEqual(['node-2'])
  })

  test('maps canonical awareness peers to contenders', () => {
    const peer = toSoftLockPeer(
      buildRemotePeer({
        clientId: 12,
        state: {
          user: { id: 'usr_bob', name: 'Bob' },
          selection: { nodeIds: ['node-5'] },
        },
      }),
    )
    expect(peer).toEqual({
      deviceId: 12,
      selection: ['node-5'],
    })
    const locks = resolveNodeLocks([peer])
    expect(locks.get('node-5')?.deviceId).toBe(12)
  })
})
