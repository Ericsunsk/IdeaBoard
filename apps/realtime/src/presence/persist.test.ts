import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { YUpdatePersister } from './persist.js'

function okResponse(): Response {
  return { ok: true, status: 200, text: async () => '{}' } as Response
}

function statusResponse(status: number): Response {
  return { ok: false, status, text: async () => '{}' } as Response
}

describe('YUpdatePersister', () => {
  it('persists on first success', async () => {
    let calls = 0
    let body = ''
    const persister = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      baseDelayMs: 1,
      fetchFn: (async (url: unknown, init?: unknown) => {
        calls += 1
        body = (init as { body: string }).body
        assert.ok(String(url).endsWith('/internal/realtime/y-updates'))
        return okResponse()
      }) as typeof fetch,
    })
    assert.equal(await persister.append('board_1', new Uint8Array([1, 2, 3]), 'usr_1'), true)
    assert.equal(calls, 1)
    assert.equal(persister.persistedTotal, 1)
    assert.equal(persister.failedTotal, 0)
    const parsed = JSON.parse(body) as Record<string, string>
    assert.equal(parsed.board_id, 'board_1')
    assert.equal(parsed.author_uid, 'usr_1')
    assert.ok(typeof parsed.update === 'string' && parsed.update.length > 0)
    persister.close()
  })

  it('drops permanent failures without retry', async () => {
    let calls = 0
    const persister = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      baseDelayMs: 1,
      fetchFn: (async () => {
        calls += 1
        return statusResponse(403)
      }) as typeof fetch,
    })
    assert.equal(await persister.append('board_1', new Uint8Array([1]), 'usr_1'), false)
    assert.equal(calls, 1)
    assert.equal(persister.droppedTotal, 1)
    assert.equal(persister.retriedTotal, 0)
    assert.equal(persister.failedTotal, 0)
    persister.close()
  })

  it('retries retryable failures and counts them', async () => {
    let calls = 0
    const persister = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      maxRetries: 2,
      baseDelayMs: 1,
      fetchFn: (async () => {
        calls += 1
        return calls < 3 ? statusResponse(503) : okResponse()
      }) as typeof fetch,
    })
    assert.equal(await persister.append('board_1', new Uint8Array([1]), 'usr_1'), true)
    assert.equal(calls, 3)
    assert.equal(persister.retriedTotal, 2)
    assert.equal(persister.persistedTotal, 1)
    persister.close()
  })

  it('fails closed after exhausting retries', async () => {
    let calls = 0
    const persister = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      maxRetries: 1,
      baseDelayMs: 1,
      fetchFn: (async () => {
        calls += 1
        throw new Error('network down')
      }) as typeof fetch,
    })
    assert.equal(await persister.append('board_1', new Uint8Array([1]), 'usr_1'), false)
    assert.equal(calls, 2)
    assert.equal(persister.failedTotal, 1)
    assert.equal(persister.retriedTotal, 1)
    persister.close()
  })

  it('posts compaction snapshots and counts outcomes', async () => {
    const persister = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      fetchFn: (async () => okResponse()) as typeof fetch,
    })
    const ok = await persister.compactNow(
      'board_1',
      JSON.stringify({ schema_version: 1, payload: { nodes: {}, edges: {} } }),
      'usr_1',
    )
    assert.equal(ok, true)
    assert.equal(persister.compactedTotal, 1)
    const failing = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      maxRetries: 0,
      fetchFn: (async () => statusResponse(500)) as typeof fetch,
    })
    assert.equal(await failing.compactNow('board_1', '{}', 'usr_1'), false)
    assert.equal(failing.compactFailedTotal, 1)
    persister.close()
    failing.close()
  })

  it('rejects a compaction over the byte budget without sending or retrying it', async () => {
    let calls = 0
    const persister = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      fetchFn: async () => {
        calls += 1
        return okResponse()
      },
    })
    try {
      assert.equal(
        await persister.compactNow(
          'board_1',
          JSON.stringify({ value: '汉'.repeat(750_000) }),
          'usr_1',
        ),
        false,
      )
      assert.equal(calls, 0)
      assert.equal(persister.retriedTotal, 0)
      assert.equal(persister.compactFailedTotal, 1)
    } finally {
      persister.close()
    }
  })

  it('retries compaction after transient failures and drops permanent failures', async () => {
    let calls = 0
    const retrying = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      maxRetries: 2,
      baseDelayMs: 1,
      fetchFn: (async () => (++calls === 1 ? statusResponse(503) : okResponse())) as typeof fetch,
    })
    assert.equal(await retrying.compactNow('board_1', JSON.stringify({}), 'usr_1'), true)
    assert.equal(calls, 2)
    assert.equal(retrying.retriedTotal, 1)
    let permanentCalls = 0
    const permanent = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'k',
      baseDelayMs: 1,
      fetchFn: (async () => {
        permanentCalls += 1
        return statusResponse(400)
      }) as typeof fetch,
    })
    assert.equal(await permanent.compactNow('board_1', JSON.stringify({}), 'usr_1'), false)
    assert.equal(permanentCalls, 1)
    retrying.close()
    permanent.close()
  })
})

it('loads a bounded persisted snapshot and ordered opaque tail using the ticket', async () => {
  const persister = new YUpdatePersister({
    apiInternalURL: 'http://go:6000',
    internalKey: 'internal',
    fetchFn: async (input, init) => {
      const url = new URL(String(input))
      assert.equal(url.pathname, '/internal/realtime/document')
      assert.equal(url.searchParams.get('ticket'), 'signed-ticket')
      assert.equal(new Headers(init?.headers).get('x-internal-key'), 'internal')
      return Response.json({
        code: 0,
        data: { snapshot: { schema_version: 1, payload: {} }, updates: ['AQI=', 'Aw=='] },
      })
    },
  })
  try {
    const loaded = await persister.load('signed-ticket')
    assert.deepEqual(loaded.snapshot, { schema_version: 1, payload: {} })
    assert.deepEqual(
      loaded.updates.map((value) => Array.from(value)),
      [[1, 2], [3]],
    )
  } finally {
    persister.close()
  }
})

it('fails recovery closed for oversized or malformed hydration responses', async () => {
  for (const response of [
    new Response('{}', { headers: { 'content-length': String((8 << 20) + 1) } }),
    Response.json({ code: 0, data: { snapshot: {}, updates: [123] } }),
    Response.json({ code: 1, data: { snapshot: {}, updates: [] } }),
    new Response('', { status: 403 }),
  ]) {
    const persister = new YUpdatePersister({
      apiInternalURL: 'http://go:6000',
      internalKey: 'internal',
      fetchFn: async () => response,
    })
    try {
      await assert.rejects(persister.load('ticket'))
    } finally {
      persister.close()
    }
  }
})

it('settles an append retry when the transport adapter is closed', async () => {
  let requested!: () => void
  const request = new Promise<void>((resolve) => {
    requested = resolve
  })
  const persister = new YUpdatePersister({
    apiInternalURL: 'http://go:6000',
    internalKey: 'internal',
    baseDelayMs: 60_000,
    fetchFn: async () => {
      requested()
      return statusResponse(503)
    },
  })
  const pending = persister.append('board', new Uint8Array([1]), 'user')
  await request
  // Let response draining install the retry timer, then close it.
  await new Promise((resolve) => setImmediate(resolve))
  persister.close()
  assert.equal(await pending, false)
  assert.equal(persister.pendingCount, 0)
})
