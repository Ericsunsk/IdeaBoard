import assert from 'node:assert/strict'
import { afterEach, describe, it } from 'node:test'
import { createLoginMetadataReporter, type LoginMetadata } from './loginMetadataReporter.js'

const originalFetch = globalThis.fetch

afterEach(() => {
  globalThis.fetch = originalFetch
})

function metadata(overrides: Partial<LoginMetadata> = {}): LoginMetadata {
  return {
    authUserId: 'auth-user-1',
    ip: '203.0.113.42',
    country: 'US',
    ...overrides,
  }
}

function reporter() {
  return createLoginMetadataReporter({
    auth: {
      apiInternalURL: 'http://127.0.0.1:6000/',
      internalKey: 'internal-key',
    },
  } as never)
}

describe('login metadata reporter', () => {
  it('posts normalized session metadata to the private API endpoint', async () => {
    let request: Request | undefined
    globalThis.fetch = async (input, init) => {
      request = new Request(input, init)
      return new Response(null, { status: 204 })
    }

    await reporter().report(metadata())

    assert.equal(request?.url, 'http://127.0.0.1:6000/internal/users/login-metadata')
    assert.equal(request?.method, 'POST')
    assert.equal(request?.headers.get('x-internal-key'), 'internal-key')
    assert.deepEqual(await request?.json(), {
      auth_user_id: 'auth-user-1',
      ip: '203.0.113.42',
      country: 'US',
    })
  })

  it('does not throw when the API rejects or the network fails', async () => {
    globalThis.fetch = async () => new Response(null, { status: 503 })
    await assert.doesNotReject(() => reporter().report(metadata()))

    globalThis.fetch = async () => {
      throw new Error('network down')
    }
    await assert.doesNotReject(() => reporter().report(metadata()))
  })

  it('skips incomplete metadata without making a request', async () => {
    let calls = 0
    globalThis.fetch = async () => {
      calls += 1
      return new Response(null, { status: 204 })
    }

    await reporter().report(metadata({ authUserId: '', ip: '' }))

    assert.equal(calls, 0)
  })
})
