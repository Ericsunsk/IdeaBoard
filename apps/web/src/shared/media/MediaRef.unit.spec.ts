import { describe, expect, it } from 'vitest'
import { buildMediaBrowserURL, isMediaRef, mediaRefEquals, type MediaRef } from './MediaRef'

describe('MediaRef', () => {
  const preview: MediaRef = {
    assetId: 'asset_42',
    derivativeTier: 'preview',
    derivativeFormat: 'webp',
  }

  it('uses the canonical same-origin derivative delivery route', () => {
    expect(buildMediaBrowserURL(preview)).toBe(
      '/api/user/media-assets/asset_42/derivatives/preview/webp/content',
    )
  })

  it('rejects incomplete and invalid references', () => {
    expect(isMediaRef({ assetId: 'asset_42' })).toBe(false)
    expect(isMediaRef({ assetId: '', derivativeTier: 'preview', derivativeFormat: 'webp' })).toBe(false)
    expect(isMediaRef({ assetId: 'asset_42', derivativeTier: 'original', derivativeFormat: 'webp' })).toBe(false)
  })

  it('includes rendition in identity equality', () => {
    expect(mediaRefEquals(preview, { ...preview })).toBe(true)
    expect(mediaRefEquals(preview, { ...preview, derivativeFormat: 'png' })).toBe(false)
  })
})
