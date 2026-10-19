// Canvas presence state language (Y.js awareness-only).
//
// The presence channel carries ephemeral collaboration facts only: a
// short-lived ticket authorizes one board-scoped y-websocket room. Awareness
// follows the canonical contract and remote states fail closed on malformed
// required fields.

/** Dual-speed presence cadence: cursor ~30ms, drag ~100ms (TapNow §5). */
export const PRESENCE_CURSOR_BROADCAST_INTERVAL_MS = 30
export const PRESENCE_DRAG_BROADCAST_INTERVAL_MS = 100

/** Follow camera tween matching the TapNow 500ms viewport subscription. */
export const FOLLOW_VIEWPORT_TWEEN_MS = 500

/** y-websocket permanent close range (4400-4499): never reconnect. */
const PRESENCE_PERMANENT_CLOSE_MIN = 4400
const PRESENCE_PERMANENT_CLOSE_MAX = 4500

/** Upper bound on broadcast selection ids per awareness state. */
export const PRESENCE_MAX_SELECTION_IDS = 50

export type PresenceUser = {
  id: string
  name: string
  color?: string | undefined
  avatarUrl?: string | undefined
}

export type PresenceCursorPoint = {
  x: number
  y: number
}

/**
 * @externalContract TapNow awareness cursor wire, shared across collaborators.
 * `dragging` keeps the frozen contract name: dual-rate broadcast (~30ms rest,
 * ~100ms dragging) keys off it, so it must not be renamed to a predicate.
 */
type PresenceCursorState = {
  position: PresenceCursorPoint
  dragging: boolean
} | null

type PresenceMarqueeRect = {
  x: number
  y: number
  width: number
  height: number
}

type PresenceSelectionState = {
  nodeIds: string[]
  marquee?: PresenceMarqueeRect | null | undefined
}

export type PresenceViewportState = {
  x: number
  y: number
  zoom: number
}

export type PresenceAwarenessState = {
  user: PresenceUser
  cursor: PresenceCursorState
  selection: PresenceSelectionState
  followingDeviceId?: number | null | undefined
  isSpotlighting?: boolean | undefined
  viewport?: PresenceViewportState | null | undefined
}

export type RemotePresencePeer = {
  clientId: number
  state: PresenceAwarenessState
}

/**
 * Presence connect gate. Connects only for a concrete board whose
 * server-computed posture is `workspace`. Private boards and local drafts
 * never connect. Viewer/editor capabilities are intentionally not an input:
 * read-only viewers still broadcast presence.
 */
export function shouldConnectPresence(
  boardId: string | null | undefined,
  accessState: string | null | undefined,
): boolean {
  const normalizedId = typeof boardId === 'string' ? boardId.trim() : ''
  if (normalizedId.length === 0) return false
  return accessState === 'workspace'
}

/** Same-origin y-websocket base (`/realtime`) for a page origin. */
export function buildPresenceServerUrl(protocol: string, host: string): string {
  const scheme = protocol === 'https:' ? 'wss:' : 'ws:'
  return `${scheme}//${host}/realtime`
}

/** Same-origin y-websocket base for this page; null outside a browser. */
export function resolvePresenceServerUrl(): string | null {
  if (typeof window === 'undefined') return null
  const host = window.location?.host?.trim() ?? ''
  if (!host) return null
  return buildPresenceServerUrl(window.location.protocol, host)
}

/** Project one flow point to screen pixels under the current viewport. */
export function flowPointToScreenPoint(
  point: PresenceCursorPoint,
  viewport: { x: number; y: number; zoom: number },
): PresenceCursorPoint {
  return {
    x: point.x * viewport.zoom + viewport.x,
    y: point.y * viewport.zoom + viewport.y,
  }
}

/**
 * Whether a socket close code is permanent (sidecar 4401 ticket / 4403
 * admit): the client must not reconnect, mirroring the y-websocket
 * convention. Anything else (including a missing code) may retry.
 */
export function isPermanentPresenceClose(code: unknown): boolean {
  return (
    typeof code === 'number' &&
    Number.isInteger(code) &&
    code >= PRESENCE_PERMANENT_CLOSE_MIN &&
    code < PRESENCE_PERMANENT_CLOSE_MAX
  )
}

/** Stable per-user cursor/flag hue derived from the user id. */
export function presencePeerColor(userId: string): string {
  let hash = 0
  for (let index = 0; index < userId.length; index += 1) {
    hash = (hash * 31 + userId.charCodeAt(index)) | 0
  }
  const hue = ((hash % 360) + 360) % 360
  return `hsl(${hue} 70% 45%)`
}

/** Peer display color: the broadcast color when present, else the stable hash. */
export function resolvePeerColor(state: PresenceAwarenessState): string {
  const color = state.user.color?.trim() ?? ''
  if (color.length > 0 && color.length <= 64) return color
  return presencePeerColor(state.user.id)
}

/** Peer cursor flow point. */
export function resolvePeerCursorPoint(state: PresenceAwarenessState): PresenceCursorPoint | null {
  return state.cursor ? { x: state.cursor.position.x, y: state.cursor.position.y } : null
}

/** Peer selection ids. */
export function resolvePeerSelectionIds(state: PresenceAwarenessState): string[] {
  return [...state.selection.nodeIds]
}

/** Peer avatar URL. */
export function resolvePeerAvatarUrl(state: PresenceAwarenessState): string | undefined {
  return state.user.avatarUrl?.trim() || undefined
}

/** Whether the raw viewport frame is a usable camera snapshot. */
export function isPresenceViewportState(raw: unknown): raw is PresenceViewportState {
  if (!raw || typeof raw !== 'object') return false
  const record = raw as Record<string, unknown>
  const x = toFiniteNumber(record.x)
  const y = toFiniteNumber(record.y)
  const zoom = toFiniteNumber(record.zoom)
  return x !== null && y !== null && zoom !== null && zoom > 0
}

function toTrimmedText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function toFiniteNumber(value: unknown): number | null {
  const parsed = typeof value === 'number' ? value : Number.NaN
  return Number.isFinite(parsed) ? parsed : null
}

function toOptionalText(value: unknown): string | undefined {
  const text = toTrimmedText(value)
  return text ? text : undefined
}

function parsePresenceUser(raw: unknown): PresenceUser | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  const id = toTrimmedText(record.id)
  if (!id) return null
  const name = toTrimmedText(record.name) || id.replace(/^usr_/, '') || id
  const user: PresenceUser = { id, name }
  const color = toOptionalText(record.color)
  if (color && color.length <= 64) user.color = color
  const avatarUrl = toOptionalText(record.avatarUrl)
  if (avatarUrl) user.avatarUrl = avatarUrl
  return user
}

function parseCursorPoint(raw: unknown): PresenceCursorPoint | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  const x = toFiniteNumber(record.x)
  const y = toFiniteNumber(record.y)
  if (x === null || y === null) return null
  return { x, y }
}

function parsePresenceCursor(raw: unknown): PresenceCursorState | undefined {
  if (raw === null) return null
  if (raw === undefined) return undefined
  if (typeof raw !== 'object') return undefined
  const record = raw as Record<string, unknown>
  // Canonical frame: `{ position: { x, y }, dragging }`.
  if (record.position !== undefined) {
    const position = parseCursorPoint(record.position)
    if (!position) return undefined
    return { position, dragging: record.dragging === true }
  }
  return undefined
}

function parseSelectionIds(raw: unknown): string[] | null {
  if (!Array.isArray(raw)) return null
  const ids: string[] = []
  for (const item of raw) {
    const id = toTrimmedText(item)
    if (id) ids.push(id)
    if (ids.length >= PRESENCE_MAX_SELECTION_IDS) break
  }
  return ids
}

function parsePresenceMarquee(raw: unknown): PresenceMarqueeRect | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  const x = toFiniteNumber(record.x)
  const y = toFiniteNumber(record.y)
  const width = toFiniteNumber(record.width)
  const height = toFiniteNumber(record.height)
  if (x === null || y === null || width === null || height === null) return null
  if (width < 0 || height < 0) return null
  return { x, y, width, height }
}

function parsePresenceSelection(raw: unknown): PresenceSelectionState | null {
  if (raw === null || raw === undefined) return null
  // Canonical frame: `{ nodeIds, marquee }`.
  if (typeof raw === 'object') {
    const record = raw as Record<string, unknown>
    const nodeIds = parseSelectionIds(record.nodeIds)
    if (!nodeIds) return null
    const selection: PresenceSelectionState = { nodeIds, marquee: null }
    if (record.marquee !== undefined && record.marquee !== null) {
      const marquee = parsePresenceMarquee(record.marquee)
      if (!marquee) return null
      selection.marquee = marquee
    }
    return selection
  }
  return null
}

function parseFollowingDeviceId(raw: unknown): number | null {
  if (raw === null || raw === undefined) return null
  if (typeof raw === 'number' && Number.isInteger(raw) && raw >= 0) return raw
  return null
}

function parsePresenceViewport(raw: unknown): PresenceViewportState | null {
  if (raw === null || raw === undefined) return null
  return isPresenceViewportState(raw) ? { x: raw.x, y: raw.y, zoom: raw.zoom } : null
}

/**
 * Validate one remote awareness state. Returns null for malformed frames
 * (dropped, never rendered). A null cursor is valid: the peer is present
 * but their pointer left the canvas. Unknown collaboration fields ride the
 * sidecar untouched; only the frontend reads them here.
 */
export function parsePresenceAwarenessState(raw: unknown): PresenceAwarenessState | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  const user = parsePresenceUser(record.user)
  if (!user) return null
  const cursor = parsePresenceCursor(record.cursor)
  if (cursor === undefined) return null
  const selection = parsePresenceSelection(record.selection)
  if (!selection) return null
  const state: PresenceAwarenessState = { user, cursor, selection }
  const following = parseFollowingDeviceId(record.followingDeviceId)
  if (following !== null) state.followingDeviceId = following
  else if (record.followingDeviceId === null) state.followingDeviceId = null
  if (record.isSpotlighting === true) state.isSpotlighting = true
  const viewport = parsePresenceViewport(record.viewport)
  if (viewport) state.viewport = viewport
  return state
}

type PresenceBroadcastPublisher = (state: PresenceAwarenessState) => void

type PresenceBroadcastThrottle = {
  push: (state: PresenceAwarenessState) => void
  flush: () => void
  cancel: () => void
}

/** Leading + trailing throttle for local awareness broadcasts. */
export function createBroadcastThrottle(
  intervalMs: number,
  publish: PresenceBroadcastPublisher,
): PresenceBroadcastThrottle {
  const interval = Math.max(1, intervalMs)
  let lastSentAt = 0
  let timer: ReturnType<typeof setTimeout> | null = null
  let pending: PresenceAwarenessState | null = null

  function clearTimer(): void {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  function send(state: PresenceAwarenessState): void {
    lastSentAt = Date.now()
    publish(state)
  }

  function scheduleTrailing(): void {
    if (timer !== null) return
    const wait = Math.max(0, lastSentAt + interval - Date.now())
    timer = setTimeout(() => {
      timer = null
      const next = pending
      pending = null
      if (next) send(next)
    }, wait)
  }

  return {
    push(state: PresenceAwarenessState): void {
      if (Date.now() - lastSentAt >= interval) {
        clearTimer()
        pending = null
        send(state)
        return
      }
      pending = state
      scheduleTrailing()
    },
    flush(): void {
      const next = pending
      clearTimer()
      pending = null
      if (next) send(next)
    },
    cancel(): void {
      clearTimer()
      pending = null
    },
  }
}
