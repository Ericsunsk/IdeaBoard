// Soft-lock bridge specs: local intent joins remote peers.

import { describe, expect, test } from 'vitest'
import { ref } from 'vue'
import { useSoftLocks } from './useSoftLocks'
import type { RemotePresencePeer } from '../../../session/collaboration'

function buildPeer(clientId: number, nodeId: string | null): RemotePresencePeer {
  return {
    clientId,
    state: {
      user: { id: `usr_${clientId}`, name: `User${clientId}` },
      selection: { nodeIds: nodeId ? [nodeId] : [] },
    },
  }
}

describe('soft lock bridge', () => {
  test('merges the local single selection with remote peers', () => {
    const bridge = useSoftLocks({
      peers: ref([buildPeer(9, 'node-1')]),
      selectedIds: ref(['node-2']),
      deviceId: ref(3),
      canWrite: ref(true),
    })
    expect(bridge.filterEditable(['node-1', 'node-2'])).toEqual(['node-2'])
    expect(bridge.filterEditable(['node-2', 'node-3'])).toEqual(['node-2', 'node-3'])
  })
})
