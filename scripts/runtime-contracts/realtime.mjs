const hardTimeout = setTimeout(() => {
  console.error('networkfixture timedout')
  process.exit(2)
}, 12000)
hardTimeout.unref()
import assert from 'node:assert/strict'
import http from 'node:http'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { WebSocket } from '/app/node_modules/ws/wrapper.mjs'
import { signTicket } from '/app/dist/admission/ticket.js'
import * as Y from '/app/node_modules/yjs/dist/yjs.mjs'
import * as encoding from '/app/node_modules/lib0/encoding.js'
import * as sync from '/app/node_modules/y-protocols/sync.js'
const secret = 'local-poc-realtime-ticket-secret-123456'
let appends = 0
let admits = 0
const api = http.createServer((req, res) => {
  assert.equal(req.headers['x-internal-key'], 'local-poc-auth-internal-key-123456789')
  let b = ''
  req.on('data', (c) => (b += c))
  req.on('end', () => {
    res.setHeader('content-type', 'application/json')
    if (new URL(req.url, 'http://api').pathname.endsWith('/admit')) {
      admits++
      res.end(JSON.stringify({ code: 0, data: { admit: true } }))
    } else if (new URL(req.url, 'http://api').pathname.endsWith('/document'))
      res.end(
        JSON.stringify({
          code: 0,
          data: { snapshot: { schema_version: 1, payload: {} }, updates: [] },
        }),
      )
    else if (new URL(req.url, 'http://api').pathname.endsWith('/y-updates')) {
      appends++
      res.end(JSON.stringify({ code: 0 }))
    } else res.end(JSON.stringify({ code: 0 }))
  })
})
await new Promise((r) => api.listen(6000, '127.0.0.1', r))
const child = spawn('/usr/local/bin/bun', ['dist/index.js'], {
  cwd: '/app',
  env: {
    ...process.env,
    NODE_ENV: 'production',
    RT_HOST: '127.0.0.1',
    RT_PORT: '6002',
    RT_TICKET_SECRET: secret,
    AUTH_INTERNAL_KEY: 'local-poc-auth-internal-key-123456789',
    API_INTERNAL_URL: 'http://127.0.0.1:6000',
  },
  stdio: ['ignore', 'pipe', 'pipe'],
})
let output = ''
child.stdout.on('data', (c) => (output += c))
child.stderr.on('data', (c) => (output += c))
let ws
try {
  const end = Date.now() + 6000
  let healthy = false
  while (Date.now() < end) {
    try {
      const r = await fetch('http://127.0.0.1:6002/health')
      healthy = r.status === 200
      if (healthy) break
    } catch {}
    await new Promise((r) => setTimeout(r, 30))
  }
  assert.equal(healthy, true, output)
  console.log('production health200')
  const bad = new WebSocket('ws://127.0.0.1:6002/realtime?ticket=invalid')
  const status = await new Promise((r, j) => {
    bad.once('close', r)
    bad.once('error', j)
    setTimeout(() => j(Error('invalidticket close timedout')), 3000).unref()
  })
  assert.equal(status, 4401)
  console.log('invalidticket rejected', status)
  const ticket = signTicket(
    {
      bid: 'board-poc',
      uid: 'user-poc',
      av: 1,
      cap: 'write',
      exp: Math.floor(Date.now() / 1000) + 60,
    },
    secret,
  )
  ws = new WebSocket('ws://127.0.0.1:6002/realtime?ticket=' + ticket)
  const first = once(ws, 'message')
  await once(ws, 'open')
  const hello = encoding.createEncoder()
  encoding.writeVarUint(hello, 0)
  const empty = new Y.Doc()
  sync.writeSyncStep1(hello, empty)
  empty.destroy()
  ws.send(encoding.toUint8Array(hello))
  const [msg] = await first
  assert.ok(msg.byteLength > 0)
  assert.equal(admits, 1)
  console.log('signedticket upgrade+binarysync passed')
  const pong = once(ws, 'pong')
  ws.ping('poc')
  await pong
  console.log('pingpong passed')
  const doc = new Y.Doc()
  doc.getMap('nodes').set('n1', { id: 'n1', type: 'image', data: { id: 'n1' } })
  const frame = encoding.createEncoder()
  encoding.writeVarUint(frame, 0)
  sync.writeUpdate(frame, Y.encodeStateAsUpdate(doc))
  ws.send(encoding.toUint8Array(frame))
  const deadline = Date.now() + 3000
  while (appends === 0 && Date.now() < deadline) await new Promise((r) => setTimeout(r, 10))
  assert.equal(appends, 1)
  doc.destroy()
  console.log('binaryupdate persisted overHTTP')
  ws.close(1000)
  await once(ws, 'close')
  child.kill('SIGTERM')
  const [code] = await once(child, 'exit')
  assert.equal(code, 0, output)
  console.log('wsclose+SIGTERM drain exit0')
} finally {
  ws?.terminate()
  if (child.exitCode === null) child.kill('SIGKILL')
  await new Promise((r) => api.close(r))
}
