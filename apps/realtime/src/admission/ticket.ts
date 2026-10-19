// Ticket verification for /realtime?ticket=. Pure decisions, no I/O.
// Wire msg values stay English lowercase tokens (machine surface, same rule
// as apps/auth trusted-identity).
//
// Ticket shape (frozen): base64url(payloadJSON).base64url(hexHmacSha256)
// where hexHmacSha256 = hex(HMAC-SHA256(secret, firstSegmentAscii)).
// Payload: { bid, av, cap, exp, uid }, exp is unix seconds.

import { createHmac, timingSafeEqual } from 'node:crypto'

export type TicketPayload = {
  bid: string
  av: number
  cap: string
  exp: number
  uid: string
}

export type TicketVerifyResult =
  { ok: true; payload: TicketPayload } | { ok: false; reason: string }

const MAX_FIELD_LENGTH = 256

// Keep local validation aligned with the Go minter
// (apps/api/internal/boards/realtime/ticket.go): av is a positive
// access_version, cap is read|write, and blank-padded claims fail closed on
// both sides so a locally accepted ticket never skews from Go Verify.
const TICKET_CAPABILITIES = new Set(['read', 'write'])

function isTicketPayload(value: unknown): value is TicketPayload {
  if (typeof value !== 'object' || value === null) return false
  const record = value as Record<string, unknown>
  return (
    typeof record.bid === 'string' &&
    record.bid.trim().length > 0 &&
    record.bid.length <= MAX_FIELD_LENGTH &&
    typeof record.av === 'number' &&
    Number.isSafeInteger(record.av) &&
    record.av >= 1 &&
    typeof record.cap === 'string' &&
    TICKET_CAPABILITIES.has(record.cap) &&
    typeof record.exp === 'number' &&
    Number.isSafeInteger(record.exp) &&
    record.exp > 0 &&
    typeof record.uid === 'string' &&
    record.uid.trim().length > 0 &&
    record.uid.length <= MAX_FIELD_LENGTH
  )
}

function decodeSegment(segment: string): string | null {
  if (!segment) return null
  try {
    return Buffer.from(segment, 'base64url').toString('utf8')
  } catch {
    return null
  }
}

/** Mint a ticket. Test helper and documentation of the frozen wire shape. */
export function signTicket(payload: TicketPayload, secret: string): string {
  if (!secret) throw new Error('ticket secret is required')
  if (!isTicketPayload(payload)) throw new Error('invalid ticket payload')
  const first = Buffer.from(
    JSON.stringify({
      bid: payload.bid,
      av: payload.av,
      cap: payload.cap,
      exp: payload.exp,
      uid: payload.uid,
    }),
    'utf8',
  ).toString('base64url')
  const hex = createHmac('sha256', secret).update(first, 'utf8').digest('hex')
  const second = Buffer.from(hex, 'utf8').toString('base64url')
  return `${first}.${second}`
}

export function verifyTicket(
  ticket: string,
  secret: string,
  nowMs: number = Date.now(),
): TicketVerifyResult {
  if (!secret) return { ok: false, reason: 'ticket secret is not configured' }
  const raw = (ticket ?? '').trim()
  if (!raw) return { ok: false, reason: 'ticket is required' }
  const parts = raw.split('.')
  if (parts.length !== 2 || !parts[0] || !parts[1]) {
    return { ok: false, reason: 'malformed ticket' }
  }
  const [first, second] = parts as [string, string]

  const payloadText = decodeSegment(first)
  const signatureText = decodeSegment(second)
  if (payloadText === null || signatureText === null) {
    return { ok: false, reason: 'malformed ticket' }
  }
  let payload: unknown
  try {
    payload = JSON.parse(payloadText)
  } catch {
    return { ok: false, reason: 'malformed ticket payload' }
  }
  if (!isTicketPayload(payload)) {
    return { ok: false, reason: 'malformed ticket payload' }
  }

  const expectedHex = createHmac('sha256', secret).update(first, 'utf8').digest('hex')
  const provided = Buffer.from(signatureText)
  const expected = Buffer.from(expectedHex)
  if (
    provided.length === 0 ||
    provided.length !== expected.length ||
    !timingSafeEqual(provided, expected)
  ) {
    return { ok: false, reason: 'invalid ticket signature' }
  }

  if (payload.exp * 1000 <= nowMs) {
    return { ok: false, reason: 'ticket expired' }
  }
  return { ok: true, payload }
}
