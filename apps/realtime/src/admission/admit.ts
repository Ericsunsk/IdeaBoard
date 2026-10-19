// Go admit call: GET /internal/realtime/admit with X-Internal-Key.
// Mirrors apps/auth loginMetadataReporter: single-shot fetch, 2s timeout,
// fail-closed on any transport, non-2xx, or body outcome.
//
// Go contract (apps/api/internal/runtime/interfaces/http/internal_realtime.go):
// primary query is ?ticket=<full ticket>; bid/av/uid may echo the verified
// payload claims (they must match). Go answers 200 + {code:0, data:{admit}}
// in every verified case, so this client branches on the admit boolean, not
// on the HTTP status. fetchFn is injectable for tests.

import type { TicketPayload } from './ticket.js'

export type AdmitOptions = {
  apiInternalURL: string
  internalKey: string
  ticket: string
  payload: Pick<TicketPayload, 'bid' | 'av' | 'uid'>
  timeoutMs?: number
  fetchFn?: typeof fetch
}

export type AdmitResult = {
  admitted: boolean
  status: number
}

const ADMIT_TIMEOUT_MS = 2_000
const MAX_ADMIT_BODY_BYTES = 64 * 1024

export async function admitToRoom(options: AdmitOptions): Promise<AdmitResult> {
  const timeoutMs = options.timeoutMs ?? ADMIT_TIMEOUT_MS
  const fetchFn = options.fetchFn ?? fetch
  let endpoint: URL
  try {
    endpoint = new URL('/internal/realtime/admit', `${options.apiInternalURL.replace(/\/$/, '')}/`)
  } catch {
    return { admitted: false, status: 0 }
  }
  endpoint.searchParams.set('ticket', options.ticket)
  endpoint.searchParams.set('bid', options.payload.bid)
  endpoint.searchParams.set('uid', options.payload.uid)
  endpoint.searchParams.set('av', String(options.payload.av))

  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), timeoutMs)
  try {
    const response = await fetchFn(endpoint.toString(), {
      method: 'GET',
      headers: { 'x-internal-key': options.internalKey },
      signal: controller.signal,
    })
    if (!response.ok) return { admitted: false, status: response.status }
    const body = await readBoundedText(response, MAX_ADMIT_BODY_BYTES)
    return { admitted: parseAdmitBody(body), status: response.status }
  } catch {
    return { admitted: false, status: 0 }
  } finally {
    clearTimeout(timeout)
  }
}

async function readBoundedText(response: Response, maxBytes: number): Promise<string> {
  // No streaming reader: the admit body is a tiny envelope; cap it by
  // content-length when present, then re-check the decoded length.
  const declared = response.headers.get('content-length')
  if (declared !== null && Number.parseInt(declared, 10) > maxBytes) {
    throw new Error('admit response too large')
  }
  const text = await response.text()
  if (text.length > maxBytes) throw new Error('admit response too large')
  return text
}

function parseAdmitBody(body: string): boolean {
  try {
    const envelope = JSON.parse(body) as {
      code?: unknown
      data?: { admit?: unknown } | null
    }
    return (
      envelope?.code === 0 &&
      (envelope?.data ?? null) !== null &&
      (envelope.data as { admit?: unknown }).admit === true
    )
  } catch {
    return false
  }
}
