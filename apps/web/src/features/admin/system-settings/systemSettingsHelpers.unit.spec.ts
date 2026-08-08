import { describe, expect, it } from 'vitest'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  validateRateLimitSettings,
} from './useAdminAccessSettings'
import {
  DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  normalizeMediaUploadSetting,
  validateMediaUploadSetting,
} from './useAdminMediaUploadSettings'

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

describe('admin media upload settings helpers', () => {
  it('normalizes missing values to defaults', () => {
    expect(normalizeMediaUploadSetting(null)).toEqual(DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING)
  })

  it('normalizes numeric values to integers', () => {
    expect(normalizeMediaUploadSetting({ upload_max_concurrent: 12.8 })).toEqual({
      upload_max_concurrent: 12,
    })
  })

  it('validates concurrent upload range', () => {
    expect(validateMediaUploadSetting({ upload_max_concurrent: MEDIA_UPLOAD_MIN_CONCURRENT })).toBe(
      '',
    )
    expect(validateMediaUploadSetting({ upload_max_concurrent: MEDIA_UPLOAD_MAX_CONCURRENT })).toBe(
      '',
    )
    expect(validateMediaUploadSetting({ upload_max_concurrent: 0 })).toBe(
      `上传并发上限必须在 ${MEDIA_UPLOAD_MIN_CONCURRENT} 到 ${MEDIA_UPLOAD_MAX_CONCURRENT} 之间`,
    )
    expect(
      validateMediaUploadSetting({ upload_max_concurrent: MEDIA_UPLOAD_MAX_CONCURRENT + 1 }),
    ).toBe(
      `上传并发上限必须在 ${MEDIA_UPLOAD_MIN_CONCURRENT} 到 ${MEDIA_UPLOAD_MAX_CONCURRENT} 之间`,
    )
  })
})
