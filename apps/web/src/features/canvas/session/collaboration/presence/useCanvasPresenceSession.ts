/** Owns the room transport, reconnect policy and local awareness broadcasts. */
import {
  onScopeDispose,
  ref,
  shallowRef,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'
import type * as Y from 'yjs'
import {
  EVENT_STREAM_RECONNECT_BACKOFF_FACTOR,
  EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS,
  EVENT_STREAM_RECONNECT_MAX_DELAY_MS,
} from '../../../../../core/config/timing'
import {
  PRESENCE_ACTIVITY_LEASE_MS,
  type PresenceActivity,
  isPermanentPresenceClose,
  parsePresenceAwarenessState,
  parseSelectionIds,
  resolvePresenceServerUrl,
  shouldConnectPresence,
  type PresenceUser,
  type RemotePresencePeer,
} from './state'
import { requestBoardRealtimeTicket } from '../../../../board/ports/canvasSession'
import {
  createYPresenceProvider,
  type PresenceCloseEvent,
  type PresenceProviderFactory,
  type PresenceProviderHandle,
} from './provider'

type CanvasPresenceDeps = {
  requestTicket?: ((workspaceId: string, boardId: string) => Promise<string>) | undefined
  createProvider?: PresenceProviderFactory | undefined
}

type CanvasPresenceSessionOptions = {
  boardId?: Ref<string | null | undefined> | undefined
  accessState?: Ref<string | null | undefined> | undefined
  workspaceId?: Ref<string | null | undefined> | undefined
  localUser?: Ref<PresenceUser | null> | undefined
  /** Whether this client may write (editors true, viewers false). */
  canWrite?: MaybeRefOrGetter<boolean> | undefined
  activity?: Ref<PresenceActivity | null>
  selectedIds: MaybeRefOrGetter<readonly string[]>
  deps?: CanvasPresenceDeps | undefined
}

type CanvasPresenceSnapshot = {
  boardId: string
  workspaceId: string
}

function readSnapshot(options: CanvasPresenceSessionOptions): CanvasPresenceSnapshot | null {
  const boardId = options.boardId?.value?.trim() ?? ''
  const workspaceId = options.workspaceId?.value?.trim() ?? ''
  const user = options.localUser?.value ?? null
  if (!shouldConnectPresence(boardId, options.accessState?.value) || !workspaceId || !user) {
    return null
  }
  return { boardId, workspaceId }
}

/** Broadcast identity using the canonical awareness fields. */
function buildBroadcastUser(user: PresenceUser): PresenceUser {
  return {
    id: user.id,
    name: user.name,
    ...(user.avatarUrl?.trim() ? { avatarUrl: user.avatarUrl.trim() } : {}),
  }
}

export function useCanvasPresenceSession(options: CanvasPresenceSessionOptions) {
  const remotePeers = shallowRef<RemotePresencePeer[]>([])
  const isDocSynced = ref(false)
  const localClientId = ref<number | null>(null)
  const sharedDoc = shallowRef<Y.Doc | null>(null)
  const requestTicket = options.deps?.requestTicket ?? requestBoardRealtimeTicket
  const createProvider = options.deps?.createProvider ?? createYPresenceProvider

  const activityLeases = new Map<number, { sequence: number; expiresAt: number }>()
  let activityExpiryTimer: ReturnType<typeof setTimeout> | undefined
  let handle: PresenceProviderHandle | null = null
  let detachProviderListeners: (() => void) | null = null
  let isConnectionClosed = false
  let connectToken = 0
  let isDisposed = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectDelayMs = EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS

  function isCurrent(token: number): boolean {
    return !isDisposed && token === connectToken
  }

  function clearReconnectTimer(): void {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function teardown(): void {
    clearReconnectTimer()
    clearTimeout(activityExpiryTimer)
    activityLeases.clear()
    if (options.activity) options.activity.value = null
    detachProviderListeners?.()
    detachProviderListeners = null
    handle?.dispose()
    handle = null
    isDocSynced.value = false
    localClientId.value = null
    sharedDoc.value = null
    remotePeers.value = []
  }

  function publishSnapshot(): void {
    const user = options.localUser?.value ?? null
    if (!handle || !user) return
    // Viewers broadcast selection-free: an empty selection keeps them out of
    // soft-lock contention (they cannot write).
    const selectionIds =
      toValue(options.canWrite ?? true) === false
        ? []
        : (parseSelectionIds(toValue(options.selectedIds)) ?? [])
    handle.provider.awareness.setLocalState({
      user: buildBroadcastUser(user),
      selection: { nodeIds: selectionIds },
      ...(options.activity?.value && toValue(options.canWrite ?? true) && isDocSynced.value
        ? { activity: options.activity.value }
        : {}),
    })
  }

  function readActivityLease(
    clientId: number,
    activity: PresenceActivity | null | undefined,
    now: number,
  ) {
    if (!activity) {
      activityLeases.delete(clientId)
      return null
    }
    let lease = activityLeases.get(clientId)
    if (activity.sequence !== lease?.sequence) {
      lease = { sequence: activity.sequence, expiresAt: now + PRESENCE_ACTIVITY_LEASE_MS }
      activityLeases.set(clientId, lease)
    }
    return lease ?? null
  }

  function onAwarenessChange(): void {
    const awareness = handle?.provider.awareness
    if (!awareness || isConnectionClosed) return
    const selfId = awareness.clientID
    clearTimeout(activityExpiryTimer)
    const now = performance.now()
    let nextExpiry = Infinity
    const present = new Set<number>()
    const peers: RemotePresencePeer[] = []
    for (const [clientId, raw] of awareness.getStates()) {
      if (clientId === selfId) continue
      const state = parsePresenceAwarenessState(raw)
      if (!state) continue
      present.add(clientId)
      const lease = readActivityLease(clientId, state.activity, now)
      const isEditing = lease !== null && lease.expiresAt > now
      if (isEditing) nextExpiry = Math.min(nextExpiry, lease!.expiresAt)
      peers.push({ clientId, state, ...(isEditing ? { isEditing: true } : {}) })
    }
    for (const clientId of activityLeases.keys()) {
      if (!present.has(clientId)) activityLeases.delete(clientId)
    }
    if (Number.isFinite(nextExpiry))
      activityExpiryTimer = setTimeout(onAwarenessChange, nextExpiry - now)
    remotePeers.value = peers
  }

  function onProviderStatus(status: string): void {
    if (status === 'disconnected') {
      isConnectionClosed = true
      isDocSynced.value = false
      clearTimeout(activityExpiryTimer)
      activityLeases.clear()
      remotePeers.value = []
    }
    if (status === 'connected') {
      reconnectDelayMs = EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS
      return
    }
  }

  function onConnectionClose(token: number): (event: PresenceCloseEvent) => void {
    return (event) => {
      isConnectionClosed = true
      isDocSynced.value = false
      if (options.activity) options.activity.value = null
      clearTimeout(activityExpiryTimer)
      activityLeases.clear()
      // Liveness-bound peers go stale with the socket on every close.
      remotePeers.value = []
      // Permanent sidecar refusals (4401 ticket, 4403 admit) must not
      // reconnect-loop: a later board/user change re-settles from the watcher.
      if (isPermanentPresenceClose(event?.code)) return
      scheduleReconnect(token)
    }
  }

  function scheduleReconnect(token: number): void {
    if (!isCurrent(token) || reconnectTimer !== null) return
    if (!readSnapshot(options)) return
    const delay = reconnectDelayMs
    reconnectDelayMs = Math.min(
      delay * EVENT_STREAM_RECONNECT_BACKOFF_FACTOR,
      EVENT_STREAM_RECONNECT_MAX_DELAY_MS,
    )
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      if (!isCurrent(token)) return
      const fresh = readSnapshot(options)
      if (!fresh) return
      void connectWithTicket(token, fresh)
    }, delay)
  }

  function attachProvider(token: number, next: PresenceProviderHandle): void {
    handle = next
    isConnectionClosed = false
    const awareness = next.provider.awareness
    localClientId.value = awareness.clientID
    sharedDoc.value = next.doc ?? null
    isDocSynced.value = next.provider.isSynced
    const detachSync = next.provider.onSync((isSynced) => {
      isDocSynced.value = isSynced
    })
    awareness.on('change', onAwarenessChange)
    const detachClose = next.provider.onConnectionClose(onConnectionClose(token))
    const detachStatus = next.provider.onStatus(onProviderStatus)
    detachProviderListeners = () => {
      awareness.off('change', onAwarenessChange)
      detachClose()
      detachStatus()
      detachSync()
    }
    onAwarenessChange()
  }

  async function connectWithTicket(token: number, snapshot: CanvasPresenceSnapshot): Promise<void> {
    let ticket: string
    try {
      ticket = await requestTicket(snapshot.workspaceId, snapshot.boardId)
    } catch {
      scheduleReconnect(token)
      return
    }
    if (!isCurrent(token)) return
    const serverBaseUrl = resolvePresenceServerUrl()
    if (!serverBaseUrl) return
    let next: PresenceProviderHandle
    try {
      next = await createProvider({ serverBaseUrl, room: snapshot.boardId, ticket })
    } catch {
      scheduleReconnect(token)
      return
    }
    if (!isCurrent(token) || !readSnapshot(options)) {
      next.dispose()
      return
    }
    handle?.dispose()
    detachProviderListeners?.()
    detachProviderListeners = null
    attachProvider(token, next)
    publishSnapshot()
  }

  function settle(): void {
    connectToken += 1
    const token = connectToken
    const snapshot = readSnapshot(options)
    teardown()
    reconnectDelayMs = EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS
    if (!snapshot) return
    void connectWithTicket(token, snapshot)
  }

  watch(
    () => [
      options.boardId?.value,
      options.accessState?.value,
      options.workspaceId?.value,
      options.localUser?.value,
      // Ticket capability is fixed when the socket is admitted. Awareness
      // alone cannot promote a read connection into a document writer.
      toValue(options.canWrite ?? true),
    ],
    settle,
    { immediate: true },
  )

  // Selection and write capability both affect lock claims.
  watch(() => [toValue(options.selectedIds), toValue(options.canWrite ?? true)], publishSnapshot, {
    deep: true,
  })

  watch(() => options.activity?.value, publishSnapshot, { flush: 'sync' })

  onScopeDispose(() => {
    isDisposed = true
    teardown()
  })

  return { remotePeers, isDocSynced, localClientId, sharedDoc }
}
