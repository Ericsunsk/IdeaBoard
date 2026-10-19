// Follow & Spotlight session for canvas presence.
//
// Owns the viewer's `followingDeviceId` (broadcast through
// `useCanvasPresence`) plus spotlight invite bookkeeping. While following,
// the target's broadcast viewport drives the local camera with a 500ms
// tween; clicking the avatar again or pressing Escape exits.

import { computed, ref, watch, type Ref } from 'vue'
import { FOLLOW_VIEWPORT_TWEEN_MS, type RemotePresencePeer } from './state'
import { countSpotlightFollowers, findSpotlightInvite, resolveFollowViewport } from './follow'

type PresenceViewportSetter = (
  viewport: { x: number; y: number; zoom: number },
  options?: { duration?: number },
) => unknown

type UsePresenceFollowOptions = {
  peers: Ref<RemotePresencePeer[]>
  localClientId: Ref<number | null>
  setViewport?: PresenceViewportSetter | undefined
  /** External following state; defaults to an internal ref when omitted. */
  followingClientId?: Ref<number | null> | undefined
}

function settleViewportPromise(result: unknown): void {
  if (result instanceof Promise) result.catch(() => {})
}

export function usePresenceFollow(options: UsePresenceFollowOptions) {
  const followingClientId = options.followingClientId ?? ref<number | null>(null)
  const dismissedSpotlightIds = ref<ReadonlySet<number>>(new Set())
  const setViewport = options.setViewport

  const followTarget = computed(
    () => options.peers.value.find((peer) => peer.clientId === followingClientId.value) ?? null,
  )

  const activeInvite = computed(() =>
    findSpotlightInvite(options.peers.value, followingClientId.value, dismissedSpotlightIds.value),
  )

  const followerCount = computed(() =>
    countSpotlightFollowers(options.peers.value, options.localClientId.value),
  )

  function applyViewport(viewport: { x: number; y: number; zoom: number }): void {
    if (!setViewport) return
    try {
      settleViewportPromise(setViewport(viewport, { duration: FOLLOW_VIEWPORT_TWEEN_MS }))
    } catch {
      // Camera tween is best-effort; the roster state stays authoritative.
    }
  }

  function trackTargetViewport(): void {
    const viewport = resolveFollowViewport(followTarget.value)
    if (viewport) applyViewport(viewport)
  }

  function startFollow(clientId: number): void {
    followingClientId.value = clientId
    trackTargetViewport()
  }

  function stopFollow(): void {
    followingClientId.value = null
  }

  function toggleFollow(clientId: number): void {
    if (followingClientId.value === clientId) stopFollow()
    else startFollow(clientId)
  }

  function acceptInvite(): void {
    const invite = activeInvite.value
    if (invite) startFollow(invite.clientId)
  }

  function dismissInvite(): void {
    const invite = activeInvite.value
    if (!invite) return
    dismissedSpotlightIds.value = new Set(dismissedSpotlightIds.value).add(invite.clientId)
  }

  /** Escape exits follow first; returns true when it consumed the key. */
  function handleEscape(): boolean {
    if (followingClientId.value !== null) {
      stopFollow()
      return true
    }
    return false
  }

  watch(() => followTarget.value?.state.viewport, trackTargetViewport, { deep: true })

  watch(
    () => options.peers.value,
    (peers) => {
      if (followingClientId.value !== null) {
        const isTargetGone = !peers.some((peer) => peer.clientId === followingClientId.value)
        if (isTargetGone) stopFollow()
      }
      const spotlighting = new Set<number>()
      for (const peer of peers) {
        if (peer.state.isSpotlighting === true) spotlighting.add(peer.clientId)
      }
      const dismissed = [...dismissedSpotlightIds.value].filter((id) => spotlighting.has(id))
      if (dismissed.length !== dismissedSpotlightIds.value.size) {
        dismissedSpotlightIds.value = new Set(dismissed)
      }
    },
  )

  return {
    followingClientId,
    dismissedSpotlightIds,
    followTarget,
    activeInvite,
    followerCount,
    startFollow,
    stopFollow,
    toggleFollow,
    acceptInvite,
    dismissInvite,
    handleEscape,
  }
}
