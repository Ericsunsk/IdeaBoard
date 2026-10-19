// Single-replica durable rooms. Recovery, admission reservations, persistence
// ordering, compaction, and final disconnect all share one Board lifetime.
import * as Y from 'yjs'
import { PresenceRoom, type SendableConn } from './protocol.js'
import { buildYCompactEnvelope, sanitizeYSnapshot } from './ydoc.js'

export type RoomPersistence = {
  load: (ticket: string) => Promise<{ snapshot: unknown; updates: readonly Uint8Array[] }>
  append: (bid: string, update: Uint8Array, authorUid: string) => Promise<boolean>
  compactNow: (bid: string, snapshot: string, authorUid: string) => Promise<boolean>
  close: () => void
}

export type RoomIdentity = { uid: string; canWrite: boolean }
export type RoomConnection = {
  initialMessages: () => Uint8Array[]
  receive: (message: Uint8Array) => Uint8Array | null
  close: () => void
}
export type RoomAdmission = {
  attach: (conn: SendableConn, identity: RoomIdentity) => RoomConnection
  cancel: () => void
}
type Entry = {
  room: PresenceRoom
  ready: Promise<void>
  pending: Promise<void>
  identities: Map<SendableConn, RoomIdentity>
  reservations: number
  depth: number
  updatesSinceCompact: number
  failed: boolean
}

const MAX_PERSISTENCE_DEPTH = 1000

export class DurableRooms {
  private readonly entries = new Map<string, Entry>()
  private readonly compactEveryUpdates: number
  private closed = false
  private compactSkippedTotal = 0

  constructor(
    private readonly options: {
      persistence: RoomPersistence
      compactEveryUpdates?: number
    },
  ) {
    this.compactEveryUpdates = Math.max(1, options.compactEveryUpdates ?? 50)
  }

  /** Reserve a recovered room before HTTP upgrades into a live socket. */
  async prepare(bid: string, ticket: string): Promise<RoomAdmission> {
    if (this.closed) throw new Error('rooms are closed')
    let entry = this.entries.get(bid)
    if (!entry) {
      entry = this.createEntry(bid, ticket)
      this.entries.set(bid, entry)
    }
    const current = entry
    current.reservations += 1
    const cancel = () => {
      if (!reserved) return
      reserved = false
      current.reservations -= 1
      this.prune(bid, current)
    }
    let reserved = true
    try {
      await current.ready
      // An empty room remains alive while writes drain. A reconnect cannot
      // start a new editing session ahead of those durable acknowledgements.
      if (current.room.connectionCount === 0 || current.failed) await current.pending
      if (current.failed) {
        cancel()
        if (this.entries.get(bid) === current) this.entries.delete(bid)
        return this.prepare(bid, ticket)
      }
      if (this.closed) throw new Error('rooms are closed')
      return {
        attach: (conn, identity) => {
          if (!reserved || this.closed || current.failed)
            throw new Error('room admission is closed')
          current.room.addConn(conn)
          current.room.setWriter(conn, identity.canWrite)
          current.identities.set(conn, identity)
          cancel()
          let connected = true
          return {
            initialMessages: () => (connected ? current.room.initialMessages() : []),
            receive: (message) => (connected ? current.room.handleMessage(conn, message) : null),
            close: () => {
              if (!connected) return
              connected = false
              current.identities.delete(conn)
              current.room.removeConn(conn)
              this.prune(bid, current)
            },
          }
        },
        cancel,
      }
    } catch (error) {
      cancel()
      throw error
    }
  }

  get metrics(): {
    rooms: number
    connections: number
    ignoredDocUpdates: number
    appliedDocUpdates: number
    compactSkippedTotal: number
  } {
    let connections = 0
    let ignoredDocUpdates = 0
    let appliedDocUpdates = 0
    for (const { room } of this.entries.values()) {
      connections += room.connectionCount
      ignoredDocUpdates += room.ignoredDocUpdates
      appliedDocUpdates += room.appliedDocUpdates
    }
    return {
      rooms: this.entries.size,
      connections,
      ignoredDocUpdates,
      appliedDocUpdates,
      compactSkippedTotal: this.compactSkippedTotal,
    }
  }

  /** Stop admission, drain accepted writes, and release the entire registry. */
  async close(): Promise<void> {
    this.closed = true
    await Promise.allSettled(
      [...this.entries.values()].map(async (entry) => {
        await entry.ready
        await entry.pending
        entry.room.destroy()
      }),
    )
    this.entries.clear()
    this.options.persistence.close()
  }

  private createEntry(bid: string, ticket: string): Entry {
    const room = new PresenceRoom(bid, {
      canAcceptDocUpdate: (sender) => {
        if (!this.closed && !entry.failed && entry.depth < MAX_PERSISTENCE_DEPTH) return true
        sender.close(
          1013,
          entry.failed ? 'document persistence unavailable' : 'document persistence queue is full',
        )
        return false
      },
      onDocUpdate: (update, sender) => {
        const identity = entry.identities.get(sender)
        if (!identity) return
        entry.depth += 1
        entry.updatesSinceCompact += 1
        // Capture at acceptance, before a later queued update can enter the
        // Doc. Compaction must never include a not-yet-acknowledged tail.
        let snapshot: string | null = null
        if (entry.updatesSinceCompact >= this.compactEveryUpdates) {
          try {
            snapshot = buildYCompactEnvelope(
              1,
              sanitizeYSnapshot(
                room.doc.getMap('nodes').toJSON(),
                room.doc.getMap('edges').toJSON(),
              ),
              Y.encodeStateAsUpdate(room.doc),
            )
            entry.updatesSinceCompact = 0
          } catch {
            // An invalid snapshot does not affect its opaque update append.
            // Keep the counter so a later update retries compaction.
            this.compactSkippedTotal += 1
          }
        }
        entry.pending = entry.pending
          .then(async () => {
            if (entry.failed) return
            const persisted = await this.options.persistence.append(bid, update, identity.uid)
            if (!persisted) {
              this.failEntry(entry)
              return
            }
            if (snapshot === null) return
            await this.options.persistence.compactNow(bid, snapshot, identity.uid)
          })
          .catch(() => {
            // A rejected update is already in the in-memory Doc. Never compact
            // it later or append updates that might depend on its CRDT structs.
            this.failEntry(entry)
          })
          .finally(() => {
            entry.depth -= 1
            this.prune(bid, entry)
          })
      },
    })
    const entry: Entry = {
      room,
      ready: Promise.resolve(),
      pending: Promise.resolve(),
      identities: new Map(),
      reservations: 0,
      depth: 0,
      updatesSinceCompact: 0,
      failed: false,
    }
    entry.ready = Promise.resolve()
      .then(() => this.options.persistence.load(ticket))
      .then(({ snapshot, updates }) => {
        room.hydrate(snapshot, updates)
      })
      .catch((error: unknown) => {
        if (this.entries.get(bid) === entry) this.entries.delete(bid)
        room.destroy()
        throw error
      })
    return entry
  }

  private failEntry(entry: Entry): void {
    entry.failed = true
    for (const conn of entry.identities.keys()) {
      try {
        conn.close(1013, 'document persistence unavailable')
      } catch {
        /* Socket cleanup can finish on the regular close path. */
      }
    }
  }

  private prune(bid: string, entry: Entry): void {
    if (entry.reservations > 0 || entry.depth > 0 || entry.room.connectionCount > 0) return
    if (this.entries.get(bid) === entry) this.entries.delete(bid)
    entry.room.destroy()
  }
}
