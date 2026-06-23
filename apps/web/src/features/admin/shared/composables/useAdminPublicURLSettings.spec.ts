import { describe, expect, it } from 'vitest'
import {
  ADMIN_PUBLIC_URL_MAX_LENGTH,
  isValidAdminPublicBaseURL,
  normalizeAdminPublicURLSetting,
  validateAdminPublicURLSetting,
} from './useAdminPublicURLSettings'

describe('admin public url settings helpers', () => {
  it('normalizes missing values to empty strings', () => {
    expect(normalizeAdminPublicURLSetting(null)).toEqual({
      public_base_url: '',
      public_asset_base_url: '',
    })
  })

  it('trims values and removes trailing slashes', () => {
    expect(
      normalizeAdminPublicURLSetting({
        public_base_url: '  https://app.example.com/  ',
        public_asset_base_url: '  https://api.example.com/uploads/  ',
      }),
    ).toEqual({
      public_base_url: 'https://app.example.com',
      public_asset_base_url: 'https://api.example.com/uploads',
    })
  })

  it('accepts empty and http(s) base URLs', () => {
    expect(isValidAdminPublicBaseURL('')).toBe(true)
    expect(isValidAdminPublicBaseURL('https://app.example.com')).toBe(true)
    expect(isValidAdminPublicBaseURL('http://localhost:8080')).toBe(true)
    expect(isValidAdminPublicBaseURL('https://cdn.example.com/uploads')).toBe(true)
  })

  it('rejects unsafe or non-base URLs', () => {
    expect(isValidAdminPublicBaseURL('/uploads')).toBe(false)
    expect(isValidAdminPublicBaseURL('javascript:alert(1)')).toBe(false)
    expect(isValidAdminPublicBaseURL('ftp://example.com')).toBe(false)
    expect(isValidAdminPublicBaseURL('https://user:pass@example.com')).toBe(false)
    expect(isValidAdminPublicBaseURL('https://example.com?token=1')).toBe(false)
    expect(isValidAdminPublicBaseURL('https://example.com#assets')).toBe(false)
  })

  it('validates both public URL fields', () => {
    expect(
      validateAdminPublicURLSetting({
        public_base_url: 'https://app.example.com',
        public_asset_base_url: 'https://api.example.com',
      }),
    ).toBe('')
    expect(
      validateAdminPublicURLSetting({
        public_base_url: '/app',
        public_asset_base_url: '',
      }),
    ).toBe('前端访问地址必须为空或 http(s) URL')
    expect(
      validateAdminPublicURLSetting({
        public_base_url: '',
        public_asset_base_url: 'mailto:ops@example.com',
      }),
    ).toBe('素材访问地址必须为空或 http(s) URL')
  })

  it('rejects values above the configured length cap', () => {
    expect(
      validateAdminPublicURLSetting({
        public_base_url: `https://example.com/${'a'.repeat(ADMIN_PUBLIC_URL_MAX_LENGTH)}`,
        public_asset_base_url: '',
      }),
    ).toBe(`前端访问地址不能超过 ${ADMIN_PUBLIC_URL_MAX_LENGTH} 个字符`)
  })
})
