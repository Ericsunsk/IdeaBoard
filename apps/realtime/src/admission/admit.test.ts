import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { admitToRoom } from './admit.js'

const OPTIONS = {
  apiInternalURL: 'http://127.0.0.1:6000',
  internalKey: 'test-internal-key',
  ticket: 'first-segment.second-segment',
  payload: { bid: 'board-1', av: 7, uid: 'user-1' },
}

function envelope(admit: unknown): string {
  return JSON.stringify({ code: 0, msg: 'success', data: { admit } })
}

function stubFetch(
  status: number,
  body: string,
  seen: { url?: string; key?: string | null },
): typeof fetch {
  return (async (url: string | URL | Request, init?: RequestInit) => {
    seen.url = String(url)
    const headers = new Headers(init?.headers)
    seen.key = headers.get('x-internal-key')
    return new Response(body, { status })
  }) as typeof fetch
}

describe('admitToRoom', () => {
  it('admits on 200 with admit:true and sends ticket plus echo fields', async () => {
    const seen: { url?: string; key?: string | null } = {}
    const result = await admitToRoom({ ...OPTIONS, fetchFn: stubFetch(200, envelope(true), seen) })
    assert.deepEqual(result, { admitted: true, status: 200 })
    const url = new URL(seen.url!)
    assert.equal(url.pathname, '/internal/realtime/admit')
    assert.equal(url.searchParams.get('ticket'), OPTIONS.ticket)
    assert.equal(url.searchParams.get('bid'), 'board-1')
    assert.equal(url.searchParams.get('uid'), 'user-1')
    assert.equal(url.searchParams.get('av'), '7')
    assert.equal(seen.key, 'test-internal-key')
  })

  it('denies on 200 with admit:false without throwing', async () => {
    const result = await admitToRoom({ ...OPTIONS, fetchFn: stubFetch(200, envelope(false), {}) })
    assert.deepEqual(result, { admitted: false, status: 200 })
  })

  it('denies on malformed or non-envelope bodies', async () => {
    for (const body of ['not-json', '{}', JSON.stringify({ code: 0 }), envelope('yes')]) {
      const result = await admitToRoom({ ...OPTIONS, fetchFn: stubFetch(200, body, {}) })
      assert.deepEqual(result, { admitted: false, status: 200 }, body)
    }
  })

  it('denies on non-2xx without throwing', async () => {
    for (const status of [401, 403, 404, 500]) {
      const result = await admitToRoom({
        ...OPTIONS,
        fetchFn: stubFetch(status, envelope(true), {}),
      })
      assert.deepEqual(result, { admitted: false, status })
    }
  })

  it('denies closed on transport failure', async () => {
    const failing = (async () => {
      throw new Error('connection refused')
    }) as typeof fetch
    const result = await admitToRoom({ ...OPTIONS, fetchFn: failing })
    assert.deepEqual(result, { admitted: false, status: 0 })
  })

  it('aborts after the timeout', async () => {
    let observedSignal: AbortSignal | undefined
    const hanging = (async (_url: unknown, init?: RequestInit) => {
      observedSignal = init?.signal as AbortSignal | undefined
      await new Promise((_resolve, reject) => {
        observedSignal?.addEventListener('abort', () => reject(new Error('aborted')))
      })
      return new Response('{}', { status: 200 })
    }) as unknown as typeof fetch
    const result = await admitToRoom({ ...OPTIONS, timeoutMs: 20, fetchFn: hanging })
    assert.deepEqual(result, { admitted: false, status: 0 })
    assert.equal(observedSignal?.aborted, true)
  })
})
