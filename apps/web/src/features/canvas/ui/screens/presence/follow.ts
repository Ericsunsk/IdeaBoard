// Follow & Spotlight collaboration helpers (pure, framework-free).
//
// Follow subscribes to one peer's broadcast `viewport` and drives the local
// camera with a 500ms tween (`FOLLOW_VIEWPORT_TWEEN_MS`). Spotlight lets a
// presenter broadcast `isSpotlighting: true`; every collaborator gets an
// invite banner, and the presenter reads a live follower count from peers
// whose `followingDeviceId` points at the presenter's device id.

import type { RemotePresencePeer } from '../../../session/collaboration'

export type FollowViewport = {
  x: number
  y: number
  zoom: number
}

/** Camera snapshot to track, or null when the peer shares no usable viewport. */
export function resolveFollowViewport(
  peer: RemotePresencePeer | null | undefined,
): FollowViewport | null {
  const viewport = peer?.state.viewport
  if (!viewport) return null
  const { x, y, zoom } = viewport
  if (!Number.isFinite(x) || !Number.isFinite(y) || !Number.isFinite(zoom)) return null
  if (zoom <= 0) return null
  return { x, y, zoom }
}

/** Whether the peer currently broadcasts as a spotlight presenter. */
export function isSpotlightPresenter(peer: RemotePresencePeer): boolean {
  return peer.state.isSpotlighting === true
}

/**
 * Live follower count for a presenter: peers whose `followingDeviceId`
 * points at `localClientId`. Null device ids never count anyone.
 */
export function countSpotlightFollowers(
  peers: readonly RemotePresencePeer[],
  localClientId: number | null | undefined,
): number {
  if (localClientId === null || localClientId === undefined) return 0
  let count = 0
  for (const peer of peers) {
    if (peer.state.followingDeviceId === localClientId) count += 1
  }
  return count
}

/**
 * First spotlight invite to surface: an active presenter the viewer neither
 * follows nor dismissed. Returns null when nothing needs a banner.
 */
export function findSpotlightInvite(
  peers: readonly RemotePresencePeer[],
  followingClientId: number | null | undefined,
  dismissedClientIds: ReadonlySet<number>,
): RemotePresencePeer | null {
  for (const peer of peers) {
    if (!isSpotlightPresenter(peer)) continue
    if (followingClientId !== null && followingClientId !== undefined) {
      if (peer.clientId === followingClientId) continue
    }
    if (dismissedClientIds.has(peer.clientId)) continue
    return peer
  }
  return null
}

/** Whether the viewer currently follows the given device. */
export function isFollowingDevice(
  followingClientId: number | null | undefined,
  clientId: number,
): boolean {
  return followingClientId === clientId
}
