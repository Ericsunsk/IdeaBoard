// Issue #800: Public/Control dual-client contract kernel skeleton.
//
// New-envelope (`{ data, meta }` / `{ error: { code, message, details, request_id } }`)
// parsing plus Request-Id / RateLimit / ETag / Last-Event-ID passthrough helpers.
// These sit ALONGSIDE the legacy `apiResponse.ts` boundary: no feature migrates
// to the new envelope yet (migration is #801-#808), and existing logic is untouched.
//
// Per-feature migration TODOs (#801-#808):
// - boardSync: move the board sync adapter to the public client + requireContractData.
// - task + idempotency: route submit/cancel through buildContractRequestHeaders.
// - library/search: add the snake_case -> camelCase boundary on top of parseContractEnvelope.
// - media/upload: resolve delivery URLs through the public client envelope.
// - SSE: resume via withSseResumeParam / buildSseResumeHeaders (Last-Event-ID).

export type ContractMeta = {
  request_id?: string | undefined
  page?: number | undefined
  page_size?: number | undefined
  total?: number | undefined
  [key: string]: unknown
}

export type ContractErrorBody = {
  code: string
  message: string
  details?: unknown
  request_id?: string | undefined
}

export type ContractSuccessEnvelope<T> = {
  data: T
  meta?: ContractMeta | undefined
}

export type ContractErrorEnvelope = {
  error: ContractErrorBody
}

export type ContractParseOk<T> = {
  ok: true
  data: T
  meta: ContractMeta | undefined
}

export type ContractParseErr = {
  ok: false
  error: ContractErrorBody
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 ? value : undefined
}

export function parseContractEnvelope<T>(payload: unknown): ContractParseOk<T> | ContractParseErr {
  if (!isRecord(payload)) {
    return {
      ok: false,
      error: { code: 'malformed_envelope', message: 'Malformed API envelope' },
    }
  }
  const errorBody = payload.error
  if (
    isRecord(errorBody) &&
    typeof errorBody.code === 'string' &&
    typeof errorBody.message === 'string'
  ) {
    return {
      ok: false,
      error: {
        code: errorBody.code,
        message: errorBody.message,
        details: errorBody.details,
        request_id: asOptionalString(errorBody.request_id),
      },
    }
  }
  if ('data' in payload) {
    const meta = isRecord(payload.meta) ? (payload.meta as ContractMeta) : undefined
    return { ok: true, data: payload.data as T, meta }
  }
  return {
    ok: false,
    error: { code: 'malformed_envelope', message: 'Malformed API envelope' },
  }
}

export type ContractKernelError = Error & {
  code?: string | undefined
  requestId?: string | undefined
  details?: unknown
}

export function requireContractData<T>(payload: unknown, fallback: string): T {
  const parsed = parseContractEnvelope<T>(payload)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || fallback) as ContractKernelError
    error.code = parsed.error.code
    error.requestId = parsed.error.request_id
    error.details = parsed.error.details
    throw error
  }
  if (parsed.data == null) throw new Error(fallback)
  return parsed.data
}

// Opaque public IDs: usr_|org_|ws_|board_|task_|asset_|upload_|evt_ + payload.
const OPAQUE_ID_RE = /^(?:usr|org|ws|board|task|asset|upload|evt)_[A-Za-z0-9]+$/

export type OpaqueIdPrefix = 'usr' | 'org' | 'ws' | 'board' | 'task' | 'asset' | 'upload' | 'evt'

export function isOpaqueId(value: unknown): boolean {
  return typeof value === 'string' && OPAQUE_ID_RE.test(value)
}

export function opaqueIdPrefix(value: string): OpaqueIdPrefix | null {
  if (!isOpaqueId(value)) return null
  return value.slice(0, value.indexOf('_')) as OpaqueIdPrefix
}

export type ReadableHeaders =
  Headers | Record<string, string | string[] | undefined> | undefined | null

function readHeader(headers: ReadableHeaders, name: string): string | null {
  if (!headers) return null
  if (typeof (headers as Headers).get === 'function') {
    return (headers as Headers).get(name)
  }
  const record = headers as Record<string, string | string[] | undefined>
  for (const key of Object.keys(record)) {
    if (key.toLowerCase() === name.toLowerCase()) {
      const value = record[key]
      if (Array.isArray(value)) return value.length > 0 ? String(value[0]) : null
      return value == null || value === '' ? null : String(value)
    }
  }
  return null
}

function readFirstHeader(headers: ReadableHeaders, names: readonly string[]): string | null {
  for (const name of names) {
    const value = readHeader(headers, name)
    if (value != null) return value
  }
  return null
}

function readHeaderInt(headers: ReadableHeaders, names: readonly string[]): number | null {
  const raw = readFirstHeader(headers, names)
  if (raw == null || raw.trim() === '') return null
  const parsed = Number(raw.trim())
  return Number.isFinite(parsed) ? parsed : null
}

// Request-Id passthrough: echoed by the gateway on every response.
export function getRequestId(headers: ReadableHeaders): string | null {
  return readFirstHeader(headers, ['request-id', 'x-request-id'])
}

export function newRequestId(): string {
  const randomUUID = (globalThis.crypto as Crypto | undefined)?.randomUUID
  if (typeof randomUUID === 'function') return randomUUID.call(globalThis.crypto)
  return `req_${Date.now().toString(36)}_${Math.floor(Math.random() * 0xffffffff).toString(36)}`
}

export function buildContractRequestHeaders(options?: {
  requestId?: string | undefined
  idempotencyKey?: string | undefined
}): Record<string, string> {
  const headers: Record<string, string> = {}
  if (options?.requestId) headers['Request-Id'] = options.requestId
  if (options?.idempotencyKey) headers['Idempotency-Key'] = options.idempotencyKey
  return headers
}

export type RateLimitState = {
  limit: number | null
  remaining: number | null
  reset: number | null
  retryAfterSeconds: number | null
}

// RateLimit passthrough: prefers RFC draft `RateLimit-*`, falls back to
// `X-RateLimit-*`; Retry-After covers 429/503 responses (seconds only,
// HTTP-date form is out of scope for the skeleton).
export function parseRateLimit(headers: ReadableHeaders): RateLimitState {
  return {
    limit: readHeaderInt(headers, ['ratelimit-limit', 'x-ratelimit-limit']),
    remaining: readHeaderInt(headers, ['ratelimit-remaining', 'x-ratelimit-remaining']),
    reset: readHeaderInt(headers, ['ratelimit-reset', 'x-ratelimit-reset']),
    retryAfterSeconds: readHeaderInt(headers, ['retry-after']),
  }
}

// ETag passthrough for conditional refetch of cacheable Control reads.
export function getETag(headers: ReadableHeaders): string | null {
  return readFirstHeader(headers, ['etag'])
}

export function buildConditionalHeaders(etag?: string | null): Record<string, string> {
  const trimmed = typeof etag === 'string' ? etag.trim() : ''
  return trimmed ? { 'If-None-Match': trimmed } : {}
}

// SSE resume: native EventSource cannot set request headers, so fetch-based
// transports resume with `?last_event_id=` while header transports use
// `Last-Event-ID`. (Migration: userEventStream SSE Last-Event-ID, #801-#808.)
export function buildSseResumeHeaders(lastEventId?: string | null): Record<string, string> {
  const trimmed = typeof lastEventId === 'string' ? lastEventId.trim() : ''
  return trimmed ? { 'Last-Event-ID': trimmed } : {}
}

export function withSseResumeParam(url: string, lastEventId?: string | null): string {
  const trimmed = typeof lastEventId === 'string' ? lastEventId.trim() : ''
  if (!trimmed) return url
  const separator = url.includes('?') ? '&' : '?'
  return `${url}${separator}last_event_id=${encodeURIComponent(trimmed)}`
}
