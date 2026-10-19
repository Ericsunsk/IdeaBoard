import { it } from 'node:test'
import assert from 'node:assert/strict'
import { once } from 'node:events'
import { WebSocket } from 'ws'
import * as Y from 'yjs'
import * as encoding from 'lib0/encoding'
import * as sync from 'y-protocols/sync'
import { signTicket } from '../admission/ticket.js'
import { createRealtimeServer } from './server.js'

async function until(ready: () => boolean): Promise<void> {
  const deadline = Date.now() + 3000
  while (!ready()) {
    if (Date.now() > deadline) throw new Error('condition timed out')
    await new Promise((resolve) => setTimeout(resolve, 5))
  }
}

it('serializes appends and compactions, and drains persistence before room recovery', async () => {
  const calls: string[] = []
  let loads = 0
  let release!: () => void
  const blocked = new Promise<void>((resolve) => {
    release = resolve
  })
  const server = createRealtimeServer({
    host: '127.0.0.1',
    port: 0,
    ticketSecret: 'test-secret',
    internalKey: 'internal',
    apiInternalURL: 'http://api.invalid',
    compactEveryUpdates: 1,
    fetchFn: async (input, init) => {
      const path = new URL(String(input)).pathname
      if (path.endsWith('/admit')) return Response.json({ code: 0, data: { admit: true } })
      if (path.endsWith('/document')) {
        loads += 1
        return Response.json({
          code: 0,
          data: { snapshot: { schema_version: 1, payload: {} }, updates: [] },
        })
      }
      if (path.endsWith('/y-updates')) {
        calls.push('append')
        if (calls.length === 1) await blocked
      } else if (path.endsWith('/compactions')) {
        assert.ok(JSON.parse(String(init?.body)).snapshot.payload.y_state)
        calls.push('compact')
      } else throw new Error(`unexpected path ${path}`)
      return Response.json({ code: 0 })
    },
  })
  server.httpServer.listen(0, '127.0.0.1')
  await once(server.httpServer, 'listening')
  const address = server.httpServer.address()
  assert.ok(address && typeof address !== 'string')
  const ticket = signTicket(
    {
      bid: 'board-ordered',
      uid: 'user-1',
      av: 1,
      cap: 'write',
      exp: Math.floor(Date.now() / 1000) + 60,
    },
    'test-secret',
  )
  const url = `ws://127.0.0.1:${address.port}/realtime?ticket=${ticket}`
  const first = new WebSocket(url)
  let second: WebSocket | undefined
  const writer = new Y.Doc()
  try {
    await once(first, 'open')
    const send = (key: string) => {
      const vector = Y.encodeStateVector(writer)
      writer.getMap('nodes').set(key, { id: key, type: 'image', data: { id: key } })
      const frame = encoding.createEncoder()
      encoding.writeVarUint(frame, 0)
      sync.writeUpdate(frame, Y.encodeStateAsUpdate(writer, vector))
      first.send(encoding.toUint8Array(frame))
    }
    send('image-1')
    send('image-2')
    await until(() => calls.length === 1)
    const health = await fetch(`http://127.0.0.1:${address.port}/health`).then((response) =>
      response.json(),
    )
    assert.equal(health.data.appliedDocUpdates, 2)
    assert.deepEqual(calls, ['append'])
    first.close()
    await once(first, 'close')
    second = new WebSocket(url)
    const opened = once(second, 'open')
    await until(() => server.metrics.admittedTotal === 2)
    assert.equal(loads, 1)
    release()
    await opened
    assert.deepEqual(calls, ['append', 'compact', 'append', 'compact'])
    assert.equal(loads, 1)
  } finally {
    release()
    first.terminate()
    second?.terminate()
    writer.destroy()
    await server.close()
  }
})
