import { describe, expect, test, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { usePresenceFollow } from './usePresenceFollow'
import type { RemotePresencePeer } from './state'

function buildPeer(input: {
  clientId: number
  viewport?: { x: number; y: number; zoom: number } | undefined
  isSpotlighting?: boolean | undefined
  followingDeviceId?: number | null | undefined
}): RemotePresencePeer {
  return {
    clientId: input.clientId,
    state: {
      user: { id: `usr_${input.clientId}`, name: `Peer ${input.clientId}` },
      cursor: { position: { x: 0, y: 0 }, dragging: false },
      selection: { nodeIds: [], marquee: null },
      ...(input.viewport ? { viewport: input.viewport } : {}),
      ...(input.isSpotlighting === true ? { isSpotlighting: true as const } : {}),
      ...(input.followingDeviceId === undefined
        ? {}
        : { followingDeviceId: input.followingDeviceId }),
    },
  }
}

function mountFollow(input?: {
  peers?: RemotePresencePeer[] | undefined
  localClientId?: number | null | undefined
}) {
  const scope = effectScope()
  const peers = ref<RemotePresencePeer[]>(input?.peers ?? [])
  const localClientId = ref<number | null>(input?.localClientId ?? 41)
  const setViewport = vi.fn()
  const follow = scope.run(() => usePresenceFollow({ peers, localClientId, setViewport }))
  if (!follow) throw new Error('follow failed to mount')
  return { scope, follow, peers, localClientId, setViewport }
}

describe('presence follow session', () => {
  test('toggles follow and tweens the camera over 500ms at once', async () => {
    const mounted = mountFollow({
      peers: [buildPeer({ clientId: 7, viewport: { x: 10, y: 20, zoom: 2 } })],
    })
    mounted.follow.toggleFollow(7)
    expect(mounted.follow.followingClientId.value).toBe(7)
    expect(mounted.setViewport).toHaveBeenCalledWith({ x: 10, y: 20, zoom: 2 }, { duration: 500 })

    mounted.follow.toggleFollow(7)
    expect(mounted.follow.followingClientId.value).toBeNull()
    mounted.scope.stop()
  })

  test('tracks the target viewport while following', async () => {
    const mounted = mountFollow({
      peers: [buildPeer({ clientId: 7, viewport: { x: 0, y: 0, zoom: 1 } })],
    })
    mounted.follow.startFollow(7)
    mounted.setViewport.mockClear()

    mounted.peers.value = [buildPeer({ clientId: 7, viewport: { x: 30, y: 40, zoom: 1 } })]
    await Promise.resolve()
    expect(mounted.setViewport).toHaveBeenCalledWith({ x: 30, y: 40, zoom: 1 }, { duration: 500 })
    mounted.scope.stop()
  })

  test('stops following when the target leaves the room', async () => {
    const mounted = mountFollow({ peers: [buildPeer({ clientId: 7 })] })
    mounted.follow.startFollow(7)
    expect(mounted.follow.followingClientId.value).toBe(7)

    mounted.peers.value = []
    await Promise.resolve()
    expect(mounted.follow.followingClientId.value).toBeNull()
    mounted.scope.stop()
  })

  test('exits follow on Escape', () => {
    const mounted = mountFollow({ peers: [buildPeer({ clientId: 7 })] })
    mounted.follow.startFollow(7)
    expect(mounted.follow.handleEscape()).toBe(true)
    expect(mounted.follow.followingClientId.value).toBeNull()
    expect(mounted.follow.handleEscape()).toBe(false)
    mounted.scope.stop()
  })

  test('counts live followers pointing at the local device', () => {
    const mounted = mountFollow({
      peers: [
        buildPeer({ clientId: 7, followingDeviceId: 41 }),
        buildPeer({ clientId: 8, followingDeviceId: 42 }),
      ],
    })
    expect(mounted.follow.followerCount.value).toBe(1)
    mounted.scope.stop()
  })

  test('accepts a spotlight invite by following the presenter', async () => {
    const mounted = mountFollow({
      peers: [buildPeer({ clientId: 7, isSpotlighting: true, viewport: { x: 1, y: 2, zoom: 1 } })],
    })
    expect(mounted.follow.activeInvite.value?.clientId).toBe(7)
    mounted.follow.acceptInvite()
    expect(mounted.follow.followingClientId.value).toBe(7)
    expect(mounted.follow.activeInvite.value).toBeNull()
    mounted.scope.stop()
  })

  test('dismisses a spotlight invite without following', async () => {
    const mounted = mountFollow({
      peers: [buildPeer({ clientId: 7, isSpotlighting: true })],
    })
    mounted.follow.dismissInvite()
    expect(mounted.follow.followingClientId.value).toBeNull()
    expect(mounted.follow.activeInvite.value).toBeNull()
    mounted.scope.stop()
  })
})
