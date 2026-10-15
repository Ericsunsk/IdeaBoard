import { describe, expect, it } from 'vitest'
import { mediaDisplayFixture } from '../../test/mediaDisplay'
import { fitMediaDisplaySize, resolveMediaDisplayUrl, toMediaDisplay } from './mediaDisplay'

describe('Media display', () => {
  it('uses one bounded display tier across layout and DPR demands without originals', () => {
    const display = mediaDisplayFixture()
    expect(resolveMediaDisplayUrl(display, 128 * 2)).toBe('/api/media/previews/fixture/thumb')
    expect(resolveMediaDisplayUrl(display, 320 * 2)).toBe('/api/media/previews/fixture/preview')
    expect(resolveMediaDisplayUrl(display, 320 * 3)).toBe(
      '/api/media/previews/fixture/large-preview',
    )
    expect(resolveMediaDisplayUrl(display, 4096)).toBe('/api/media/previews/fixture/large-preview')
    expect(resolveMediaDisplayUrl(undefined, 256)).toBe('')
  })

  it('preserves presentation URLs and rejects external or malformed display variants', () => {
    const display = mediaDisplayFixture('encoded%2Fsource')
    expect(toMediaDisplay(display)).toEqual(display)
    expect(toMediaDisplay(undefined)).toBeUndefined()
    expect(() =>
      toMediaDisplay({
        variants: [{ tier: 'thumb', max_side: 256, url: 'https://example.com/original.jpg' }],
      }),
    ).toThrow()
    expect(() =>
      toMediaDisplay({ variants: [{ tier: 'original', max_side: 4096, url: '/api/content' }] }),
    ).toThrow()
  })

  it('fits portrait and landscape display demand within both lightbox bounds', () => {
    expect(fitMediaDisplaySize({ width: 800, height: 1600 }, 1000, 800)).toEqual({
      width: 400,
      height: 800,
    })
    expect(fitMediaDisplaySize({ width: 1600, height: 800 }, 1000, 800)).toEqual({
      width: 1000,
      height: 500,
    })
  })
})

describe('Media display API origin', () => {
  it('uses the configured development API origin without duplicating its prefix', async () => {
    const { vi } = await import('vitest')
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.test/api')
    vi.stubEnv('PROD', false)
    vi.stubEnv('MODE', 'development')
    vi.resetModules()
    try {
      const { resolveMediaDisplayUrl } = await import('./mediaDisplay')
      expect(resolveMediaDisplayUrl(mediaDisplayFixture(), 256)).toBe(
        'https://api.example.test/api/media/previews/fixture/thumb',
      )
    } finally {
      vi.unstubAllEnvs()
      vi.resetModules()
    }
  })
})
