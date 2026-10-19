// Canvas presence session (Y.js awareness-only).
//
// Connects only while a concrete board with `workspace` posture is open on
// this screen: CanvasScreen mounts this composable, and the watcher below
// tears the room down on private posture, drafts, board switches, and scope
// disposal (unmount). Viewers broadcast cursor-only (empty selection, no
// lock claims, no writes); only the posture gates.
//
// Transport: one ticket per (re)connect (`./ticket`), same-origin
// `/realtime?ticket=` via `./provider` (the room rides the ticket payload,
// never the URL). Reconnect reuses the workspace event-stream backoff
// constants and re-signs a fresh ticket on every attempt; permanent sidecar
// refusals (4401 ticket, 4403 admit) never reconnect. Local
// cursor/selection broadcasts use dual-speed throttling (TapNow §5):
// cursor ~30ms, dragging ~100ms. Viewers broadcast cursor-only (empty
// selection) so they never claim soft locks and never write.

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
import { requestPresenceTicket } from './ticket'
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
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  getSelectedElementIds: () => string[]
  viewport: Ref<{ x: number; y: number; zoom: number }>
}

type CanvasPresenceOptions = {
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
  element: Ref<HTMLElement | null>
  deps?: CanvasPresenceDeps | undefined
}

type CanvasPresenceSnapshot = {
  boardId: string
  workspaceId: string
  user: PresenceUser
}

function readSnapshot(options: CanvasPresenceOptions): CanvasPresenceSnapshot | null {
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

export function useCanvasPresence(options: CanvasPresenceOptions) {
  const remotePeers = shallowRef<RemotePresencePeer[]>([])
  const isPresenceConnected = ref(false)
  const isDocSynced = ref(false)
  const localClientId = ref<number | null>(null)
  const sharedDoc = shallowRef<Y.Doc | null>(null)
  const requestTicket = options.deps?.requestTicket ?? requestPresenceTicket
  const createProvider = options.deps?.createProvider ?? createYPresenceProvider

  let handle: PresenceProviderHandle | null = null
  let detachProviderListeners: (() => void) | null = null
  let connectToken = 0
  let isDisposed = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectDelayMs = EVENT_STREAM_RECONNECT_INITIAL_DELAY_MS
  let lastCursor: PresenceCursorPoint | null = null
  let isLastDragging = false
  let pointerElement: HTMLElement | null = null

  function isCurrent(token: number): boolean {
    return !isDisposed && token === connectToken
  }

  function clearReconnectTimer(): void {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function detachPointerListeners(element: HTMLElement | null): void {
    element?.removeEventListener('pointermove', onPointerMove)
    element?.removeEventListener('pointerleave', onPointerLeave)
    if (pointerElement === element) pointerElement = null
  }

  function attachPointerListeners(element: HTMLElement | null): void {
    if (pointerElement === element) return
    detachPointerListeners(pointerElement)
    pointerElement = element
    element?.addEventListener('pointermove', onPointerMove)
    element?.addEventListener('pointerleave', onPointerLeave)
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
    detachPointerListeners(pointerElement)
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

  function onPointerMove(event: PointerEvent): void {
    if (!handle) return
    const flow = options.flow.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    if (!Number.isFinite(flow.x) || !Number.isFinite(flow.y)) return
    publishLocal({ x: flow.x, y: flow.y }, event.buttons !== 0)
  }

  function onPointerLeave(): void {
    if (!handle) return
    publishLocal(null)
    flushThrottles()
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
    attachPointerListeners(options.element.value)
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

  watch(
    options.element,
    (next) => {
      if (handle) attachPointerListeners(next ?? null)
    },
    { immediate: true, flush: 'sync' },
  )

  onScopeDispose(() => {
    isDisposed = true
    teardown()
  })

  return { remotePeers, isPresenceConnected, isDocSynced, localClientId, sharedDoc }
}
