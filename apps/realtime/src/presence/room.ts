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
  updateSequence: number
  compactedSequence: number
  lastWriterUid: string | null
  invalidSnapshotSequence: number | null
  compactTimer: ReturnType<typeof setTimeout> | undefined
  failed: boolean
}

const MAX_PERSISTENCE_DEPTH = 1000
const MAX_COMPACT_DELAY_MS = 1000

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
          // Recovery has no historical author at this seam. An admitted editor
          // may materialize the durable tail as a new snapshot revision; a
          // viewer must never be promoted into its write actor.
          if (identity.canWrite && current.updateSequence > current.compactedSequence) {
            current.lastWriterUid = identity.uid
            this.scheduleCompaction(bid, current)
          }
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
              if (current.room.connectionCount === 0) this.queueCompaction(bid, current)
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
      [...this.entries.entries()].map(async ([bid, entry]) => {
        await entry.ready
        this.queueCompaction(bid, entry)
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
        entry.updateSequence += 1
        entry.lastWriterUid = identity.uid
        entry.pending = entry.pending
          .then(async () => {
            if (entry.failed) return
            const persisted = await this.options.persistence.append(bid, update, identity.uid)
            if (!persisted) {
              this.failEntry(entry)
              return
            }
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
        if (entry.updatesSinceCompact >= this.compactEveryUpdates) {
          this.queueCompaction(bid, entry)
        } else {
          this.scheduleCompaction(bid, entry)
        }
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
      updateSequence: 0,
      compactedSequence: 0,
      lastWriterUid: null,
      invalidSnapshotSequence: null,
      compactTimer: undefined,
      failed: false,
    }
    entry.ready = Promise.resolve()
      .then(() => this.options.persistence.load(ticket))
      .then(({ snapshot, updates }) => {
        room.hydrate(snapshot, updates)
        entry.updateSequence = updates.length
        entry.updatesSinceCompact = updates.length
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
    this.clearCompactionTimer(entry)
    for (const conn of entry.identities.keys()) {
      try {
        conn.close(1013, 'document persistence unavailable')
      } catch {
        /* Socket cleanup can finish on the regular close path. */
      }
    }
  }

  private scheduleCompaction(bid: string, entry: Entry): void {
    // Do not debounce: continuous edits and connected viewers must not defer
    // projection indefinitely. All triggers use the same persistence queue.
    if (this.closed || entry.failed || entry.compactTimer || !entry.lastWriterUid) return
    entry.compactTimer = setTimeout(() => {
      entry.compactTimer = undefined
      if (this.entries.get(bid) === entry && !this.closed) this.queueCompaction(bid, entry)
    }, MAX_COMPACT_DELAY_MS)
    entry.compactTimer.unref()
  }

  private clearCompactionTimer(entry: Entry): void {
    clearTimeout(entry.compactTimer)
    entry.compactTimer = undefined
  }

  private queueCompaction(bid: string, entry: Entry): void {
    this.clearCompactionTimer(entry)
    const authorUid = entry.lastWriterUid
    const sequence = entry.updateSequence
    if (!authorUid || entry.failed || sequence <= entry.compactedSequence) return
    // An admission reserved before disconnect may attach while this drains.
    // Freeze the snapshot now so later, unacknowledged edits cannot enter it.
    const snapshot = this.captureSnapshot(entry)
    if (snapshot === null) return
    entry.updatesSinceCompact = 0
    entry.depth += 1
    entry.pending = entry.pending
      .then(async () => {
        if (entry.failed || sequence <= entry.compactedSequence) return
        if (await this.options.persistence.compactNow(bid, snapshot, authorUid)) {
          entry.compactedSequence = sequence
        }
      })
      // The opaque tail is already durable. A failed projection must not
      // discard it or prevent room cleanup and later recovery.
      .catch(() => {})
      .finally(() => {
        entry.depth -= 1
        this.prune(bid, entry)
      })
  }

  private captureSnapshot(entry: Entry): string | null {
    if (entry.invalidSnapshotSequence === entry.updateSequence) return null
    try {
      return buildYCompactEnvelope(
        1,
        sanitizeYSnapshot(
          entry.room.doc.getMap('nodes').toJSON(),
          entry.room.doc.getMap('edges').toJSON(),
        ),
        Y.encodeStateAsUpdate(entry.room.doc),
      )
    } catch {
      // The opaque update remains durable; retry only after another edit
      // can change the invalid document, never again for the same state.
      entry.invalidSnapshotSequence = entry.updateSequence
      this.compactSkippedTotal += 1
      return null
    }
  }

  private prune(bid: string, entry: Entry): void {
    if (entry.reservations > 0 || entry.depth > 0 || entry.room.connectionCount > 0) return
    this.clearCompactionTimer(entry)
    if (this.entries.get(bid) === entry) this.entries.delete(bid)
    entry.room.destroy()
  }
}
