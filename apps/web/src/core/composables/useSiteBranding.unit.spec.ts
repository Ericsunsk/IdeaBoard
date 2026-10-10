import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import {
  fetchSiteBranding,
  getSiteBrandFallbackText,
  isValidSiteBrandingLogoURL,
  normalizeSiteBrandingSetting,
  resolveSiteBrandingLogoURL,
  SITE_BRANDING_PROJECT_NAME_MAX_CHARS,
  validateSiteBrandingSetting,
} from './useSiteBranding'

vi.mock('../../api/client', () => ({ api: { request: vi.fn() } }))

describe('fetchSiteBranding fallback', () => {
  afterEach(() => {
    vi.mocked(api.request).mockReset()
  })

  it('resolves branding data without touching feature copy', async () => {
    vi.mocked(api.request).mockResolvedValueOnce({
      data: { project_name: 'Acme', logo_url: '' },
    })
    await expect(fetchSiteBranding('Caller fallback')).resolves.toEqual({
      project_name: 'Acme',
      logo_url: '',
    })
    // Supplementary public read: silent 401s degrade locally, never reload.
    expect(vi.mocked(api.request)).toHaveBeenCalledWith(
      '/settings/branding',
      expect.objectContaining({ authFailure: 'silent' }),
    )
  })

  it('surfaces the contract error message when the envelope carries one', async () => {
    vi.mocked(api.request).mockResolvedValueOnce({
      error: { code: 'branding_unavailable', message: 'Brand unavailable' },
    })
    await expect(fetchSiteBranding('Caller fallback')).rejects.toThrow('Brand unavailable')
  })

  it('surfaces the caller-supplied fallback when the transport error is unsafe', async () => {
    vi.mocked(api.request).mockRejectedValueOnce(new Error('dial tcp 127.0.0.1:5432'))
    await expect(fetchSiteBranding('Caller fallback')).rejects.toThrow('Caller fallback')
  })
})

describe('site branding helpers', () => {
  it('normalizes empty values to the default project name', () => {
    expect(normalizeSiteBrandingSetting(null)).toEqual({
      project_name: 'IdeaBoard',
      logo_url: '',
    })
    expect(
      normalizeSiteBrandingSetting({
        project_name: '  Creative Ops  ',
        logo_url: '  /uploads/site/assets/logo.webp  ',
      }),
    ).toEqual({
      project_name: 'Creative Ops',
      logo_url: '/uploads/site/assets/logo.webp',
    })
  })

  it('keeps persisted upload logo URLs canonical', () => {
    const sameOriginLogoURL = new URL(
      '/uploads/site/assets/logo.webp',
      window.location.origin,
    ).toString()

    expect(
      normalizeSiteBrandingSetting({
        project_name: 'IdeaBoard',
        logo_url: sameOriginLogoURL,
      }).logo_url,
    ).toBe('/uploads/site/assets/logo.webp')
  })

  it('rejects same-origin private upload logo URLs after canonicalization', () => {
    const sameOriginPrivateURL = new URL(
      '/uploads/profile/OWNR1A/avatar/logo.webp',
      window.location.origin,
    ).toString()

    expect(
      validateSiteBrandingSetting(
        normalizeSiteBrandingSetting({
          project_name: 'IdeaBoard',
          logo_url: sameOriginPrivateURL,
        }),
      ),
    ).toBe('Logo 地址必须为空、/uploads/site/assets/ 路径或 http(s) URL')
  })

  it('resolves logo URLs for display', () => {
    expect(resolveSiteBrandingLogoURL(' /uploads/site/assets/logo.webp ')).toBe(
      '/uploads/site/assets/logo.webp',
    )
    expect(resolveSiteBrandingLogoURL('https://example.com/logo.webp')).toBe(
      'https://example.com/logo.webp',
    )
  })

  it('validates project name and logo URL', () => {
    expect(
      validateSiteBrandingSetting({
        project_name: 'IdeaBoard',
        logo_url: '/uploads/site/assets/logo.png',
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
    ).toBe('Logo 地址必须为空、/uploads/site/assets/ 路径或 http(s) URL')
  })

  it('allows only empty, site asset, and http logo URLs', () => {
    expect(isValidSiteBrandingLogoURL('')).toBe(true)
    expect(isValidSiteBrandingLogoURL('/uploads/site/assets/logo.png')).toBe(true)
    expect(isValidSiteBrandingLogoURL('https://example.com/logo.png')).toBe(true)
    expect(isValidSiteBrandingLogoURL('http://example.com/logo.png')).toBe(true)
    expect(isValidSiteBrandingLogoURL('/uploads/admin/logo.png')).toBe(false)
    expect(isValidSiteBrandingLogoURL('/uploads/profile/OWNR1A/avatar/logo.png')).toBe(false)
    expect(isValidSiteBrandingLogoURL('/assets/logo.png')).toBe(false)
    expect(isValidSiteBrandingLogoURL('/uploads/site/assets/../secret.png')).toBe(false)
    expect(isValidSiteBrandingLogoURL('ftp://example.com/logo.png')).toBe(false)
  })

  it('derives compact brand fallback text', () => {
    expect(getSiteBrandFallbackText('IdeaBoard')).toBe('IB')
    expect(getSiteBrandFallbackText('Design Ops')).toBe('DO')
    expect(getSiteBrandFallbackText('灵感画板')).toBe('灵感')
    expect(getSiteBrandFallbackText('')).toBe('IB')
  })
})
