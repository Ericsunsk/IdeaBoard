import { describe, expect, it, vi } from 'vitest'
import {
  resolveTaskImageResultAtIndexDisplay,
  resolveTaskImageResultDisplay,
} from './flowImageDirectActionTaskResult'

describe('flowImageDirectActionTaskResult', () => {
  it('keeps resolved task image dimensions for display sizing', async () => {
    const loadImageSize = vi.fn()

    await expect(
      resolveTaskImageResultDisplay(
        { url: ' https://example.com/output.png ', assetId: 'asset_1', width: 400, height: 200 },
        1,
        loadImageSize,
      ),
    ).resolves.toEqual({
      url: '/api/user/media-assets/asset_1/derivatives/preview/webp/content',
      mediaRef: { assetId: 'asset_1', derivativeTier: 'preview', derivativeFormat: 'webp' },
      ratio: 0.5,
      width: 400,
      height: 200,
    })
    expect(loadImageSize).not.toHaveBeenCalled()
  })

  it('derives a MediaRef delivery URL from the public task output identity', async () => {
    const loadImageSize = vi.fn()

    await expect(
      resolveTaskImageResultDisplay(
        { url: 'https://example.com/output.png', width: 400, height: 200, assetId: 'asset_42' },
        1,
        loadImageSize,
      ),
    ).resolves.toMatchObject({
      url: '/api/user/media-assets/asset_42/derivatives/preview/webp/content',
      mediaRef: { assetId: 'asset_42', derivativeTier: 'preview', derivativeFormat: 'webp' },
    })
  })

  it('falls back to ratio when dimensions cannot be probed', async () => {
    const loadImageSize = vi.fn(async () => null)

    await expect(
      resolveTaskImageResultDisplay(
        { url: 'https://example.com/output.png', assetId: 'asset_1' },
        1.25,
        loadImageSize,
      ),
    ).resolves.toEqual({
      url: '/api/user/media-assets/asset_1/derivatives/preview/webp/content',
      mediaRef: { assetId: 'asset_1', derivativeTier: 'preview', derivativeFormat: 'webp' },
      ratio: 1.25,
    })
    expect(loadImageSize).toHaveBeenCalledWith(
      '/api/user/media-assets/asset_1/derivatives/preview/webp/content',
    )
  })

  it('resolves a stable batch slot by result index', async () => {
    await expect(
      resolveTaskImageResultAtIndexDisplay(
        [
          { url: 'https://example.com/second.png', assetId: 'asset_2', resultIndex: 1 },
          { url: 'https://example.com/first.png', assetId: 'asset_1', resultIndex: 0 },
        ],
        0,
        1,
        vi.fn(async () => ({ width: 100, height: 100 })),
      ),
    ).resolves.toMatchObject({
      url: '/api/user/media-assets/asset_1/derivatives/preview/webp/content',
      resultIndex: 0,
    })
  })

  it('fails a missing batch slot instead of falling through to another result', async () => {
    await expect(
      resolveTaskImageResultAtIndexDisplay(
        [
          { url: 'https://example.com/first.png', resultIndex: 0 },
          { url: 'https://example.com/third.png', resultIndex: 2 },
        ],
        1,
        1,
        vi.fn(async () => ({ width: 100, height: 100 })),
      ),
    ).rejects.toThrow('Task completed but did not return this image.')
  })
})
