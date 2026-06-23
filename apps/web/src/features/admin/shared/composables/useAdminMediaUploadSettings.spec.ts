import { describe, expect, it } from 'vitest'
import {
  DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
  MEDIA_UPLOAD_MAX_CONCURRENT,
  MEDIA_UPLOAD_MIN_CONCURRENT,
  normalizeMediaUploadSetting,
  validateMediaUploadSetting,
} from './useAdminMediaUploadSettings'

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
    expect(validateMediaUploadSetting({ upload_max_concurrent: MEDIA_UPLOAD_MAX_CONCURRENT + 1 }))
      .toBe(
        `上传并发上限必须在 ${MEDIA_UPLOAD_MIN_CONCURRENT} 到 ${MEDIA_UPLOAD_MAX_CONCURRENT} 之间`,
      )
  })
})
