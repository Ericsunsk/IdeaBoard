import { describe, expect, it } from 'vitest'
import {
  ADMIN_BRANDING_PROJECT_NAME_MAX_CHARS,
  getAdminBrandFallbackText,
  isValidAdminBrandingLogoURL,
  normalizeAdminBrandingSetting,
  validateAdminBrandingSetting,
} from './useAdminBrandingSettings'

describe('admin branding settings helpers', () => {
  it('normalizes empty values to the default project name', () => {
    expect(normalizeAdminBrandingSetting(null)).toEqual({
      project_name: 'IdeaBoard',
      logo_url: '',
    })
    expect(
      normalizeAdminBrandingSetting({
        project_name: '  Creative Ops  ',
        logo_url: '  /uploads/admin/logo.webp  ',
      }),
    ).toEqual({
      project_name: 'Creative Ops',
      logo_url: '/uploads/admin/logo.webp',
    })
  })

  it('validates project name and logo URL', () => {
    expect(
      validateAdminBrandingSetting({
        project_name: 'IdeaBoard',
        logo_url: '/uploads/admin/logo.png',
      }),
    ).toBe('')
    expect(
      validateAdminBrandingSetting({
        project_name: '',
        logo_url: '',
      }),
    ).toBe('项目名称不能为空')
    expect(
      validateAdminBrandingSetting({
        project_name: '名'.repeat(ADMIN_BRANDING_PROJECT_NAME_MAX_CHARS + 1),
        logo_url: '',
      }),
    ).toBe(`项目名称不能超过 ${ADMIN_BRANDING_PROJECT_NAME_MAX_CHARS} 个字符`)
    expect(
      validateAdminBrandingSetting({
        project_name: 'IdeaBoard',
        logo_url: 'javascript:alert(1)',
      }),
    ).toBe('Logo 地址必须为空、/uploads/ 路径或 http(s) URL')
  })

  it('allows only empty, upload, and http logo URLs', () => {
    expect(isValidAdminBrandingLogoURL('')).toBe(true)
    expect(isValidAdminBrandingLogoURL('/uploads/admin/logo.png')).toBe(true)
    expect(isValidAdminBrandingLogoURL('https://example.com/logo.png')).toBe(true)
    expect(isValidAdminBrandingLogoURL('http://example.com/logo.png')).toBe(true)
    expect(isValidAdminBrandingLogoURL('/assets/logo.png')).toBe(false)
    expect(isValidAdminBrandingLogoURL('/uploads/../secret.png')).toBe(false)
    expect(isValidAdminBrandingLogoURL('ftp://example.com/logo.png')).toBe(false)
  })

  it('derives compact brand fallback text', () => {
    expect(getAdminBrandFallbackText('IdeaBoard')).toBe('IB')
    expect(getAdminBrandFallbackText('Design Ops')).toBe('DO')
    expect(getAdminBrandFallbackText('灵感画板')).toBe('灵感')
    expect(getAdminBrandFallbackText('')).toBe('IB')
  })
})
