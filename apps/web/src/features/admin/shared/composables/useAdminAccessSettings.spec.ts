import { describe, expect, it } from 'vitest'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  validateRateLimitSettings,
} from './useAdminAccessSettings'

describe('admin access settings validation', () => {
  it('accepts valid integer rate limit settings', () => {
    expect(validateRateLimitSettings({ max_requests: 100, window_seconds: 60 })).toBe('')
  })

  it('rejects empty or non-positive request limits', () => {
    expect(validateRateLimitSettings({ max_requests: null, window_seconds: 60 })).toBe(
      '最大请求需为大于 0 的整数',
    )
    expect(validateRateLimitSettings({ max_requests: 0, window_seconds: 60 })).toBe(
      '最大请求需为大于 0 的整数',
    )
  })

  it('rejects values above configured caps', () => {
    expect(
      validateRateLimitSettings({
        max_requests: RATE_LIMIT_MAX_REQUESTS + 1,
        window_seconds: 60,
      }),
    ).toBe(`最大请求不能超过 ${RATE_LIMIT_MAX_REQUESTS}`)
    expect(
      validateRateLimitSettings({
        max_requests: 100,
        window_seconds: RATE_LIMIT_MAX_WINDOW_SECONDS + 1,
      }),
    ).toBe(`窗口周期不能超过 ${RATE_LIMIT_MAX_WINDOW_SECONDS} 秒`)
  })
})
