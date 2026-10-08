import { describe, expect, it } from 'vitest'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  validateRateLimitSettings,
} from './useAccessSettings'
import {
  DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  normalizeMediaUploadSetting,
  validateMediaUploadSetting,
} from './useMediaUploadSettings'

/**
 * Pure-function mirror of the MSW-level lifecycle coverage in
 * `useSettingsLifecycle.unit.spec.ts` (which owns the load/dirty/save/reset and
 * failure-keeps-editable contracts). This file only pins the validation truth
 * table in one compact matrix.
 */
describe('admin access settings validation', () => {
  it.each([
    {
      name: 'accepts valid integer rate limit settings',
      input: { max_requests: 100, window_seconds: 60 },
      expected: '',
    },
    {
      name: 'rejects null request limits',
      input: { max_requests: null, window_seconds: 60 },
      expected: '最大请求需为大于 0 的整数',
    },
    {
      name: 'rejects non-positive request limits',
      input: { max_requests: 0, window_seconds: 60 },
      expected: '最大请求需为大于 0 的整数',
    },
    {
      name: 'rejects request limits above the configured cap',
      input: { max_requests: RATE_LIMIT_MAX_REQUESTS + 1, window_seconds: 60 },
      expected: `最大请求不能超过 ${RATE_LIMIT_MAX_REQUESTS}`,
    },
    {
      name: 'rejects windows above the configured cap',
      input: { max_requests: 100, window_seconds: RATE_LIMIT_MAX_WINDOW_SECONDS + 1 },
      expected: `窗口周期不能超过 ${RATE_LIMIT_MAX_WINDOW_SECONDS} 秒`,
    },
  ])('$name', ({ input, expected }) => {
    expect(validateRateLimitSettings(input)).toBe(expected)
  })
})

describe('admin media upload settings helpers', () => {
  it.each([
    {
      name: 'normalizes missing values to defaults',
      input: null,
      expected: DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
    },
    {
      name: 'normalizes numeric values to integers',
      input: { upload_max_concurrent: 12.8 },
      expected: { upload_max_concurrent: 12 },
    },
  ])('$name', ({ input, expected }) => {
    expect(normalizeMediaUploadSetting(input)).toEqual(expected)
  })

  it.each([
    {
      name: 'accepts the minimum concurrent upload bound',
      input: { upload_max_concurrent: MEDIA_UPLOAD_MIN_CONCURRENT },
      expected: '',
    },
    {
      name: 'accepts the maximum concurrent upload bound',
      input: { upload_max_concurrent: MEDIA_UPLOAD_MAX_CONCURRENT },
      expected: '',
    },
    {
      name: 'rejects zero concurrent uploads',
      input: { upload_max_concurrent: 0 },
      expected: `上传并发上限必须在 ${MEDIA_UPLOAD_MIN_CONCURRENT} 到 ${MEDIA_UPLOAD_MAX_CONCURRENT} 之间`,
    },
    {
      name: 'rejects concurrent uploads above the maximum',
      input: { upload_max_concurrent: MEDIA_UPLOAD_MAX_CONCURRENT + 1 },
      expected: `上传并发上限必须在 ${MEDIA_UPLOAD_MIN_CONCURRENT} 到 ${MEDIA_UPLOAD_MAX_CONCURRENT} 之间`,
    },
  ])('$name', ({ input, expected }) => {
    expect(validateMediaUploadSetting(input)).toBe(expected)
  })
})
