// Soft node locks (TapNow §5, pure client determination).
//
// - Selecting a single node broadcasts `awareness.selection.nodeIds = [nodeId]`;
//   multi-select or empty selection claims nothing.
// - Every client recomputes locks locally from awareness states: no central
//   lock manager. When two peers claim the same node, the smaller `deviceId`
//   strictly wins (`incoming < existing` claims, otherwise yields).
// - Viewers never participate: entries with `canWrite === false` are
//   ignored (viewers broadcast an empty selection, see
//   `useCanvasPresenceSession`).

import type { RemotePresencePeer } from '../../../session/collaboration'

export type SoftLockPeer = {
  deviceId: number
  selection: readonly string[]
  canWrite?: boolean | undefined
}

export type SoftLockOwner = {
  deviceId: number
}

/** Single-selection lock intent: exactly one non-blank id claims, else null. */
export function getSingleSelectionLockIntent(selection: readonly string[]): string | null {
  const ids: string[] = []
  for (const raw of selection ?? []) {
    if (typeof raw !== 'string') continue
    const id = raw.trim()
    if (id) ids.push(id)
  }
  return ids.length === 1 && ids[0] ? (ids[0] as string) : null
}

function isWritable(peer: SoftLockPeer): boolean {
  if (peer.canWrite === false) return false
  return Number.isInteger(peer.deviceId)
}

/** Map one awareness peer to its lock contender (viewer peers stay mappable). */
export function toSoftLockPeer(peer: RemotePresencePeer): SoftLockPeer {
  return {
    deviceId: peer.clientId,
    selection: peer.state.selection.nodeIds,
  }
}

/**
 * Recompute `nodeId -> owner` from awareness selections. Deterministic:
 * the smallest deviceId wins per node. Viewers (`canWrite: false`) and
 * multi-select peers contribute nothing.
 */
export function resolveNodeLocks(peers: readonly SoftLockPeer[]): Map<string, SoftLockOwner> {
  const locks = new Map<string, SoftLockOwner>()
  for (const peer of peers ?? []) {
    if (!peer || !isWritable(peer)) continue
    const intent = getSingleSelectionLockIntent(peer.selection)
    if (!intent) continue
    const existing = locks.get(intent)
    if (existing && peer.deviceId >= existing.deviceId) continue
    locks.set(intent, {
      deviceId: peer.deviceId,
    })
  }
  return locks
}

/** Keep only the ids the local device may edit (drag cohort filter). */
export function filterEditableNodeIds(
  nodeIds: readonly string[],
  locks: ReadonlyMap<string, SoftLockOwner>,
  selfDeviceId: number,
): string[] {
  return nodeIds.filter((nodeId) => {
    const owner = locks.get(nodeId)
    return !owner || owner.deviceId === selfDeviceId
  })
}
