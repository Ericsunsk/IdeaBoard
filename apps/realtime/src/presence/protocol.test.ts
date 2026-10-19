import { buildYCompactEnvelope, sanitizeYSnapshot } from './ydoc.js'
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import * as Y from 'yjs'
import * as syncProtocol from 'y-protocols/sync'
import * as awarenessProtocol from 'y-protocols/awareness'
import * as encoding from 'lib0/encoding'
import * as decoding from 'lib0/decoding'
import { PresenceRoom, messageAwareness, messageSync, type SendableConn } from './protocol.js'

function fakeConn(): SendableConn & { sent: Uint8Array[] } {
  return {
    readyState: 1,
    close() {},
    sent: [],
    send(data: Uint8Array) {
      this.sent.push(data)
    },
  }
}

function frameSyncStep1(): Uint8Array {
  const encoder = encoding.createEncoder()
  encoding.writeVarUint(encoder, messageSync)
  encoding.writeVarUint(encoder, syncProtocol.messageYjsSyncStep1)
  const emptyDoc = new Y.Doc()
  try {
    encoding.writeVarUint8Array(encoder, Y.encodeStateVector(emptyDoc))
  } finally {
    emptyDoc.destroy()
  }
  return encoding.toUint8Array(encoder)
}

function frameDocUpdate(update: Uint8Array): Uint8Array {
  const encoder = encoding.createEncoder()
  encoding.writeVarUint(encoder, messageSync)
  syncProtocol.writeUpdate(encoder, update)
  return encoding.toUint8Array(encoder)
}

function frameAwarenessUpdate(update: Uint8Array): Uint8Array {
  const encoder = encoding.createEncoder()
  encoding.writeVarUint(encoder, messageAwareness)
  encoding.writeVarUint8Array(encoder, update)
  return encoding.toUint8Array(encoder)
}

describe('PresenceRoom', () => {
  it('hydrates the canonical empty revision-one document', () => {
    const room = new PresenceRoom('board-new')
    room.hydrate({ schema_version: 1, payload: {} }, [])
    assert.equal(room.doc.getMap('nodes').size, 0)
    room.destroy()
  })
  it('replays updates referencing compacted clocks and preserves deletions after recreation', () => {
    const original = new PresenceRoom('board-recovered')
    const seed = {
      schema_version: 4,
      payload: {
        imageNodes: [
          { id: 1, x: 0, y: 0 },
          { id: 2, x: 0, y: 0 },
        ],
        connections: [],
      },
    }
    original.hydrate(seed, [])
    const writer = new Y.Doc()
    Y.applyUpdate(writer, Y.encodeStateAsUpdate(original.doc))
    writer.getMap('nodes').delete('image-1')
    const before = Y.encodeStateVector(writer)
    const state = Y.encodeStateAsUpdate(writer)
    const snapshot = JSON.parse(
      buildYCompactEnvelope(1, sanitizeYSnapshot(writer.getMap('nodes').toJSON(), {}), state),
    ) as unknown
    writer
      .getMap('nodes')
      .set('image-2', { id: 'image-2', type: 'image', position: { x: 9, y: 10 }, data: { id: 2 } })
    const tail = Y.encodeStateAsUpdate(writer, before)
    const recreated = new PresenceRoom('board-recovered')
    recreated.hydrate(snapshot, [tail])
    assert.deepEqual(recreated.doc.getMap('nodes').toJSON(), writer.getMap('nodes').toJSON())
    assert.equal(recreated.doc.getMap('nodes').has('image-1'), false)
    writer.destroy()
    original.destroy()
    recreated.destroy()
  })

  it('replays deletions against the stable first-room seed identity', () => {
    const seed = {
      schema_version: 4,
      payload: { imageNodes: [{ id: 1, x: 0, y: 0 }], connections: [] },
    }
    const original = new PresenceRoom('board-seed')
    original.hydrate(seed, [])
    const before = Y.encodeStateVector(original.doc)
    original.doc.getMap('nodes').delete('image-1')
    const tail = Y.encodeStateAsUpdate(original.doc, before)
    const recreated = new PresenceRoom('board-seed')
    recreated.hydrate(seed, [tail])
    assert.equal(recreated.doc.getMap('nodes').has('image-1'), false)
    original.destroy()
    recreated.destroy()
  })

  it('seeds a first collaboration room from the persisted Board document', () => {
    const room = new PresenceRoom('board-seed')
    room.hydrate(
      {
        schema_version: 4,
        payload: {
          imageNodes: [{ id: 2, type: 'image-node', x: 3, y: 4 }],
          promptNodes: [],
          fusionNodes: [],
          groups: [],
          connections: [],
        },
      },
      [],
    )
    assert.equal((room.doc.getMap('nodes').get('image-2') as { type: string }).type, 'image')
    room.destroy()
  })
  it('answers SyncStep1 without storing document content', () => {
    const room = new PresenceRoom('board-1')
    const conn = fakeConn()
    room.addConn(conn)
    const reply = room.handleMessage(conn, frameSyncStep1())
    assert.ok(reply !== null)
    const decoder = decoding.createDecoder(reply!)
    assert.equal(decoding.readVarUint(decoder), messageSync)
    assert.equal(decoding.readVarUint(decoder), syncProtocol.messageYjsSyncStep2)
    assert.equal(room.ignoredDocUpdates, 0)
    assert.equal(room.appliedDocUpdates, 0)
    room.destroy()
  })

  it('applies writer updates, broadcasts to peers, and hands them to persistence', () => {
    const persisted: Uint8Array[] = []
    const room = new PresenceRoom('board-1', {
      onDocUpdate: (update) => {
        persisted.push(update)
      },
    })
    const writerConn = fakeConn()
    const peerConn = fakeConn()
    room.addConn(writerConn)
    room.addConn(peerConn)
    room.setWriter(writerConn, true)
    const writer = new Y.Doc()
    writer.getMap('nodes').set('n1', { id: 'n1', type: 'image' })
    writer.getMap('edges').set('e1', { id: 'e1', source: 'n1', target: 'n1' })
    const update = Y.encodeStateAsUpdate(writer)
    assert.equal(room.handleMessage(writerConn, frameDocUpdate(update)), null)
    assert.equal(room.appliedDocUpdates, 1)
    assert.equal(room.ignoredDocUpdates, 0)
    assert.deepEqual(Array.from(room.doc.getMap('nodes').keys()), ['n1'])
    assert.equal(persisted.length, 1)
    // Peers converge; the sender is not echoed.
    assert.equal(peerConn.sent.length, 1)
    const peerDecoder = decoding.createDecoder(peerConn.sent[0]!)
    assert.equal(decoding.readVarUint(peerDecoder), messageSync)
    assert.equal(decoding.readVarUint(peerDecoder), syncProtocol.messageYjsUpdate)
    assert.equal(writerConn.sent.length, 0)
    writer.destroy()
    room.destroy()
  })

  it('drops document updates from read-only connections', () => {
    let persisted = 0
    const room = new PresenceRoom('board-1', {
      onDocUpdate: () => {
        persisted += 1
      },
    })
    const readerConn = fakeConn()
    const peerConn = fakeConn()
    room.addConn(readerConn)
    room.addConn(peerConn)
    const writer = new Y.Doc()
    writer.getMap('nodes').set('n1', { id: 'n1', type: 'image' })
    const update = Y.encodeStateAsUpdate(writer)
    assert.equal(room.handleMessage(readerConn, frameDocUpdate(update)), null)
    assert.equal(room.ignoredDocUpdates, 1)
    assert.equal(room.appliedDocUpdates, 0)
    assert.equal(persisted, 0)
    assert.equal(room.doc.getMap('nodes').size, 0)
    assert.equal(peerConn.sent.length, 0)
    writer.destroy()
    room.destroy()
  })

  it('drops truncated update frames and counts them', () => {
    const room = new PresenceRoom('board-1')
    const conn = fakeConn()
    room.addConn(conn)
    room.setWriter(conn, true)
    const encoder = encoding.createEncoder()
    encoding.writeVarUint(encoder, messageSync)
    encoding.writeVarUint(encoder, syncProtocol.messageYjsUpdate)
    assert.equal(room.handleMessage(conn, encoding.toUint8Array(encoder)), null)
    assert.equal(room.ignoredDocUpdates, 1)
    assert.equal(room.appliedDocUpdates, 0)
    room.destroy()
  })

  it('relays standard awareness updates to room members', () => {
    const room = new PresenceRoom('board-1')
    const first = fakeConn()
    const second = fakeConn()
    room.addConn(first)
    room.addConn(second)
    const source = new awarenessProtocol.Awareness(new Y.Doc())
    source.setLocalState({ cursor: { x: 10, y: 20 } })
    const update = awarenessProtocol.encodeAwarenessUpdate(source, [source.clientID])
    assert.equal(room.handleMessage(first, frameAwarenessUpdate(update)), null)
    assert.equal(room.awareness.getStates().size, 1)
    assert.ok(first.sent.length >= 1)
    assert.ok(second.sent.length >= 1)
    source.destroy()
    room.destroy()
  })

  it('removes awareness states when the controlling conn leaves', () => {
    const room = new PresenceRoom('board-1')
    const conn = fakeConn()
    room.addConn(conn)
    const source = new awarenessProtocol.Awareness(new Y.Doc())
    source.setLocalState({ cursor: { x: 1, y: 2 } })
    room.handleMessage(
      conn,
      frameAwarenessUpdate(awarenessProtocol.encodeAwarenessUpdate(source, [source.clientID])),
    )
    assert.equal(room.awareness.getStates().size, 1)
    room.removeConn(conn)
    assert.equal(room.awareness.getStates().size, 0)
    source.destroy()
    room.destroy()
  })
})
