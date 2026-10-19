import { test } from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'
import { internalFetch } from './internalHttp.js'
import { admitToRoom } from '../admission/admit.js'

async function serve(handler: http.RequestListener, run: (url: string) => Promise<void>) {
  const server = http.createServer(handler)
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  assert.ok(address && typeof address !== 'string')
  try {
    await run(`http://127.0.0.1:${address.port}`)
  } finally {
    server.closeAllConnections()
    await new Promise<void>((resolve) => server.close(() => resolve()))
  }
}

test(
  'internal HTTP rejects a response interrupted after its headers',
  { timeout: 2_000 },
  async () => {
    await serve(
      (_req, res) => {
        res.writeHead(200, { 'content-length': '100' })
        res.write('partial')
        setImmediate(() => res.destroy())
      },
      async (url) => {
        await assert.rejects(internalFetch(url), /aborted|reset/i)
      },
    )
  },
)

test('internal HTTP aborts while waiting for the response body', { timeout: 2_000 }, async () => {
  const controller = new AbortController()
  await serve(
    (_req, res) => {
      res.writeHead(200)
      res.write('partial')
      setImmediate(() => controller.abort())
    },
    async (url) => {
      await assert.rejects(internalFetch(url, { signal: controller.signal }), /abort/i)
    },
  )
})

test('internal HTTP preserves complete responses after a later abort', async () => {
  const controller = new AbortController()
  await serve(
    (_req, res) => res.end('complete'),
    async (url) => {
      const response = await internalFetch(url, { signal: controller.signal })
      assert.equal(response.status, 200)
      assert.equal(await response.text(), 'complete')
      controller.abort()
      assert.equal(await response.text(), 'complete')
    },
  )
})

test('admission timeout denies a stalled real HTTP response body', { timeout: 2_000 }, async () => {
  await serve(
    (_req, res) => {
      res.writeHead(200)
      res.write('{"code":0,')
    },
    async (url) => {
      const result = await admitToRoom({
        apiInternalURL: url,
        internalKey: 'test-internal-key',
        ticket: 'test-ticket',
        timeoutMs: 20,
      })
      assert.deepEqual(result, { admitted: false, status: 0 })
    },
  )
})
