import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { buildMediaBrowserURL, isMediaRef, mediaRefEquals, type MediaRef } from './mediaRef'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../api/mediaAssetDelivery'

beforeEach(() => {
  setMediaWorkspaceIdResolver(() => 'ws_test123')
})

afterEach(() => {
  resetMediaWorkspaceIdResolver()
})

describe('MediaRef', () => {
  const preview: MediaRef = {
    assetId: 'asset_42',
    derivativeTier: 'preview',
  }

  it('uses the canonical format-agnostic derivative delivery route', () => {
    expect(buildMediaBrowserURL(preview)).toBe(
      '/api/v1/workspaces/ws_test123/media-assets/asset_42/derivatives/preview',
    )
  })

  it('rejects incomplete and invalid references', () => {
    expect(isMediaRef({ assetId: 'asset_42' })).toBe(false)
    expect(isMediaRef({ assetId: '', derivativeTier: 'preview' })).toBe(false)
    expect(isMediaRef({ assetId: 'asset_42', derivativeTier: 'original' })).toBe(false)
  })

  it('rejects provider input profiles at the browser delivery boundary', () => {
    expect(isMediaRef({ assetId: 'asset_42', derivativeTier: 'processing-input' })).toBe(false)
    expect(() =>
      // @ts-expect-error Provider input profiles are not browser display tiers.
      buildMediaBrowserURL({ assetId: 'asset_42', derivativeTier: 'processing-input' }),
    ).toThrow('buildMediaBrowserURL: valid MediaRef is required')
  })

  it('compares asset identity and tier', () => {
    expect(mediaRefEquals(preview, { ...preview })).toBe(true)
    expect(mediaRefEquals(preview, { ...preview, derivativeTier: 'thumb' })).toBe(false)
    expect(mediaRefEquals(preview, { ...preview, assetId: 'asset_43' })).toBe(false)
  })

  it('rejects retired derivative tiers', () => {
    expect(isMediaRef({ assetId: 'asset_42', derivativeTier: 'processing' })).toBe(false)
  })
})
