// Canvas presence state language (Y.js awareness-only).
//
// The presence channel carries ephemeral collaboration facts only: a
// short-lived ticket authorizes one board-scoped y-websocket room. Awareness
// follows the canonical contract and remote states fail closed on malformed
// required fields.

/** y-websocket permanent close range (4400-4499): never reconnect. */
const PRESENCE_PERMANENT_CLOSE_MIN = 4400
const PRESENCE_PERMANENT_CLOSE_MAX = 4500

/** Upper bound on broadcast selection ids per awareness state. */
const PRESENCE_MAX_SELECTION_IDS = 50

export type PresenceUser = {
  id: string
  name: string
  avatarUrl?: string | undefined
}

type PresenceSelectionState = {
  nodeIds: string[]
}

export type PresenceAwarenessState = {
  user: PresenceUser
  selection: PresenceSelectionState
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

function toTrimmedText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
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
  const avatarUrl = toOptionalText(record.avatarUrl)
  if (avatarUrl) user.avatarUrl = avatarUrl
  return user
}

export function parseSelectionIds(raw: unknown): string[] | null {
  if (!Array.isArray(raw)) return null
  const ids: string[] = []
  for (const item of raw) {
    const id = toTrimmedText(item)
    if (id) ids.push(id)
    if (ids.length >= PRESENCE_MAX_SELECTION_IDS) break
  }
  return ids
}

function parsePresenceSelection(raw: unknown): PresenceSelectionState | null {
  if (!raw || typeof raw !== 'object') return null
  const nodeIds = parseSelectionIds((raw as Record<string, unknown>).nodeIds)
  return nodeIds ? { nodeIds } : null
}

/**
 * Validate one remote awareness state. Returns null for malformed frames
 * (dropped, never rendered). Unknown fields are dropped; only the
 * canonical `user`/`selection` contract is read.
 */
export function parsePresenceAwarenessState(raw: unknown): PresenceAwarenessState | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  const user = parsePresenceUser(record.user)
  if (!user) return null
  const selection = parsePresenceSelection(record.selection)
  if (!selection) return null
  return { user, selection }
}
