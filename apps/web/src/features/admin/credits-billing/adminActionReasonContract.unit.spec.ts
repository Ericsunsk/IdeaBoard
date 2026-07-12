import { describe, expect, it } from 'vitest'
import {
  MAX_ADMIN_ACTION_REASON_LENGTH,
  validateAdminActionReason,
} from './adminActionReasonContract'

describe('admin action reason contract', () => {
  it('rejects blank and oversized reasons', () => {
    expect(validateAdminActionReason('   ')).toMatchObject({
      error: '请输入操作原因',
      valid: false,
    })
    expect(validateAdminActionReason('x'.repeat(MAX_ADMIN_ACTION_REASON_LENGTH + 1))).toMatchObject({
      error: `操作原因不能超过 ${MAX_ADMIN_ACTION_REASON_LENGTH} 个字符`,
      valid: false,
    })
  })

  it('normalizes a valid reason before it crosses the API boundary', () => {
    expect(validateAdminActionReason('  调整生图计价  ')).toEqual({
      normalizedReason: '调整生图计价',
      error: '',
      valid: true,
    })
  })

  it('counts Unicode code points consistently with the backend RuneCount contract', () => {
    expect(validateAdminActionReason('🧭'.repeat(MAX_ADMIN_ACTION_REASON_LENGTH))).toMatchObject({
      error: '',
      valid: true,
    })
    expect(
      validateAdminActionReason('🧭'.repeat(MAX_ADMIN_ACTION_REASON_LENGTH + 1)),
    ).toMatchObject({
      error: expect.any(String),
      valid: false,
    })
  })
})
