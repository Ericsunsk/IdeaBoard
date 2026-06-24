import { describe, expect, it } from 'vitest'
import {
  getSiteBrandFallbackText,
  isValidSiteBrandingLogoURL,
  normalizeSiteBrandingSetting,
  SITE_BRANDING_PROJECT_NAME_MAX_CHARS,
  validateSiteBrandingSetting,
} from './useSiteBranding'

describe('site branding helpers', () => {
  it('normalizes empty values to the default project name', () => {
    expect(normalizeSiteBrandingSetting(null)).toEqual({
      project_name: 'IdeaBoard',
      logo_url: '',
    })
    expect(
      normalizeSiteBrandingSetting({
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
      validateSiteBrandingSetting({
        project_name: 'IdeaBoard',
        logo_url: '/uploads/admin/logo.png',
      }),
    ).toBe('')
    expect(
      validateSiteBrandingSetting({
        project_name: '',
        logo_url: '',
      }),
    ).toBe('项目名称不能为空')
    expect(
      validateSiteBrandingSetting({
        project_name: '名'.repeat(SITE_BRANDING_PROJECT_NAME_MAX_CHARS + 1),
        logo_url: '',
      }),
    ).toBe(`项目名称不能超过 ${SITE_BRANDING_PROJECT_NAME_MAX_CHARS} 个字符`)
    expect(
      validateSiteBrandingSetting({
        project_name: 'IdeaBoard',
        logo_url: 'javascript:alert(1)',
      }),
    ).toBe('Logo 地址必须为空、/uploads/ 路径或 http(s) URL')
  })

  it('allows only empty, upload, and http logo URLs', () => {
    expect(isValidSiteBrandingLogoURL('')).toBe(true)
    expect(isValidSiteBrandingLogoURL('/uploads/admin/logo.png')).toBe(true)
    expect(isValidSiteBrandingLogoURL('https://example.com/logo.png')).toBe(true)
    expect(isValidSiteBrandingLogoURL('http://example.com/logo.png')).toBe(true)
    expect(isValidSiteBrandingLogoURL('/assets/logo.png')).toBe(false)
    expect(isValidSiteBrandingLogoURL('/uploads/../secret.png')).toBe(false)
    expect(isValidSiteBrandingLogoURL('ftp://example.com/logo.png')).toBe(false)
  })

  it('derives compact brand fallback text', () => {
    expect(getSiteBrandFallbackText('IdeaBoard')).toBe('IB')
    expect(getSiteBrandFallbackText('Design Ops')).toBe('DO')
    expect(getSiteBrandFallbackText('灵感画板')).toBe('灵感')
    expect(getSiteBrandFallbackText('')).toBe('IB')
  })
})
