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
  PRESENCE_CURSOR_BROADCAST_INTERVAL_MS,
  PRESENCE_DRAG_BROADCAST_INTERVAL_MS,
  PRESENCE_MAX_SELECTION_IDS,
  createBroadcastThrottle,
  isPermanentPresenceClose,
  isPresenceViewportState,
  parsePresenceAwarenessState,
  presencePeerColor,
  resolvePresenceServerUrl,
  shouldConnectPresence,
  type PresenceAwarenessState,
  type PresenceCursorPoint,
  type PresenceUser,
  type PresenceViewportState,
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

type CanvasPresenceFlow = {
  getSelectedElementIds: () => string[]
  viewport: Ref<{ x: number; y: number; zoom: number }>
}

export type CanvasPresenceSessionOptions = {
  boardId?: Ref<string | null | undefined> | undefined
  accessState?: Ref<string | null | undefined> | undefined
  workspaceId?: Ref<string | null | undefined> | undefined
  localUser?: Ref<PresenceUser | null> | undefined
  /** Device currently followed; broadcast as `followingDeviceId`. */
  followingDeviceId?: Ref<number | null | undefined> | undefined
  /** Whether this client broadcasts as a spotlight presenter. */
  isSpotlighting?: Ref<boolean | undefined> | undefined
  /** Whether this client may write (editors true, viewers false). */
  canWrite?: MaybeRefOrGetter<boolean> | undefined
  flow: CanvasPresenceFlow
  deps?: CanvasPresenceDeps | undefined
}

type CanvasPresenceSnapshot = {
  boardId: string
  workspaceId: string
  user: PresenceUser
}

function readSnapshot(options: CanvasPresenceSessionOptions): CanvasPresenceSnapshot | null {
  const boardId = options.boardId?.value?.trim() ?? ''
  const workspaceId = options.workspaceId?.value?.trim() ?? ''
  const user = options.localUser?.value ?? null
  if (!shouldConnectPresence(boardId, options.accessState?.value) || !workspaceId || !user) {
    return null
  }
  return { boardId, workspaceId, user }
}

function readSelectionIds(getSelectedElementIds: () => string[]): string[] {
  let ids: string[]
  try {
    ids = getSelectedElementIds()
  } catch {
    return []
  }
  if (!Array.isArray(ids)) return []
  const selected: string[] = []
  for (const id of ids) {
    if (typeof id === 'string' && id.trim()) selected.push(id.trim())
    if (selected.length >= PRESENCE_MAX_SELECTION_IDS) break
  }
  return selected
}

/** Broadcast identity using the canonical awareness fields. */
function buildBroadcastUser(user: PresenceUser): PresenceUser {
  return {
    id: user.id,
    name: user.name,
    color: user.color?.trim() || presencePeerColor(user.id),
    ...(user.avatarUrl?.trim() ? { avatarUrl: user.avatarUrl.trim() } : {}),
  }
}

export function useCanvasPresenceSession(options: CanvasPresenceSessionOptions) {
  const remotePeers = shallowRef<RemotePresencePeer[]>([])
  const isPresenceConnected = ref(false)
  const isDocSynced = ref(false)
  const localClientId = ref<number | null>(null)
  const sharedDoc = shallowRef<Y.Doc | null>(null)
  const requestTicket = options.deps?.requestTicket ?? requestBoardRealtimeTicket
  const createProvider = options.deps?.createProvider ?? createYPresenceProvider

  let handle: PresenceProviderHandle | null = null
  let detachProviderListeners: (() => void) | null = null
  let connectToken = 0
  let isDisposed = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectDelayMs = EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS
  let lastCursor: PresenceCursorPoint | null = null
  let isLastDragging = false

  function isCurrent(token: number): boolean {
    return !isDisposed && token === connectToken
  }

  function clearReconnectTimer(): void {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function publish(state: PresenceAwarenessState): void {
    handle?.provider.awareness.setLocalState(state)
  }

  const cursorThrottle = createBroadcastThrottle(PRESENCE_CURSOR_BROADCAST_INTERVAL_MS, publish)
  const dragThrottle = createBroadcastThrottle(PRESENCE_DRAG_BROADCAST_INTERVAL_MS, publish)

  function flushThrottles(): void {
    cursorThrottle.flush()
    dragThrottle.flush()
  }

  function cancelThrottles(): void {
    cursorThrottle.cancel()
    dragThrottle.cancel()
  }

  function teardown(): void {
    clearReconnectTimer()
    detachProviderListeners?.()
    detachProviderListeners = null
    handle?.dispose()
    handle = null
    cancelThrottles()
    isPresenceConnected.value = false
    isDocSynced.value = false
    localClientId.value = null
    sharedDoc.value = null
    remotePeers.value = []
    lastCursor = null
    isLastDragging = false
  }

  function readViewportSnapshot(): PresenceViewportState | null {
    const viewport = options.flow.viewport.value
    if (!viewport || !isPresenceViewportState(viewport)) return null
    return { x: viewport.x, y: viewport.y, zoom: viewport.zoom }
  }

  function publishSnapshot(cursor: PresenceCursorPoint | null, isDragging: boolean): void {
    const user = options.localUser?.value ?? null
    if (!handle || !user) return
    const following = options.followingDeviceId?.value ?? null
    // Viewers broadcast cursor-only: an empty selection keeps them out of
    // soft-lock contention (they cannot write).
    const selectionIds =
      toValue(options.canWrite ?? true) === false
        ? []
        : readSelectionIds(options.flow.getSelectedElementIds)
    const state: PresenceAwarenessState = {
      user: buildBroadcastUser(user),
      cursor: cursor ? { position: cursor, dragging: isDragging } : null,
      selection: { nodeIds: selectionIds, marquee: null },
      followingDeviceId: following,
      isSpotlighting: options.isSpotlighting?.value === true,
      viewport: readViewportSnapshot(),
    }
    if (isDragging) dragThrottle.push(state)
    else cursorThrottle.push(state)
  }

  function publishLocal(cursor: PresenceCursorPoint | null, isDragging = false): void {
    lastCursor = cursor
    isLastDragging = isDragging
    publishSnapshot(cursor, isDragging)
  }

  function publishPointer(cursor: PresenceCursorPoint | null, isDragging = false): void {
    if (!handle) return
    publishLocal(cursor, isDragging)
    if (cursor === null) flushThrottles()
  }

  function onAwarenessChange(): void {
    const awareness = handle?.provider.awareness
    if (!awareness) return
    const selfId = awareness.clientID
    const peers: RemotePresencePeer[] = []
    for (const [clientId, raw] of awareness.getStates()) {
      if (clientId === selfId) continue
      const state = parsePresenceAwarenessState(raw)
      if (state) peers.push({ clientId, state })
    }
    remotePeers.value = peers
  }

  function onProviderStatus(status: string): void {
    if (status === 'connected') {
      reconnectDelayMs = EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS
      isPresenceConnected.value = true
      return
    }
    isPresenceConnected.value = false
  }

  function onConnectionClose(token: number): (event: PresenceCloseEvent) => void {
    return (event) => {
      isPresenceConnected.value = false
      isDocSynced.value = false
      // Liveness-bound peers go stale with the socket on every close.
      remotePeers.value = []
      // Permanent sidecar refusals (4401 ticket, 4403 admit) must not
      // reconnect-loop: the kick model converges through the ticket window,
      // and a later board/user change re-settles from the watcher.
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
    publishLocal(null)
    flushThrottles()
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

  function publishCollaborationNow(): void {
    if (!handle) return
    publishSnapshot(lastCursor, isLastDragging)
    flushThrottles()
  }

  watch(
    () => [
      options.boardId?.value,
      options.accessState?.value,
      options.workspaceId?.value,
      options.localUser?.value,
    ],
    settle,
    { immediate: true },
  )

  // Camera and collaboration flags ride the same awareness state: viewport
  // moves republish throttled, while follow/spotlight toggles flush at once.
  watch(
    () => options.flow.viewport.value,
    () => {
      if (handle) publishSnapshot(lastCursor, isLastDragging)
    },
    { deep: true },
  )

  watch(
    () => [options.followingDeviceId?.value, options.isSpotlighting?.value],
    publishCollaborationNow,
  )

  onScopeDispose(() => {
    isDisposed = true
    teardown()
  })

  return { remotePeers, isPresenceConnected, isDocSynced, localClientId, sharedDoc, publishPointer }
}
