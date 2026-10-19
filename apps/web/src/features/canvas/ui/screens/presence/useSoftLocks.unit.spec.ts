// Soft-lock bridge specs: local intent joins remote peers, viewers abstain.

import { describe, expect, test } from 'vitest'
import { ref } from 'vue'
import { useSoftLocks } from './useSoftLocks'
import type { RemotePresencePeer } from '../../../session/collaboration'

function buildPeer(clientId: number, nodeId: string | null): RemotePresencePeer {
  return {
    clientId,
    state: {
      user: { id: `usr_${clientId}`, name: `User${clientId}` },
      cursor: null,
      selection: { nodeIds: nodeId ? [nodeId] : [], marquee: null },
    },
  }
}

describe('soft lock bridge', () => {
  test('merges the local single selection with remote peers', () => {
    const bridge = useSoftLocks({
      peers: ref([buildPeer(9, 'node-1')]),
      selectedIds: ref(['node-2']),
      deviceId: ref(3),
      userId: ref('usr_ada'),
      userName: ref('Ada'),
      canWrite: ref(true),
    })
    expect(bridge.nodeLocksById.value.get('node-1')?.deviceId).toBe(9)
    expect(bridge.nodeLocksById.value.get('node-2')?.deviceId).toBe(3)
    expect(bridge.filterEditable(['node-1', 'node-2'])).toEqual(['node-2'])
  })

  test('viewers claim nothing but still see remote locks', () => {
    const bridge = useSoftLocks({
      peers: ref([buildPeer(9, 'node-1')]),
      selectedIds: ref(['node-1']),
      deviceId: ref(3),
      userId: ref('usr_ada'),
      userName: ref('Ada'),
      canWrite: ref(false),
    })
    expect(bridge.nodeLocksById.value.get('node-1')?.deviceId).toBe(9)
  })
})
