// Issue #800: frontend contract test stub for the Public/Control kernel.
// No generated-query imports or mocks here by design (see checkTestGuardrails:
// contract specs must not mock generated-query modules).
import { describe, expect, it } from 'vitest'
import {
  buildConditionalHeaders,
  buildContractRequestHeaders,
  buildSseResumeHeaders,
  getETag,
  getRequestId,
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
  parseContractEnvelope,
  parseRateLimit,
  requireContractData,
  withSseResumeParam,
} from './contractKernel'

describe('contract kernel envelope', () => {
  it('parses success fixtures with data and optional meta', () => {
    expect(parseContractEnvelope({ data: { id: 1 } })).toEqual({
      ok: true,
      data: { id: 1 },
      meta: undefined,
    })
    expect(parseContractEnvelope({ data: [], meta: { request_id: 'req_1', total: 0 } })).toEqual({
      ok: true,
      data: [],
      meta: { request_id: 'req_1', total: 0 },
    })
  })

  it('parses error fixtures with code, message, details, and request_id', () => {
    expect(
      parseContractEnvelope({
        error: {
          code: 'board_not_found',
          message: 'Board not found',
          details: { id: 7 },
          request_id: 'req_9',
        },
      }),
    ).toEqual({
      ok: false,
      error: {
        code: 'board_not_found',
        message: 'Board not found',
        details: { id: 7 },
        request_id: 'req_9',
      },
    })
  })

  it('rejects malformed envelopes without throwing', () => {
    for (const payload of [
      null,
      42,
      'oops',
      [],
      {},
      { error: { code: 1 } },
      { data: undefined, meta: 3 },
    ]) {
      const parsed = parseContractEnvelope(payload)
      if (payload !== null && typeof payload === 'object' && 'data' in (payload as object)) {
        expect(parsed.ok).toBe(true)
      } else {
        expect(parsed).toEqual({
          ok: false,
          error: { code: 'malformed_envelope', message: 'Malformed API envelope' },
        })
      }
    }
  })

  it('requires data with error metadata preserved', () => {
    expect(requireContractData({ data: 'ok' }, 'fallback')).toBe('ok')
    expect(() => requireContractData({ data: null }, 'missing data')).toThrow('missing data')
    try {
      requireContractData(
        { error: { code: 'gone', message: 'Gone', request_id: 'req_1' } },
        'fallback',
      )
      expect.unreachable()
    } catch (error) {
      expect(error).toMatchObject({ code: 'gone', requestId: 'req_1', message: 'Gone' })
    }
  })
})

describe('contract kernel headers', () => {
  it('passes Request-Id through from response headers', () => {
    expect(getRequestId(new Headers({ 'Request-Id': 'req_abc' }))).toBe('req_abc')
    expect(getRequestId(new Headers({ 'X-Request-Id': 'req_xyz' }))).toBe('req_xyz')
    expect(getRequestId({ 'REQUEST-ID': 'req_mixed' })).toBe('req_mixed')
    expect(getRequestId(undefined)).toBeNull()
  })

  it('builds request headers with Request-Id and Idempotency-Key', () => {
    expect(buildContractRequestHeaders()).toEqual({})
    expect(
      buildContractRequestHeaders({ requestId: 'req_1', idempotencyKey: 'task-cancel:t_1' }),
    ).toEqual({ 'Request-Id': 'req_1', 'Idempotency-Key': 'task-cancel:t_1' })
    expect(newRequestId()).toMatch(/^.{8,}$/)
  })

  it('passes RateLimit state through from response headers', () => {
    expect(
      parseRateLimit(
        new Headers({
          'RateLimit-Limit': '100',
          'RateLimit-Remaining': '42',
          'RateLimit-Reset': '30',
          'Retry-After': '5',
        }),
      ),
    ).toEqual({ limit: 100, remaining: 42, reset: 30, retryAfterSeconds: 5 })
    expect(
      parseRateLimit({ 'X-RateLimit-Limit': '10', 'X-RateLimit-Remaining': '0' }),
    ).toMatchObject({ limit: 10, remaining: 0, reset: null, retryAfterSeconds: null })
    expect(parseRateLimit(undefined)).toEqual({
      limit: null,
      remaining: null,
      reset: null,
      retryAfterSeconds: null,
    })
  })

  it('passes ETag through for conditional refetch', () => {
    expect(getETag(new Headers({ ETag: 'W/"abc"' }))).toBe('W/"abc"')
    expect(getETag({})).toBeNull()
    expect(buildConditionalHeaders('W/"abc"')).toEqual({ 'If-None-Match': 'W/"abc"' })
    expect(buildConditionalHeaders(null)).toEqual({})
    expect(buildConditionalHeaders('   ')).toEqual({})
  })

  it('resumes SSE streams via Last-Event-ID header or URL param', () => {
    expect(buildSseResumeHeaders('evt_42')).toEqual({ 'Last-Event-ID': 'evt_42' })
    expect(buildSseResumeHeaders(undefined)).toEqual({})
    expect(withSseResumeParam('/api/user/events', 'evt_42')).toBe(
      '/api/user/events?last_event_id=evt_42',
    )
    expect(withSseResumeParam('/api/user/events?a=b', 'evt 42')).toBe(
      '/api/user/events?a=b&last_event_id=evt%2042',
    )
    expect(withSseResumeParam('/api/user/events', '')).toBe('/api/user/events')
  })
})

describe('contract kernel opaque IDs', () => {
  it('validates every documented prefix', () => {
    for (const prefix of [
      'usr',
      'org',
      'ws',
      'board',
      'task',
      'asset',
      'upload',
      'evt',
      'ba',
      'bti',
    ]) {
      expect(isOpaqueId(`${prefix}_abc123`)).toBe(true)
      expect(opaqueIdPrefix(`${prefix}_abc123`)).toBe(prefix)
    }
  })

  it('accepts the backend dash/underscore payload alphabet', () => {
    for (const id of ['ws_a-b_c', 'task_x-y_z', 'upload_up-01_x']) {
      expect(isOpaqueId(id)).toBe(true)
      expect(opaqueIdPrefix(id)).toBe(id.slice(0, id.indexOf('_')))
    }
  })

  it('rejects legacy numeric IDs and malformed values', () => {
    for (const value of [
      '123',
      'task-',
      'task_',
      'unknown_x1',
      '',
      42,
      null,
      undefined,
      'TASK_a1',
    ]) {
      expect(isOpaqueId(value)).toBe(false)
    }
    expect(opaqueIdPrefix('123')).toBeNull()
  })
})
