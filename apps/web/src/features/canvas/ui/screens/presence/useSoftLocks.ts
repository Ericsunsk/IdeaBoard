// Soft-lock Vue bridge over presence peers + local selection.

import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  filterEditableNodeIds,
  resolveNodeLocks,
  toSoftLockPeer,
  type SoftLockOwner,
} from './locks'
import type { RemotePresencePeer } from '../../../session/collaboration'

type UseSoftLocksOptions = {
  peers: MaybeRefOrGetter<readonly RemotePresencePeer[]>
  selectedIds: MaybeRefOrGetter<readonly string[]>
  deviceId: MaybeRefOrGetter<number | null>
  userId: MaybeRefOrGetter<string>
  userName: MaybeRefOrGetter<string>
  canWrite: MaybeRefOrGetter<boolean>
}

export function useSoftLocks(options: UseSoftLocksOptions) {
  const allPeers = computed(() => {
    const deviceId = toValue(options.deviceId)
    const userId = toValue(options.userId)
    const userName = toValue(options.userName)
    const list = toValue(options.peers).map((peer) => toSoftLockPeer(peer))
    if (
      deviceId !== null &&
      Number.isInteger(deviceId) &&
      toValue(options.canWrite) &&
      userId.trim() &&
      userName.trim()
    ) {
      list.push({
        deviceId,
        userId,
        name: userName,
        selection: toValue(options.selectedIds),
        canWrite: true,
      })
    }
    return list
  })

  const nodeLocksById = computed<ReadonlyMap<string, SoftLockOwner>>(() =>
    resolveNodeLocks(allPeers.value),
  )

  function filterEditable(nodeIds: readonly string[]): string[] {
    const deviceId = toValue(options.deviceId)
    if (deviceId === null) return [...(nodeIds ?? [])]
    return filterEditableNodeIds(nodeIds, nodeLocksById.value, deviceId)
  }

  return { nodeLocksById, filterEditable }
}
