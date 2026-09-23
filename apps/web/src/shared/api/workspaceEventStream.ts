// Workspace event stream transport (issue #807, Public Data Plane).
//
// Versioned envelopes over `GET /api/v1/workspaces/{workspace_id}/events`
// (SSE), shared with signed webhooks (same envelope, delivery differs).
// Additive only: legacy `/user/events` (`userEventStream.ts`) is untouched
// (removal is #809).
//
// Resume: every server frame carries `id:` (the `evt_` envelope id), so
// native EventSource resumes automatically via Last-Event-ID. Fetch-based
// transports read the cursor back explicitly: this module tracks the last
// seen `evt_` id per workspace stream and re-attaches `?last_event_id=`
// on reconnect. A cursor bound to another workspace never resumes here
// (streams are keyed by workspace id). An `event: gap` notice frame
// (`{truncated: true}`) means the cursor was unknown or evicted: clients
// refetch state instead of silently missing events.
//
// Envelope validation mirrors the backend frozen registry
// (`internal/events/v1_envelope.go`, all version 1): unknown types and
// non-`evt_` ids fail closed (frame dropped, never delivered).

import { resolveApiUrl } from '../config/runtimeUrls'
import { isOpaqueId, opaqueIdPrefix } from './contractKernel'

export const WORKSPACE_EVENT_TYPES = [
  'task.created',
  'task.running',
  'task.completed',
  'task.failed',
  'task.cancelled',
  'media.asset_ready',
  'media.asset_archived',
  'board.document_replaced',
  'board.member_changed',
  'board.share_revoked',
  'billing.settled',
  'library.materialization_completed',
  'library.materialization_failed',
] as const

export type WorkspaceEventType = (typeof WORKSPACE_EVENT_TYPES)[number]

const WORKSPACE_EVENT_TYPE_SET: ReadonlySet<string> = new Set(WORKSPACE_EVENT_TYPES)

export function isWorkspaceEventType(value: unknown): value is WorkspaceEventType {
  return typeof value === 'string' && WORKSPACE_EVENT_TYPE_SET.has(value)
}

export type WorkspaceEventActor = {
  type: 'user' | 'service' | 'system'
  id: string
}

export type WorkspaceEventResource = {
  type: 'task' | 'board' | 'media_asset' | 'library_materialization' | 'checkout_session'
  id: string
}

export type WorkspaceEventEnvelope = {
  id: string
  type: WorkspaceEventType
  version: number
  occurred_at: string
  actor: WorkspaceEventActor
  resource: WorkspaceEventResource
  workspace_id: string
  data: Record<string, unknown>
}

function assertEvtId(value: unknown, label: string): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'evt') {
    throw new Error(`Invalid workspace event ${label}: expected evt_...`)
  }
  return value
}

function assertWorkspaceId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'ws') {
    throw new Error('Invalid workspace id: expected ws_...')
  }
  return value
}

function parseEventActor(raw: unknown): WorkspaceEventActor {
  const actor = raw as Record<string, unknown> | null | undefined
  if (
    typeof actor !== 'object' ||
    actor === null ||
    (actor.type !== 'user' && actor.type !== 'service' && actor.type !== 'system') ||
    typeof actor.id !== 'string' ||
    actor.id.trim().length === 0
  ) {
    throw new Error('Invalid workspace event envelope: actor is required')
  }
  return { type: actor.type, id: actor.id }
}

function parseEventResource(raw: unknown): WorkspaceEventResource {
  const resource = raw as Record<string, unknown> | null | undefined
  if (
    typeof resource !== 'object' ||
    resource === null ||
    typeof resource.type !== 'string' ||
    typeof resource.id !== 'string' ||
    resource.id.trim().length === 0
  ) {
    throw new Error('Invalid workspace event envelope: resource is required')
  }
  return { type: resource.type as WorkspaceEventResource['type'], id: resource.id }
}

function parseEventData(raw: unknown): Record<string, unknown> {
  const data = raw as Record<string, unknown> | null | undefined
  if (typeof data !== 'object' || data === null || Array.isArray(data)) {
    throw new Error('Invalid workspace event envelope: data is required')
  }
  return data
}

/** Parse + validate one SSE `data:` payload into a typed envelope. Throws on invalid. */
export function parseWorkspaceEventEnvelope(raw: unknown): WorkspaceEventEnvelope {
  const envelope = (
    typeof raw === 'string' ? (JSON.parse(raw) as unknown) : raw
  ) as Record<string, unknown>
  if (typeof envelope !== 'object' || envelope === null) {
    throw new Error('Invalid workspace event envelope: expected an object')
  }
  const id = assertEvtId(envelope.id, 'id')
  const type = envelope.type
  if (!isWorkspaceEventType(type)) {
    throw new Error(`Unknown workspace event type: ${String(type)}`)
  }
  if (envelope.version !== 1) {
    throw new Error(`Unsupported workspace event version for ${type}`)
  }
  if (typeof envelope.occurred_at !== 'string' || envelope.occurred_at.trim().length === 0) {
    throw new Error('Invalid workspace event envelope: occurred_at is required')
  }
  return {
    id,
    type,
    version: 1,
    occurred_at: envelope.occurred_at,
    actor: parseEventActor(envelope.actor),
    resource: parseEventResource(envelope.resource),
    workspace_id: assertWorkspaceId(envelope.workspace_id),
    data: parseEventData(envelope.data),
  }
}

/** Normalize a `types` filter: empty means all frozen types; unknown types throw. */
export function normalizeWorkspaceEventTypesFilter(types: readonly string[] | undefined): string[] {
  if (types == null || types.length === 0) return []
  const seen = new Set<string>()
  for (const raw of types) {
    const trimmed = typeof raw === 'string' ? raw.trim() : ''
    if (trimmed.length === 0) continue
    if (!isWorkspaceEventType(trimmed)) {
      throw new Error(`Unknown workspace event type filter: ${trimmed}`)
    }
    seen.add(trimmed)
  }
  return [...seen].sort()
}

export type WorkspaceEventsQuery = {
  types?: readonly string[] | undefined
  lastEventId?: string | undefined
}

function encodeId(value: string): string {
  return encodeURIComponent(value)
}

/** Canonical transport suffix: `/v1/workspaces/{ws}/events[?types=&last_event_id=]`. */
export function workspaceEventsSuffix(workspaceId: string, query?: WorkspaceEventsQuery): string {
  const workspace = assertWorkspaceId(workspaceId.trim())
  const search = new URLSearchParams()
  const types = normalizeWorkspaceEventTypesFilter(query?.types)
  if (types.length > 0) search.set('types', types.join(','))
  const cursor = query?.lastEventId?.trim() ?? ''
  if (cursor.length > 0) search.set('last_event_id', assertEvtId(cursor, 'cursor'))
  const suffix = `/v1/workspaces/${encodeId(workspace)}/events`
  const serialized = search.toString()
  return serialized.length > 0 ? `${suffix}?${serialized}` : suffix
}

export function workspaceEventsUrl(workspaceId: string, query?: WorkspaceEventsQuery): string {
  return resolveApiUrl(workspaceEventsSuffix(workspaceId, query))
}

export type WorkspaceEventListener = (envelope: WorkspaceEventEnvelope) => void
export type WorkspaceEventGapListener = (info: { lastEventId: string }) => void

export type SubscribeWorkspaceEventsOptions = {
  types?: readonly string[] | undefined
  lastEventId?: string | undefined
  onGap?: WorkspaceEventGapListener | undefined
  onError?: (() => void) | undefined
}

const RECONNECT_INITIAL_DELAY_MS = 1000
const RECONNECT_MAX_DELAY_MS = 30000
const RECONNECT_BACKOFF_FACTOR = 1.6

type WorkspaceStreamEntry = {
  workspaceId: string
  types: string[]
  source: EventSource | null
  listeners: Set<WorkspaceEventListener>
  gapListeners: Set<WorkspaceEventGapListener>
  errorListeners: Set<() => void>
  lastEventId: string
  reconnectDelayMs: number
  reconnectTimer: ReturnType<typeof setTimeout> | null
  closed: boolean
}

const streams = new Map<string, WorkspaceStreamEntry>()

function streamKey(workspaceId: string, types: readonly string[]): string {
  return types.length === 0 ? workspaceId : `${workspaceId}|${types.join(',')}`
}

function transportTypes(entry: WorkspaceStreamEntry): readonly string[] {
  return entry.types.length > 0 ? entry.types : WORKSPACE_EVENT_TYPES
}

function clearReconnectTimer(entry: WorkspaceStreamEntry): void {
  if (entry.reconnectTimer !== null) {
    clearTimeout(entry.reconnectTimer)
    entry.reconnectTimer = null
  }
}

function closeEntry(entry: WorkspaceStreamEntry): void {
  entry.closed = true
  clearReconnectTimer(entry)
  if (entry.source) {
    entry.source.close()
    entry.source = null
  }
}

function closeWhenIdle(entry: WorkspaceStreamEntry): void {
  if (entry.listeners.size > 0 || entry.gapListeners.size > 0 || entry.errorListeners.size > 0) {
    return
  }
  closeEntry(entry)
  streams.delete(streamKey(entry.workspaceId, entry.types))
}

function scheduleReconnect(entry: WorkspaceStreamEntry): void {
  if (entry.closed || entry.reconnectTimer !== null) return
  const delay = entry.reconnectDelayMs
  entry.reconnectDelayMs = Math.min(delay * RECONNECT_BACKOFF_FACTOR, RECONNECT_MAX_DELAY_MS)
  entry.reconnectTimer = setTimeout(() => {
    entry.reconnectTimer = null
    if (entry.closed) return
    openTransport(entry)
  }, delay)
}

function handleEnvelopeFrame(entry: WorkspaceStreamEntry, raw: string): void {
  let envelope: WorkspaceEventEnvelope
  try {
    envelope = parseWorkspaceEventEnvelope(raw)
  } catch {
    return
  }
  if (envelope.workspace_id !== entry.workspaceId) return
  entry.lastEventId = envelope.id
  entry.reconnectDelayMs = RECONNECT_INITIAL_DELAY_MS
  for (const listener of entry.listeners) listener(envelope)
}

function openTransport(entry: WorkspaceStreamEntry): void {
  if (entry.closed || typeof EventSource === 'undefined') return
  if (entry.source) {
    entry.source.close()
    entry.source = null
  }
  const url = workspaceEventsUrl(entry.workspaceId, {
    ...(entry.types.length > 0 ? { types: entry.types } : {}),
    ...(entry.lastEventId.length > 0 ? { lastEventId: entry.lastEventId } : {}),
  })
  const source = new EventSource(url, { withCredentials: true })
  entry.source = source
  for (const type of transportTypes(entry)) {
    source.addEventListener(type, (event) => {
      handleEnvelopeFrame(entry, (event as MessageEvent<string>).data ?? '')
    })
  }
  source.addEventListener('gap', () => {
    const info = { lastEventId: entry.lastEventId }
    for (const listener of entry.gapListeners) listener(info)
  })
  source.onerror = () => {
    for (const listener of entry.errorListeners) listener()
    scheduleReconnect(entry)
  }
}

/**
 * Subscribe to the versioned workspace event stream. Streams are shared per
 * workspace + types filter; every subscriber receives parsed envelopes for
 * matching types. Returns an unsubscribe function. The `onGap` callback
 * fires when the server reports cursor truncation (refetch state); `onError`
 * fires on transport errors (the stream reconnects with backoff, resuming
 * from the last seen `evt_` id).
 */
export function subscribeWorkspaceEvent(
  workspaceId: string,
  listener: WorkspaceEventListener,
  options?: SubscribeWorkspaceEventsOptions,
): () => void {
  const workspace = assertWorkspaceId(workspaceId.trim())
  const types = normalizeWorkspaceEventTypesFilter(options?.types)
  const key = streamKey(workspace, types)
  let entry = streams.get(key)
  if (!entry || entry.closed) {
    entry = {
      workspaceId: workspace,
      types,
      source: null,
      listeners: new Set(),
      gapListeners: new Set(),
      errorListeners: new Set(),
      lastEventId: options?.lastEventId?.trim() ?? '',
      reconnectDelayMs: RECONNECT_INITIAL_DELAY_MS,
      reconnectTimer: null,
      closed: false,
    }
    if (entry.lastEventId.length > 0) assertEvtId(entry.lastEventId, 'cursor')
    streams.set(key, entry)
    openTransport(entry)
  }
  const active = entry
  active.listeners.add(listener)
  if (options?.onGap) active.gapListeners.add(options.onGap)
  if (options?.onError) active.errorListeners.add(options.onError)

  return () => {
    active.listeners.delete(listener)
    if (options?.onGap) active.gapListeners.delete(options.onGap)
    if (options?.onError) active.errorListeners.delete(options.onError)
    closeWhenIdle(active)
  }
}

/** Test seam: drop all shared streams (specs only). */
export function resetWorkspaceEventStreamsForTests(): void {
  for (const entry of streams.values()) closeEntry(entry)
  streams.clear()
}

export type WaitWorkspaceEnvelopeOptions = {
  types?: readonly string[] | undefined
  timeoutMs?: number | undefined
  signal?: AbortSignal | undefined
}

const WAIT_DEFAULT_TIMEOUT_MS = 6 * 60 * 1000

/**
 * Resolve with the first envelope matching `predicate`. Rejects on timeout
 * or abort. Transport errors do not reject (the stream reconnects); only
 * the deadline/abort settles the wait.
 */
export function waitForWorkspaceEnvelope(
  workspaceId: string,
  predicate: (envelope: WorkspaceEventEnvelope) => boolean,
  options?: WaitWorkspaceEnvelopeOptions,
): Promise<WorkspaceEventEnvelope> {
  const timeoutMs = options?.timeoutMs ?? WAIT_DEFAULT_TIMEOUT_MS
  return new Promise((resolve, reject) => {
    let settled = false
    let timer = 0
    let unsubscribe: () => void = () => undefined

    const cleanup = () => {
      window.clearTimeout(timer)
      options?.signal?.removeEventListener('abort', onAbort)
      unsubscribe()
    }
    const settleResolve = (envelope: WorkspaceEventEnvelope) => {
      if (settled) return
      settled = true
      cleanup()
      resolve(envelope)
    }
    const settleReject = (error: Error) => {
      if (settled) return
      settled = true
      cleanup()
      reject(error)
    }
    function onAbort(): void {
      settleReject(new Error('Workspace event wait aborted'))
    }

    if (options?.signal?.aborted === true) {
      settleReject(new Error('Workspace event wait aborted'))
      return
    }
    options?.signal?.addEventListener('abort', onAbort, { once: true })
    timer = window.setTimeout(() => {
      settleReject(new Error('Workspace event wait timed out'))
    }, Math.max(0, timeoutMs))

    try {
      unsubscribe = subscribeWorkspaceEvent(
        workspaceId,
        (envelope) => {
          const matches = (() => {
            try {
              return predicate(envelope)
            } catch {
              return false
            }
          })()
          if (matches) settleResolve(envelope)
        },
        { types: options?.types },
      )
    } catch (error) {
      settleReject(error instanceof Error ? error : new Error(String(error)))
    }
  })
}

/**
 * Subscribe to workspace media readiness transitions. Extracts `asset_id`
 * from `media.asset_ready` / `media.asset_archived` envelopes and reports
 * it for cache invalidation. Malformed frames never reach the callback.
 * Legacy user-scope invalidation (`mediaAssetDelivery.ts`) is untouched.
 */
export function subscribeWorkspaceMediaInvalidation(
  workspaceId: string,
  onInvalidated: (assetId: string) => void,
  onError?: () => void,
): () => void {
  return subscribeWorkspaceEvent(
    workspaceId,
    (envelope) => {
      if (envelope.type !== 'media.asset_ready' && envelope.type !== 'media.asset_archived') {
        return
      }
      const raw = envelope.data.asset_id
      const assetId = typeof raw === 'string' ? raw.trim() : ''
      if (assetId.length > 0) onInvalidated(assetId)
    },
    { types: ['media.asset_ready', 'media.asset_archived'], ...(onError == null ? {} : { onError }) },
  )
}
