import { it } from 'node:test'
import assert from 'node:assert/strict'
import * as Y from 'yjs'
import * as encoding from 'lib0/encoding'
import * as decoding from 'lib0/decoding'
import * as sync from 'y-protocols/sync'
import { DurableRooms, type RoomPersistence, type RoomConnection } from './room.js'
import type { SendableConn } from './protocol.js'

function deferred() {
  let resolve!: () => void
  let reject!: (reason: Error) => void
  const promise = new Promise<void>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

function socket(): SendableConn & { sent: Uint8Array[]; closed: number[] } {
  return {
    readyState: 1,
    sent: [],
    closed: [],
    send(message) {
      this.sent.push(message)
    },
    close(code) {
      this.closed.push(code)
    },
  }
}

function memoryPersistence() {
  let snapshot: unknown = { schema_version: 1, payload: {} }
  let updates: Uint8Array[] = []
  const calls: string[] = []
  const snapshots: string[] = []
  let loads = 0
  let closed = false
  let blockAppend: Promise<void> | undefined
  let blockLoad: Promise<void> | undefined
  const persistence: RoomPersistence = {
    async load() {
      loads += 1
      if (blockLoad) await blockLoad
      return { snapshot, updates: [...updates] }
    },
    async append(_bid, update, uid) {
      calls.push(`append:${uid}`)
      if (blockAppend) await blockAppend
      updates.push(update.slice())
      return true
    },
    async compactNow(_bid, value, uid) {
      calls.push(`compact:${uid}`)
      snapshots.push(value)
      snapshot = JSON.parse(value)
      updates = []
      return true
    },
    close() {
      closed = true
    },
  }
  return {
    persistence,
    calls,
    snapshots,
    get loads() {
      return loads
    },
    get closed() {
      return closed
    },
    set blockAppend(value: Promise<void> | undefined) {
      blockAppend = value
    },
    set blockLoad(value: Promise<void> | undefined) {
      blockLoad = value
    },
  }
}

function updateFrame(writer: Y.Doc, key: string): Uint8Array {
  const before = Y.encodeStateVector(writer)
  writer.getMap('nodes').set(key, { id: key, type: 'image', data: { id: key } })
  const encoder = encoding.createEncoder()
  encoding.writeVarUint(encoder, 0)
  sync.writeUpdate(encoder, Y.encodeStateAsUpdate(writer, before))
  return encoding.toUint8Array(encoder)
}

function readNodes(connection: RoomConnection): Record<string, unknown> {
  const reader = new Y.Doc()
  try {
    const frame = encoding.createEncoder()
    encoding.writeVarUint(frame, 0)
    sync.writeSyncStep1(frame, reader)
    const response = connection.receive(encoding.toUint8Array(frame))
    assert.ok(response)
    const decoder = decoding.createDecoder(response)
    assert.equal(decoding.readVarUint(decoder), 0)
    assert.equal(decoding.readVarUint(decoder), sync.messageYjsSyncStep2)
    sync.readSyncStep2(decoder, reader, null)
    return reader.getMap('nodes').toJSON()
  } finally {
    reader.destroy()
  }
}

it('materializes short edits within one second while a viewer keeps the room alive', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const storage = memoryPersistence()
  const rooms = new DurableRooms({ persistence: storage.persistence })
  const writer = new Y.Doc()
  try {
    const connection = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'writer',
      canWrite: true,
    })
    const viewer = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'reader',
      canWrite: false,
    })
    connection.receive(updateFrame(writer, 'image-1'))
    t.mock.timers.tick(600)
    connection.receive(updateFrame(writer, 'image-2'))
    connection.close()
    t.mock.timers.tick(400)
    await new Promise(setImmediate)
    assert.equal(rooms.metrics.connections, 1)
    assert.equal(storage.snapshots.length, 1)
    assert.deepEqual(Object.keys(JSON.parse(storage.snapshots[0]!).payload.nodes), [
      'image-1',
      'image-2',
    ])
    viewer.close()
  } finally {
    writer.destroy()
    await rooms.close()
  }
})

it('materializes a restored durable tail for an admitted editor without requiring new edits', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const storage = memoryPersistence()
  const seed = new Y.Doc()
  updateFrame(seed, 'restored-image')
  await storage.persistence.append('board', Y.encodeStateAsUpdate(seed), 'original-writer')
  storage.calls.length = 0
  const rooms = new DurableRooms({ persistence: storage.persistence })
  try {
    const viewer = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'reader',
      canWrite: false,
    })
    assert.deepEqual(Object.keys(readNodes(viewer)), ['restored-image'])
    t.mock.timers.tick(1000)
    await new Promise(setImmediate)
    assert.deepEqual(storage.calls, [])
    const editor = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'current-editor',
      canWrite: true,
    })
    t.mock.timers.tick(1000)
    await new Promise(setImmediate)
    assert.deepEqual(storage.calls, ['compact:current-editor'])
    assert.deepEqual(Object.keys(JSON.parse(storage.snapshots[0]!).payload.nodes), [
      'restored-image',
    ])
    editor.close()
    viewer.close()
  } finally {
    seed.destroy()
    await rooms.close()
  }
})

it('never materializes a restored tail when only viewers open and leave', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const storage = memoryPersistence()
  const seed = new Y.Doc()
  updateFrame(seed, 'restored-image')
  await storage.persistence.append('board', Y.encodeStateAsUpdate(seed), 'original-writer')
  storage.calls.length = 0
  const rooms = new DurableRooms({ persistence: storage.persistence })
  try {
    const viewer = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'reader',
      canWrite: false,
    })
    viewer.close()
    t.mock.timers.tick(2000)
    await new Promise(setImmediate)
    assert.equal(rooms.metrics.rooms, 0)
    assert.deepEqual(storage.calls, [])
  } finally {
    seed.destroy()
    await rooms.close()
  }
})

it('keeps timed snapshots behind append acknowledgements and cancels them on terminal failure', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const storage = memoryPersistence()
  const blocked = deferred()
  storage.persistence.append = async () => {
    await blocked.promise
    return false
  }
  const rooms = new DurableRooms({ persistence: storage.persistence })
  const writer = new Y.Doc()
  try {
    const connection = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'writer',
      canWrite: true,
    })
    connection.receive(updateFrame(writer, 'rejected-image'))
    t.mock.timers.tick(1000)
    await new Promise(setImmediate)
    assert.deepEqual(storage.calls, [])
    blocked.resolve()
    await new Promise(setImmediate)
    connection.close()
    t.mock.timers.tick(2000)
    await new Promise(setImmediate)
    assert.equal(rooms.metrics.rooms, 0)
    assert.deepEqual(storage.calls, [])
  } finally {
    blocked.resolve()
    writer.destroy()
    await rooms.close()
  }
})

it('materializes a short editing session before releasing the last connection', async () => {
  const storage = memoryPersistence()
  const rooms = new DurableRooms({ persistence: storage.persistence })
  const writer = new Y.Doc()
  try {
    const connection = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'writer',
      canWrite: true,
    })
    connection.receive(updateFrame(writer, 'image-1'))
    connection.close()
    // A fresh viewer waits for the previous session's durable projection,
    // even though it never reached the default 50-update compact threshold.
    const viewer = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'reader',
      canWrite: false,
    })
    assert.equal(storage.snapshots.length, 1)
    assert.deepEqual(Object.keys(JSON.parse(storage.snapshots[0]!).payload.nodes), ['image-1'])
    assert.equal(rooms.metrics.rooms, 1)
    viewer.close()
  } finally {
    writer.destroy()
    await rooms.close()
  }
})

it('freezes final compaction before a reserved reconnect accepts more edits', async () => {
  const storage = memoryPersistence()
  const append = storage.persistence.append
  const blocked = deferred()
  let appends = 0
  storage.persistence.append = async (bid, update, uid) => {
    if (++appends === 1) {
      await blocked.promise
      return append(bid, update, uid)
    }
    return false
  }
  const rooms = new DurableRooms({ persistence: storage.persistence })
  const writer = new Y.Doc()
  try {
    const first = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'first',
      canWrite: true,
    })
    const reserved = await rooms.prepare('board', 'ticket')
    first.receive(updateFrame(writer, 'image-1'))
    first.close()
    const second = reserved.attach(socket(), { uid: 'second', canWrite: true })
    second.receive(updateFrame(writer, 'image-2'))
    second.close()
    blocked.resolve()
    await rooms.close()
    assert.equal(storage.snapshots.length, 1)
    assert.deepEqual(Object.keys(JSON.parse(storage.snapshots[0]!).payload.nodes), ['image-1'])
    assert.deepEqual(storage.calls, ['append:first', 'compact:first'])
  } finally {
    blocked.resolve()
    writer.destroy()
    await rooms.close()
  }
})

it('materializes unprojected edits during shutdown without waiting for client disconnect', async () => {
  const storage = memoryPersistence()
  const rooms = new DurableRooms({ persistence: storage.persistence })
  const writer = new Y.Doc()
  try {
    const connection = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'writer',
      canWrite: true,
    })
    connection.receive(updateFrame(writer, 'image-1'))
    await rooms.close()
    assert.equal(storage.snapshots.length, 1)
    assert.deepEqual(Object.keys(JSON.parse(storage.snapshots[0]!).payload.nodes), ['image-1'])
    assert.deepEqual(storage.calls, ['append:writer', 'compact:writer'])
  } finally {
    writer.destroy()
    await rooms.close()
  }
})

it('owns append/compact ordering, retains a disconnected Doc until drain, and recovers after pruning', async () => {
  const storage = memoryPersistence()
  const blocked = deferred()
  storage.blockAppend = blocked.promise
  const rooms = new DurableRooms({ persistence: storage.persistence, compactEveryUpdates: 1 })
  const writer = new Y.Doc()
  try {
    const first = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'first',
      canWrite: true,
    })
    first.receive(updateFrame(writer, 'image-1'))
    first.receive(updateFrame(writer, 'image-2'))
    await Promise.resolve()
    assert.deepEqual(storage.calls, ['append:first'])
    first.close()
    let recovered = false
    const pending = rooms.prepare('board', 'ticket').then((admission) => {
      recovered = true
      return admission
    })
    await Promise.resolve()
    assert.equal(recovered, false)
    assert.equal(storage.loads, 1)
    blocked.resolve()
    const second = (await pending).attach(socket(), { uid: 'second', canWrite: false })
    assert.deepEqual(storage.calls, [
      'append:first',
      'compact:first',
      'append:first',
      'compact:first',
    ])
    assert.deepEqual(
      storage.snapshots.map((snapshot) => Object.keys(JSON.parse(snapshot).payload.nodes).length),
      [1, 2],
    )
    assert.equal(Object.keys(readNodes(second)).length, 2)
    second.close()
    assert.equal(rooms.metrics.rooms, 0)
    const third = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'third',
      canWrite: false,
    })
    assert.equal(storage.loads, 2)
    assert.equal(Object.keys(readNodes(third)).length, 2)
    third.close()
  } finally {
    blocked.resolve()
    writer.destroy()
    await rooms.close()
  }
})

it('rejects a full persistence queue before applying or broadcasting the update', async () => {
  const storage = memoryPersistence()
  const blocked = deferred()
  storage.blockAppend = blocked.promise
  // Isolate the append budget from queued snapshot jobs.
  const rooms = new DurableRooms({ persistence: storage.persistence, compactEveryUpdates: 10_000 })
  const writer = new Y.Doc()
  try {
    const sender = socket()
    const peer = socket()
    const first = (await rooms.prepare('board', 'ticket')).attach(sender, {
      uid: 'writer',
      canWrite: true,
    })
    const second = (await rooms.prepare('board', 'ticket')).attach(peer, {
      uid: 'reader',
      canWrite: false,
    })
    for (let i = 0; i < 1000; i += 1) {
      first.receive(updateFrame(writer, `image-${i}`))
    }
    first.receive(updateFrame(writer, 'image-overflow'))
    assert.deepEqual(sender.closed, [1013])
    assert.equal(rooms.metrics.appliedDocUpdates, 1000)
    assert.equal(rooms.metrics.ignoredDocUpdates, 1)
    assert.equal(peer.sent.length, 1000)
    const nodes = readNodes(second)
    assert.equal(Object.keys(nodes).length, 1000)
    assert.equal('image-overflow' in nodes, false)
    first.close()
    second.close()
  } finally {
    blocked.resolve()
    writer.destroy()
    await rooms.close()
  }
})

it('shares concurrent hydration and removes failed admissions so the next attempt can recover', async () => {
  const storage = memoryPersistence()
  const blocked = deferred()
  storage.blockLoad = blocked.promise
  const rooms = new DurableRooms({ persistence: storage.persistence })
  try {
    const first = rooms.prepare('board', 'first-ticket')
    const second = rooms.prepare('board', 'second-ticket')
    const failures = Promise.allSettled([first, second])
    await Promise.resolve()
    assert.equal(storage.loads, 1)
    blocked.reject(new Error('unavailable'))
    assert.deepEqual(
      (await failures).map(({ status }) => status),
      ['rejected', 'rejected'],
    )
    assert.equal(rooms.metrics.rooms, 0)
    storage.blockLoad = undefined
    const admission = await rooms.prepare('board', 'retry-ticket')
    assert.equal(storage.loads, 2)
    admission.cancel()
    assert.equal(rooms.metrics.rooms, 0)
    assert.throws(() => admission.attach(socket(), { uid: 'user', canWrite: true }), /closed/)
  } finally {
    await rooms.close()
  }
})

it('gates readers while keeping protocol handshakes available', async () => {
  const storage = memoryPersistence()
  const rooms = new DurableRooms({ persistence: storage.persistence })
  const writer = new Y.Doc()
  try {
    const connection = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'user',
      canWrite: false,
    })
    assert.equal(connection.initialMessages().length, 1)
    connection.receive(updateFrame(writer, 'image-1'))
    assert.equal(rooms.metrics.appliedDocUpdates, 0)
    assert.equal(rooms.metrics.ignoredDocUpdates, 1)
    assert.deepEqual(storage.calls, [])
    connection.close()
  } finally {
    writer.destroy()
    await rooms.close()
  }
})

it('invalidates the session on terminal append failure and recovers only durable state', async () => {
  const storage = memoryPersistence()
  const blocked = deferred()
  let appends = 0
  storage.persistence.append = async () => {
    appends += 1
    await blocked.promise
    return false
  }
  const rooms = new DurableRooms({ persistence: storage.persistence, compactEveryUpdates: 1 })
  const writer = new Y.Doc()
  let connection: RoomConnection | undefined
  try {
    const sender = socket()
    const failed = deferred()
    sender.close = (code) => {
      sender.closed.push(code)
      failed.resolve()
    }
    connection = (await rooms.prepare('board', 'ticket')).attach(sender, {
      uid: 'user',
      canWrite: true,
    })
    connection.receive(updateFrame(writer, 'image-1'))
    connection.receive(updateFrame(writer, 'image-2'))
    blocked.resolve()
    await failed.promise
    // Fresh admission waits for skipped dependent writes before rebuilding.
    const recovered = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'reader',
      canWrite: false,
    })
    assert.equal(appends, 1)
    assert.deepEqual(storage.calls, [])
    assert.deepEqual(sender.closed, [1013])
    assert.equal(storage.loads, 2)
    assert.deepEqual(readNodes(recovered), {})
    connection.receive(updateFrame(writer, 'image-3'))
    assert.equal(appends, 1)
    recovered.close()
    connection.close()
  } finally {
    blocked.resolve()
    connection?.close()
    writer.destroy()
    await rooms.close()
  }
})

it('never compacts a later rejected update ahead of its append acknowledgement', async () => {
  const storage = memoryPersistence()
  const append = storage.persistence.append
  const blocked = deferred()
  const failed = deferred()
  let appends = 0
  storage.persistence.append = async (bid, update, uid) => {
    appends += 1
    if (appends === 1) {
      await blocked.promise
      return append(bid, update, uid)
    }
    return false
  }
  const rooms = new DurableRooms({ persistence: storage.persistence, compactEveryUpdates: 1 })
  const writer = new Y.Doc()
  let connection: RoomConnection | undefined
  try {
    const sender = socket()
    sender.close = (code) => {
      sender.closed.push(code)
      failed.resolve()
    }
    connection = (await rooms.prepare('board', 'ticket')).attach(sender, {
      uid: 'user',
      canWrite: true,
    })
    connection.receive(updateFrame(writer, 'image-1'))
    connection.receive(updateFrame(writer, 'image-2'))
    blocked.resolve()
    await failed.promise
    const recovered = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'reader',
      canWrite: false,
    })
    assert.deepEqual(Object.keys(readNodes(recovered)), ['image-1'])
    assert.equal(storage.snapshots.length, 1)
    recovered.close()
    connection.close()
  } finally {
    blocked.resolve()
    connection?.close()
    writer.destroy()
    await rooms.close()
  }
})

it('stops admissions and drains accepted writes before closing the persistence adapter', async () => {
  const storage = memoryPersistence()
  const blocked = deferred()
  storage.blockAppend = blocked.promise
  const rooms = new DurableRooms({ persistence: storage.persistence, compactEveryUpdates: 1 })
  const writer = new Y.Doc()
  try {
    const connection = (await rooms.prepare('board', 'ticket')).attach(socket(), {
      uid: 'writer',
      canWrite: true,
    })
    connection.receive(updateFrame(writer, 'image-1'))
    connection.close()
    const closing = rooms.close()
    await assert.rejects(rooms.prepare('board', 'ticket'), /closed/)
    assert.equal(storage.closed, false)
    blocked.resolve()
    await closing
    assert.equal(storage.closed, true)
    assert.deepEqual(storage.calls, ['append:writer', 'compact:writer'])
    assert.equal(rooms.metrics.rooms, 0)
  } finally {
    blocked.resolve()
    writer.destroy()
    await rooms.close()
  }
})

it('retries compaction after a sanitize failure without resetting the counter', async () => {
  const seed = new Y.Doc()
  try {
    for (let i = 0; i < 10_001; i += 1) {
      const key = `seed-${i}`
      seed.getMap('nodes').set(key, { id: key, type: 'image' })
    }
    const snapshot = {
      schema_version: 1,
      payload: {
        nodes: {},
        edges: {},
        y_state: Buffer.from(Y.encodeStateAsUpdate(seed)).toString('base64'),
      },
    }
    const calls: string[] = []
    const persistence: RoomPersistence = {
      async load() {
        return { snapshot, updates: [] }
      },
      async append(_bid, _update, uid) {
        calls.push(`append:${uid}`)
        return true
      },
      async compactNow(_bid, _value, uid) {
        calls.push(`compact:${uid}`)
        return true
      },
      close() {},
    }
    const rooms = new DurableRooms({ persistence, compactEveryUpdates: 2 })
    const writer = new Y.Doc()
    try {
      const connection = (await rooms.prepare('board', 'ticket')).attach(socket(), {
        uid: 'writer',
        canWrite: true,
      })
      connection.receive(updateFrame(writer, 'extra-1'))
      connection.receive(updateFrame(writer, 'extra-2'))
      connection.receive(updateFrame(writer, 'extra-3'))
      // Both attempts past the threshold fail closed on the entry budget and
      // keep the counter so the next update retries instead of waiting.
      assert.equal(rooms.metrics.compactSkippedTotal, 2)
      connection.close()
      await rooms.close()
      assert.deepEqual(calls, ['append:writer', 'append:writer', 'append:writer'])
      assert.equal(rooms.metrics.compactSkippedTotal, 2)
    } finally {
      writer.destroy()
      await rooms.close()
    }
  } finally {
    seed.destroy()
  }
})
