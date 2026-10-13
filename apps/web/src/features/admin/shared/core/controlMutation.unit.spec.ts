import { describe, expect, it } from 'vitest'
import {
  buildControlMutationHeaders,
  mapControlError,
  readControlErrorCode,
} from './controlMutation'

describe('control mutation helpers', () => {
  it('builds stable Control headers with an explicit or resource default key', () => {
    expect(
      buildControlMutationHeaders({ idempotencyKey: 'retry-key' }, 'resource-key'),
    ).toMatchObject({
      'Idempotency-Key': 'retry-key',
    })
    expect(buildControlMutationHeaders({}, 'resource-key')).toMatchObject({
      'Idempotency-Key': 'resource-key',
    })
  })

  it('reads machine codes from contract and HTTP-like errors', () => {
    expect(readControlErrorCode({ code: 'control_not_found' })).toBe('control_not_found')
    expect(readControlErrorCode({ status: 409 })).toBe(409)
    expect(readControlErrorCode(new Error('transport'))).toBeUndefined()
  })

  it('generates distinct request and idempotency keys for new mutations', () => {
    const first = buildControlMutationHeaders()
    const second = buildControlMutationHeaders()
    expect(first).toEqual({
      'Request-Id': expect.any(String),
      'Idempotency-Key': expect.any(String),
    })
    expect(first['Request-Id']).toBeTruthy()
    expect(first['Request-Id']).not.toBe(second['Request-Id'])
    expect(first['Idempotency-Key']).toBeTruthy()
    expect(first['Idempotency-Key']).not.toBe(second['Idempotency-Key'])
  })

  it('maps domain-owned copy while preserving the request id', () => {
    const mapped = mapControlError({ code: 'control_not_found', request_id: 'req-1' }, 'fallback', {
      control_not_found: '用户不存在',
    })
    expect(mapped).toMatchObject({
      code: 'control_not_found',
      message: '用户不存在',
      requestId: 'req-1',
    })
  })
})
