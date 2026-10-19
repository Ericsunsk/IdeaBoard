import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createHmac } from 'node:crypto'
import { signTicket, verifyTicket, type TicketPayload } from './ticket.js'

const SECRET = 'test-ticket-secret-0123456789abcdef'
const OTHER_SECRET = 'other-ticket-secret-0123456789abcdef'

function payload(overrides: Partial<TicketPayload> = {}): TicketPayload {
  return {
    bid: 'board-1',
    av: 7,
    cap: 'read',
    exp: Math.floor(Date.now() / 1000) + 60,
    uid: 'user-1',
    ...overrides,
  }
}

describe('verifyTicket', () => {
  it('accepts a freshly signed ticket', () => {
    const ticket = signTicket(payload(), SECRET)
    const result = verifyTicket(ticket, SECRET)
    assert.equal(result.ok, true)
    if (result.ok) {
      assert.equal(result.payload.bid, 'board-1')
      assert.equal(result.payload.uid, 'user-1')
    }
  })

  it('rejects a tampered payload', () => {
    const ticket = signTicket(payload(), SECRET)
    const second = ticket.split('.')[1]
    const tampered = `${Buffer.from(JSON.stringify({ ...payload(), bid: 'board-2' })).toString('base64url')}.${second}`
    const result = verifyTicket(tampered, SECRET)
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.reason, 'invalid ticket signature')
  })

  it('rejects a signature made with another secret', () => {
    const ticket = signTicket(payload(), OTHER_SECRET)
    const result = verifyTicket(ticket, SECRET)
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.reason, 'invalid ticket signature')
  })

  it('rejects an expired ticket', () => {
    const ticket = signTicket(payload({ exp: Math.floor(Date.now() / 1000) - 1 }), SECRET)
    const result = verifyTicket(ticket, SECRET)
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.reason, 'ticket expired')
  })

  it('rejects malformed tickets', () => {
    for (const bad of ['', 'abc', 'a.b.c', '.sig', 'payload.', '!!!.???']) {
      const result = verifyTicket(bad, SECRET)
      assert.equal(result.ok, false, bad)
    }
  })

  it('rejects well-signed but malformed payloads', () => {
    const sign = (text: string): string => {
      const first = Buffer.from(text, 'utf8').toString('base64url')
      const hex = createHmac('sha256', SECRET).update(first, 'utf8').digest('hex')
      return `${first}.${Buffer.from(hex, 'utf8').toString('base64url')}`
    }
    const base = payload()
    const cases: Array<[string, string]> = [
      ['not-json', 'not-json'],
      ['missing fields', JSON.stringify({ bid: 'x' })],
      // Unknown cap: Go Verify only accepts read|write, so local shape matches.
      ['unknown cap', JSON.stringify({ ...base, cap: 'edit' })],
      ['zero av', JSON.stringify({ ...base, av: 0 })],
      ['blank bid', JSON.stringify({ ...base, bid: '   ' })],
    ]
    for (const [name, text] of cases) {
      const result = verifyTicket(sign(text), SECRET)
      assert.equal(result.ok, false, name)
      if (!result.ok) assert.equal(result.reason, 'malformed ticket payload')
    }
  })

  it('fails closed without a secret', () => {
    const ticket = signTicket(payload(), SECRET)
    const result = verifyTicket(ticket, '')
    assert.equal(result.ok, false)
  })

  it('round-trips the exact frozen wire shape', () => {
    const ticket = signTicket(payload(), SECRET)
    const [first, second] = ticket.split('.')
    assert.ok(first && second)
    const payloadText = Buffer.from(first!, 'base64url').toString('utf8')
    const signatureText = Buffer.from(second!, 'base64url').toString('utf8')
    assert.deepEqual(JSON.parse(payloadText), payload())
    assert.match(signatureText, /^[0-9a-f]{64}$/)
  })
})
