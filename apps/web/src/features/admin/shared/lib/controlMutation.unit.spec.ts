import { describe, expect, it } from 'vitest'
import {
  buildControlMutationHeaders,
  mapControlError,
  readControlErrorCode,
  requireOperatorReason,
} from './controlMutation'

describe('control mutation helpers', () => {
  it('trims operator reasons and rejects blank values with a machine code', () => {
    expect(requireOperatorReason('  reviewed by support  ')).toBe('reviewed by support')
    expect(() => requireOperatorReason('  ')).toThrowError(
      expect.objectContaining({ code: 'control_reason_required' }),
    )
  })

  it('builds stable Control headers with an explicit or resource default key', () => {
    expect(
      buildControlMutationHeaders(
        { operatorReason: ' reviewed ', idempotencyKey: 'retry-key' },
        'resource-key',
      ),
    ).toMatchObject({
      'Idempotency-Key': 'retry-key',
      'X-Operator-Reason': 'reviewed',
    })
    expect(
      buildControlMutationHeaders({ operatorReason: 'reviewed' }, 'resource-key'),
    ).toMatchObject({
      'Idempotency-Key': 'resource-key',
      'X-Operator-Reason': 'reviewed',
    })
  })

  it('reads machine codes from contract and HTTP-like errors', () => {
    expect(readControlErrorCode({ code: 'control_not_found' })).toBe('control_not_found')
    expect(readControlErrorCode({ status: 409 })).toBe(409)
    expect(readControlErrorCode(new Error('transport'))).toBeUndefined()
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
