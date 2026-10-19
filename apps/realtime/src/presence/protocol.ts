// Presence Y protocol handling (ADR-0020, was presence-only under ADR-0019).
// Standard pieces only: yjs provides the room Doc (getMap('nodes') +
// getMap('edges'), keyed by id), y-protocols/sync frames the handshake,
// y-protocols/awareness carries identity/selection, lib0 does the varuint
// framing. No custom message types.
//
// SyncStep1 is answered read-only (state vector of the room Doc) so
// unmodified y-websocket clients complete their handshake. SyncStep2/Update
// frames converge here: they are applied to the room Doc, broadcast to room
// peers for WS incremental convergence, and handed to onDocUpdate for Go
// persistence (opaque bytes via POST /internal/realtime/y-updates, with
// retry; see persist.ts). Frames from connections without write capability,
// malformed frames, and oversized updates are dropped and counted in
// ignoredDocUpdates (/health). Awareness stays standard forwarding and never
// touches persistence. Document snapshots still hydrate first over HTTP
// (GET board + previewscene); the non-collaborator If-Match + 409 path is
// untouched.

import * as Y from 'yjs'
import * as syncProtocol from 'y-protocols/sync'
import * as awarenessProtocol from 'y-protocols/awareness'
import * as encoding from 'lib0/encoding'
import * as decoding from 'lib0/decoding'
import { maxYUpdateBytes } from './ydoc.js'

export const messageSync = 0
export const messageAwareness = 1

export type SendableConn = {
  readyState: number
  send: (data: Uint8Array) => void
  close: (code: number, reason: string) => void
}

// lib0 has no exported OPEN constant; ws uses 1 for OPEN.
const readyStateOpen = 1

export class PresenceRoom {
  readonly name: string
  /** Room Doc: getMap('nodes') + getMap('edges'), keyed by id (ADR-0020). */
  readonly doc: Y.Doc
  readonly awareness: awarenessProtocol.Awareness
  readonly conns = new Map<SendableConn, Set<number>>()
  /** Dropped doc frames (no write cap, malformed, oversized). */
  ignoredDocUpdates = 0
  /** Applied doc frames (converged + broadcast + handed to onDocUpdate). */
  appliedDocUpdates = 0
  private readonly writers = new Map<SendableConn, boolean>()
  private destroyed = false

  constructor(
    name: string,
    private readonly hooks: {
      onDocUpdate?: (update: Uint8Array, sender: SendableConn) => void
      canAcceptDocUpdate?: (sender: SendableConn) => boolean
    } = {},
  ) {
    this.name = name
    this.doc = new Y.Doc()
    this.awareness = new awarenessProtocol.Awareness(this.doc)
    this.awareness.setLocalState(null)
    this.awareness.on(
      'update',
      (
        { added, updated, removed }: { added: number[]; updated: number[]; removed: number[] },
        origin: unknown,
      ) => {
        const changed = [...added, ...updated, ...removed]
        // Track which client ids each conn controls so close reaps them.
        if (origin !== null && origin !== undefined) {
          const controlled = this.conns.get(origin as SendableConn)
          if (controlled !== undefined) {
            added.forEach((clientID) => controlled.add(clientID))
            removed.forEach((clientID) => controlled.delete(clientID))
          }
        }
        const encoder = encoding.createEncoder()
        encoding.writeVarUint(encoder, messageAwareness)
        encoding.writeVarUint8Array(
          encoder,
          awarenessProtocol.encodeAwarenessUpdate(this.awareness, changed),
        )
        this.broadcast(encoding.toUint8Array(encoder))
      },
    )
  }

  hydrate(snapshot: unknown, updates: readonly Uint8Array[]): void {
    if (
      this.connectionCount > 0 ||
      this.doc.getMap('nodes').size + this.doc.getMap('edges').size > 0
    ) {
      throw new Error('room hydration must precede admission')
    }
    if (!snapshot || typeof snapshot !== 'object' || Array.isArray(snapshot))
      throw new Error('invalid persisted snapshot')
    const envelope = snapshot as { payload?: unknown; schema_version?: unknown }
    const payload = envelope.payload
    if (!payload || typeof payload !== 'object' || Array.isArray(payload))
      throw new Error('invalid persisted payload')
    const value = payload as Record<string, unknown>
    const nodes = this.doc.getMap('nodes')
    const edges = this.doc.getMap('edges')
    if (typeof value.y_state === 'string' && value.y_state) {
      Y.applyUpdate(this.doc, new Uint8Array(Buffer.from(value.y_state, 'base64')))
    } else {
      const initialEmpty = envelope.schema_version === 1 && Object.keys(value).length === 0
      if (!initialEmpty && (envelope.schema_version !== 4 || !Array.isArray(value.imageNodes))) {
        throw new Error('persisted collaboration snapshot requires Y state')
      }
      // All first-room seeds use the same reserved identity. Tail updates may
      // reference these structs even after every original connection leaves.
      const clientID = this.doc.clientID
      this.doc.clientID = 0
      try {
        const nodeKeyById = new Map<string, string>()
        for (const [field, kind] of [
          ['imageNodes', 'image'],
          ['promptNodes', 'prompt'],
          ['fusionNodes', 'fusion'],
          ['groups', 'group'],
        ] as const) {
          const items = value[field]
          if (!Array.isArray(items)) continue
          for (const raw of items) {
            if (!raw || typeof raw !== 'object' || Array.isArray(raw)) continue
            const data = raw as Record<string, unknown>
            if (typeof data.id !== 'string' && typeof data.id !== 'number') continue
            const key =
              kind === 'group' && String(data.id).startsWith('group-')
                ? String(data.id)
                : `${kind}-${String(data.id)}`
            nodeKeyById.set(String(data.id), key)
            nodes.set(key, {
              id: key,
              type: kind,
              position: { x: Number(data.x) || 0, y: Number(data.y) || 0 },
              data,
            })
          }
        }
        if (Array.isArray(value.connections))
          for (const raw of value.connections) {
            if (!raw || typeof raw !== 'object' || Array.isArray(raw)) continue
            const data = raw as Record<string, unknown>
            if (typeof data.id !== 'string' && typeof data.id !== 'number') continue
            const key = `connection-${String(data.id)}`
            edges.set(key, {
              id: key,
              source: nodeKeyById.get(String(data.fromNodeId)) ?? String(data.fromNodeId),
              target: nodeKeyById.get(String(data.toNodeId)) ?? String(data.toNodeId),
              data,
            })
          }
      } finally {
        this.doc.clientID = clientID
      }
    }
    for (const update of updates) Y.applyUpdate(this.doc, update)
  }

  get connectionCount(): number {
    return this.conns.size
  }

  addConn(conn: SendableConn): void {
    this.conns.set(conn, new Set())
    this.writers.set(conn, false)
  }

  /** Admit-time write gating: only ticket cap=write connections converge doc frames. */
  setWriter(conn: SendableConn, canWrite: boolean): void {
    if (this.conns.has(conn)) this.writers.set(conn, canWrite)
  }

  /** Handle one inbound binary message. Returns reply bytes, or null. */
  handleMessage(conn: SendableConn, message: Uint8Array): Uint8Array | null {
    try {
      const decoder = decoding.createDecoder(message)
      const messageType = decoding.readVarUint(decoder)
      if (messageType === messageSync) {
        const syncType = decoding.readVarUint(decoder)
        if (syncType === syncProtocol.messageYjsSyncStep1) {
          // Step1 is read-only (encodes a diff of the room Doc): answer
          // Step2 on the same decoder, keeping the outer frame prefix.
          const encoder = encoding.createEncoder()
          encoding.writeVarUint(encoder, messageSync)
          syncProtocol.readSyncStep1(decoder, encoder, this.doc)
          return encoding.length(encoder) > 1 ? encoding.toUint8Array(encoder) : null
        }
        // SyncStep2 / Update: converge, broadcast to peers, persist.
        // Both carry a single update payload after the sync type.
        let update: Uint8Array
        try {
          update = decoding.readVarUint8Array(decoder)
        } catch {
          this.ignoredDocUpdates += 1
          return null
        }
        if (update.length === 0 || update.length > maxYUpdateBytes) {
          this.ignoredDocUpdates += 1
          return null
        }
        if (this.writers.get(conn) !== true) {
          this.ignoredDocUpdates += 1
          return null
        }
        if (this.hooks.canAcceptDocUpdate && !this.hooks.canAcceptDocUpdate(conn)) {
          this.ignoredDocUpdates += 1
          return null
        }
        try {
          Y.applyUpdate(this.doc, update, conn)
        } catch {
          this.ignoredDocUpdates += 1
          return null
        }
        this.appliedDocUpdates += 1
        this.broadcastDocUpdate(conn, update)
        try {
          this.hooks.onDocUpdate?.(update, conn)
        } catch {
          // Persistence handoff never breaks convergence or broadcast.
        }
        return null
      }
      if (messageType === messageAwareness) {
        awarenessProtocol.applyAwarenessUpdate(
          this.awareness,
          decoding.readVarUint8Array(decoder),
          conn,
        )
        return null
      }
      return null
    } catch {
      return null
    }
  }

  /** Snapshot for a fresh connection: SyncStep1 reply + awareness states. */
  initialMessages(): Uint8Array[] {
    const out: Uint8Array[] = []
    const step1 = encoding.createEncoder()
    encoding.writeVarUint(step1, messageSync)
    syncProtocol.writeSyncStep1(step1, this.doc)
    out.push(encoding.toUint8Array(step1))
    const states = this.awareness.getStates()
    if (states.size > 0) {
      const encoder = encoding.createEncoder()
      encoding.writeVarUint(encoder, messageAwareness)
      encoding.writeVarUint8Array(
        encoder,
        awarenessProtocol.encodeAwarenessUpdate(this.awareness, Array.from(states.keys())),
      )
      out.push(encoding.toUint8Array(encoder))
    }
    return out
  }

  removeConn(conn: SendableConn): void {
    const controlled = this.conns.get(conn)
    if (controlled === undefined) return
    this.conns.delete(conn)
    this.writers.delete(conn)
    awarenessProtocol.removeAwarenessStates(this.awareness, Array.from(controlled), null)
  }

  /** Broadcast one applied update to every peer except the sender. */
  private broadcastDocUpdate(sender: SendableConn, update: Uint8Array): void {
    const encoder = encoding.createEncoder()
    encoding.writeVarUint(encoder, messageSync)
    syncProtocol.writeUpdate(encoder, update)
    const frame = encoding.toUint8Array(encoder)
    for (const conn of this.conns.keys()) {
      if (conn === sender || conn.readyState !== readyStateOpen) continue
      try {
        conn.send(frame)
      } catch {
        // Dead conns are reaped by the server close/pong path.
      }
    }
  }

  broadcast(message: Uint8Array): void {
    for (const conn of this.conns.keys()) {
      if (conn.readyState !== readyStateOpen) continue
      try {
        conn.send(message)
      } catch {
        // Dead conns are reaped by the server close/pong path.
      }
    }
  }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    this.conns.clear()
    this.doc.destroy()
  }
}
