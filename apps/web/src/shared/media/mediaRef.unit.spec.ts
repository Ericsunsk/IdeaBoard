import { describe, expect, it } from 'vitest'
import { buildMediaBrowserURL, isMediaRef, mediaRefEquals, type MediaRef } from './mediaRef'

describe('MediaRef', () => {
  const preview: MediaRef = {
    assetId: 'asset_42',
    derivativeTier: 'preview',
  }

  it('uses the canonical format-agnostic derivative delivery route', () => {
    expect(buildMediaBrowserURL(preview)).toBe(
      '/api/user/media-assets/asset_42/derivatives/preview/content',
    )
  })

  it('rejects incomplete and invalid references', () => {
    expect(isMediaRef({ assetId: 'asset_42' })).toBe(false)
    expect(isMediaRef({ assetId: '', derivativeTier: 'preview' })).toBe(false)
    expect(isMediaRef({ assetId: 'asset_42', derivativeTier: 'original' })).toBe(false)
  })

  it('compares asset identity and tier', () => {
    expect(mediaRefEquals(preview, { ...preview })).toBe(true)
    expect(mediaRefEquals(preview, { ...preview, derivativeTier: 'thumb' })).toBe(false)
    expect(mediaRefEquals(preview, { ...preview, assetId: 'asset_43' })).toBe(false)
  })

  it('routes the retired processing tier to large-preview without breaking persisted refs', () => {
    const legacy: MediaRef = { assetId: 'asset_42', derivativeTier: 'processing' }
    expect(isMediaRef(legacy)).toBe(true)
    expect(buildMediaBrowserURL(legacy)).toBe(
      '/api/user/media-assets/asset_42/derivatives/large-preview/content',
    )
  })
})
