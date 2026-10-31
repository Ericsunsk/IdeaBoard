import { describe, expect, it } from 'vitest'
import { buildControlMutationHeaders } from './controlMutation'
import { withControlCopy } from '../core/controlCopy'

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

  it('handles contract and HTTP-like errors while preserving transport errors', () => {
    expect(
      withControlCopy({ code: 'control_not_found' }, 'fallback', {
        control_not_found: '用户不存在',
      }),
    ).toMatchObject({ code: 'control_not_found', message: '用户不存在' })
    expect(withControlCopy({ status: 409 }, 'fallback')).toMatchObject({
      code: 'idempotency_conflict',
      message: '操作已提交，请勿重复操作',
    })
    const transportError = new Error('transport')
    expect(withControlCopy(transportError, 'fallback')).toBe(transportError)
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
    const mapped = withControlCopy({ code: 'control_not_found', request_id: 'req-1' }, 'fallback', {
      control_not_found: '用户不存在',
    })
    expect(mapped).toMatchObject({
      code: 'control_not_found',
      message: '用户不存在',
      requestId: 'req-1',
    })
  })
})
