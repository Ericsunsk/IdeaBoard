import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { FastifyReply } from 'fastify'
import { getErrorStatus, getHeaderValue, sendError, sendSuccess } from './httpResponse.js'

describe('getHeaderValue', () => {
  it('reads single, missing, and multi-value headers', () => {
    assert.equal(getHeaderValue('secret-key'), 'secret-key')
    assert.equal(getHeaderValue(undefined), '')
    assert.equal(getHeaderValue(['first', 'second']), 'first')
    assert.equal(getHeaderValue([]), '')
  })
})

describe('getErrorStatus', () => {
  it('passes through valid 4xx and 5xx statuses', () => {
    assert.equal(getErrorStatus({ statusCode: 404 }), 404)
    assert.equal(getErrorStatus({ status: 503 }), 503)
  })

  it('falls back to 500 for missing or out-of-range statuses', () => {
    assert.equal(getErrorStatus(new Error('boom')), 500)
    assert.equal(getErrorStatus({ statusCode: 200 }), 500)
    assert.equal(getErrorStatus({ statusCode: 'bad' }), 500)
  })
})

describe('auth HTTP envelope', () => {
  it('renders success and error payloads', () => {
    const successReply = createReply()
    sendSuccess(successReply as unknown as FastifyReply, { status: 'ok' })
    assert.equal(successReply.statusCode, 200)
    assert.deepEqual(successReply.payload, {
      code: 0,
      msg: 'success',
      data: { status: 'ok' },
    })

    const errorReply = createReply()
    sendError(errorReply as unknown as FastifyReply, 401, 'unauthorized')
    assert.equal(errorReply.statusCode, 401)
    assert.deepEqual(errorReply.payload, { code: 401, msg: 'unauthorized' })
  })
})

function createReply() {
  return {
    statusCode: undefined as number | undefined,
    payload: undefined as unknown,
    status(code: number) {
      this.statusCode = code
      return this
    },
    send(payload: unknown) {
      this.payload = payload
      return this
    },
  }
}
