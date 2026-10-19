import { describe, expect, test } from 'vitest'
import {
  countSpotlightFollowers,
  findSpotlightInvite,
  isFollowingDevice,
  isSpotlightPresenter,
  resolveFollowViewport,
} from './follow'
import { FOLLOW_VIEWPORT_TWEEN_MS } from '../../../session/collaboration'
import type { RemotePresencePeer } from '../../../session/collaboration'

function buildPeer(overrides: Partial<RemotePresencePeer> = {}): RemotePresencePeer {
  return {
    clientId: 7,
    state: {
      user: { id: 'usr_bob', name: 'Bob' },
      cursor: { position: { x: 0, y: 0 }, dragging: false },
      selection: { nodeIds: [], marquee: null },
    },
    ...overrides,
  }
}

describe('follow viewport', () => {
  test('uses the 500ms TapNow tween for camera tracking', () => {
    expect(FOLLOW_VIEWPORT_TWEEN_MS).toBe(500)
  })

  test('resolves the tracked camera from the peer viewport', () => {
    expect(
      resolveFollowViewport(
        buildPeer({ state: { ...buildPeer().state, viewport: { x: 1, y: 2, zoom: 1.5 } } }),
      ),
    ).toEqual({ x: 1, y: 2, zoom: 1.5 })
  })

  test('ignores peers without a usable viewport', () => {
    expect(resolveFollowViewport(buildPeer())).toBeNull()
    expect(resolveFollowViewport(null)).toBeNull()
    expect(
      resolveFollowViewport(
        buildPeer({ state: { ...buildPeer().state, viewport: { x: 1, y: 2, zoom: 0 } } }),
      ),
    ).toBeNull()
  })
})

describe('spotlight followers', () => {
  test('counts peers whose followingDeviceId points at the presenter', () => {
    const peers = [
      buildPeer({ clientId: 7, state: { ...buildPeer().state, followingDeviceId: 41 } }),
      buildPeer({ clientId: 8, state: { ...buildPeer().state, followingDeviceId: 42 } }),
      buildPeer({ clientId: 9, state: { ...buildPeer().state, followingDeviceId: 41 } }),
    ]
    expect(countSpotlightFollowers(peers, 41)).toBe(2)
  })

  test('counts nobody without a local device id', () => {
    expect(countSpotlightFollowers([buildPeer()], null)).toBe(0)
    expect(countSpotlightFollowers([buildPeer()], undefined)).toBe(0)
  })

  test('detects active spotlight presenters', () => {
    expect(isSpotlightPresenter(buildPeer())).toBe(false)
    expect(
      isSpotlightPresenter(buildPeer({ state: { ...buildPeer().state, isSpotlighting: true } })),
    ).toBe(true)
  })
})

describe('spotlight invites', () => {
  test('surfaces the first active presenter', () => {
    const peers = [
      buildPeer({ clientId: 7, state: { ...buildPeer().state, isSpotlighting: true } }),
      buildPeer({ clientId: 8, state: { ...buildPeer().state, isSpotlighting: true } }),
    ]
    expect(findSpotlightInvite(peers, null, new Set())?.clientId).toBe(7)
  })

  test('skips the followed presenter and dismissed invites', () => {
    const peers = [
      buildPeer({ clientId: 7, state: { ...buildPeer().state, isSpotlighting: true } }),
      buildPeer({ clientId: 8, state: { ...buildPeer().state, isSpotlighting: true } }),
    ]
    expect(findSpotlightInvite(peers, 7, new Set())?.clientId).toBe(8)
    expect(findSpotlightInvite(peers, null, new Set([7]))?.clientId).toBe(8)
    expect(findSpotlightInvite(peers, null, new Set([7, 8]))).toBeNull()
  })

  test('stays quiet without an active presenter', () => {
    expect(findSpotlightInvite([buildPeer()], null, new Set())).toBeNull()
    expect(findSpotlightInvite([], null, new Set())).toBeNull()
  })

  test('matches the followed device exactly', () => {
    expect(isFollowingDevice(7, 7)).toBe(true)
    expect(isFollowingDevice(7, 8)).toBe(false)
    expect(isFollowingDevice(null, 7)).toBe(false)
  })
})
