// Y.js presence provider seam (standard yjs + y-websocket client).
//
// The provider hosts one shared Y.Doc per room. Presence uses `awareness`
// only; the Canvas collaboration session mounts the frozen
// `nodes`/`edges` shared maps on the same doc when the board is a writable
// workspace board (viewers and private boards never create shared types).
// The socket is same-origin `/realtime?ticket=<ticket>` (ADR-0019): the
// sidecar only upgrades that exact path and derives the room from the
// ticket payload, so the stock `/<room>` suffix the provider appends is
// stripped by the socket adapter below. Protocol framing, awareness, and
// the sync handshake stay stock y-websocket. The provider's own
// auto-reconnect stays disabled so every reconnect re-signs a fresh ticket
// through the connection owner (see `./useCanvasPresence`).

import type * as Y from 'yjs'
import type { PresenceAwarenessState } from './state'

export type PresenceCloseEvent = {
  code?: unknown
  reason?: unknown
} | null

type PresenceAwarenessPort = {
  readonly clientID: number
  setLocalState: (state: PresenceAwarenessState | null) => void
  getStates: () => Map<number, unknown>
  on: (event: 'change', listener: () => void) => void
  off: (event: 'change', listener: () => void) => void
}

export type PresenceProviderStatus = 'connected' | 'disconnected' | 'connecting'

type PresenceProviderPort = {
  readonly awareness: PresenceAwarenessPort
  onConnectionClose: (listener: (event: PresenceCloseEvent) => void) => () => void
  onStatus: (listener: (status: PresenceProviderStatus) => void) => () => void
  readonly isSynced: boolean
  onSync: (listener: (isSynced: boolean) => void) => () => void
}

export type PresenceProviderHandle = {
  provider: PresenceProviderPort
  /** Shared room doc (awareness + doc-binding `nodes`/`edges` maps). */
  doc: Y.Doc
  dispose: () => void
}

export type PresenceProviderFactory = (input: {
  serverBaseUrl: string
  room: string
  ticket: string
}) => Promise<PresenceProviderHandle>

function isPresenceProviderStatus(value: unknown): value is PresenceProviderStatus {
  return value === 'connected' || value === 'disconnected' || value === 'connecting'
}

/**
 * Strip the provider-appended `/<room>` suffix so the upgrade hits the
 * exact `/realtime` path the sidecar admits. Board ids are opaque
 * single-segment identities, so dropping the last path segment is exact;
 * the `?ticket=` query is preserved untouched.
 */
export function toBareRealtimeUrl(url: string | URL): string {
  const parsed = new URL(typeof url === 'string' ? url : url.toString())
  parsed.pathname = parsed.pathname.replace(/\/[^/]*$/, '') || '/'
  return parsed.toString()
}

/** Stock WebSocket that only rewrites the upgrade URL to bare `/realtime`. */
class RealtimePresenceSocket extends WebSocket {
  constructor(url: string | URL, protocols?: string | string[]) {
    super(toBareRealtimeUrl(url), protocols)
  }
}

/** Production factory: dynamic import keeps yjs out of the initial bundle. */
export const createYPresenceProvider: PresenceProviderFactory = async (input) => {
  const [{ Doc }, { WebsocketProvider }] = await Promise.all([import('yjs'), import('y-websocket')])
  // The room supplies persisted document state through the sync handshake.
  const doc = new Doc()
  const provider = new WebsocketProvider(input.serverBaseUrl, input.room, doc, {
    params: { ticket: input.ticket },
    disableBc: true,
    shouldReconnect: () => false,
    WebSocketPolyfill: RealtimePresenceSocket,
  })
  const awareness = provider.awareness

  function dispose(): void {
    try {
      awareness.destroy()
    } catch {
      // Best-effort teardown: the socket close below is authoritative.
    }
    try {
      provider.destroy()
    } catch {
      // Best-effort teardown: the socket close below is authoritative.
    }
    try {
      doc.destroy()
    } catch {
      // Best-effort teardown: provider destroy already disconnected.
    }
  }

  return {
    provider: {
      get isSynced() {
        return provider.synced
      },
      onSync: (listener) => {
        provider.on('sync', listener)
        return () => provider.off('sync', listener)
      },
      awareness: {
        clientID: awareness.clientID,
        setLocalState: (state) => awareness.setLocalState(state),
        getStates: () => awareness.getStates() as Map<number, unknown>,
        on: (event, listener) => awareness.on(event, listener),
        off: (event, listener) => awareness.off(event, listener),
      },
      onConnectionClose: (listener) => {
        provider.on('connection-close', listener)
        return () => provider.off('connection-close', listener)
      },
      onStatus: (listener) => {
        const forward = (event: { status: unknown }) => {
          if (isPresenceProviderStatus(event.status)) listener(event.status)
        }
        provider.on('status', forward)
        return () => provider.off('status', forward)
      },
    },
    doc,
    dispose,
  }
}
