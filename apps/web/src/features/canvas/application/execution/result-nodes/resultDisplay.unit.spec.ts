import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resolveTaskImageResultAtIndexDisplay,
  resolveTaskImageResultDisplay,
} from './resultDisplay'
import { getMediaAssetMetadata } from '../../../../../shared/api/mediaAssetDelivery'

vi.mock('../../../../../shared/api/mediaAssetDelivery', async (importOriginal) => {
  const original =
    await importOriginal<typeof import('../../../../../shared/api/mediaAssetDelivery')>()
  return { ...original, getMediaAssetMetadata: vi.fn() }
})

const metadataMock = vi.mocked(getMediaAssetMetadata)

function readyMetadata(assetId = 'asset_1') {
  return {
    asset_id: assetId,
    status: 'ready',
    derivatives: { preview: { status: 'ready', url: '/api/preview' } },
  } as never
}

describe('resultDisplay', () => {
  beforeEach(() => {
    metadataMock.mockReset()
    metadataMock.mockResolvedValue(readyMetadata())
  })

  it('waits for the preview derivative before binding the receipt', async () => {
    metadataMock
      .mockResolvedValueOnce({
        asset_id: 'asset_1',
        status: 'ready',
        derivatives: { preview: { status: 'pending' } },
      } as never)
      .mockResolvedValue(readyMetadata())
    const loadImageSize = vi.fn(async () => ({ width: 400, height: 200 }))

    await expect(
      resolveTaskImageResultDisplay(
        { url: 'https://example.com/output.png', assetId: 'asset_1' },
        1,
        loadImageSize,
      ),
    ).resolves.toMatchObject({
      url: '/api/user/media-assets/asset_1/derivatives/preview/content',
      ratio: 0.5,
    })
    expect(metadataMock).toHaveBeenCalledWith('asset_1')
    expect(loadImageSize).toHaveBeenCalled()
  })

  it('warns and resolves when derivative readiness metadata fails', async () => {
    const metadataError = new Error('metadata unavailable')
    metadataMock.mockRejectedValueOnce(metadataError)
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    await expect(
      resolveTaskImageResultDisplay(
        { url: 'https://example.com/output.png', assetId: 'asset_1' },
        1,
        vi.fn(async () => ({ width: 400, height: 200 })),
      ),
    ).resolves.toMatchObject({ ratio: 0.5 })
    expect(warnSpy).toHaveBeenCalledWith('[canvas] failed to await receipt derivative readiness', {
      assetId: 'asset_1',
      operation: 'awaitReceiptDerivativeReady',
      error: metadataError,
    })

    warnSpy.mockRestore()
  })

  it('keeps resolved task image dimensions for display sizing', async () => {
    const loadImageSize = vi.fn()

    await expect(
      resolveTaskImageResultDisplay(
        { url: ' https://example.com/output.png ', assetId: 'asset_1', width: 400, height: 200 },
        1,
        loadImageSize,
      ),
    ).resolves.toEqual({
      url: '/api/user/media-assets/asset_1/derivatives/preview/content',
      mediaRef: { assetId: 'asset_1', derivativeTier: 'preview' },
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
      url: '/api/user/media-assets/asset_42/derivatives/preview/content',
      mediaRef: { assetId: 'asset_42', derivativeTier: 'preview' },
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
      url: '/api/user/media-assets/asset_1/derivatives/preview/content',
      mediaRef: { assetId: 'asset_1', derivativeTier: 'preview' },
      ratio: 1.25,
    })
    expect(loadImageSize).toHaveBeenCalledWith(
      '/api/user/media-assets/asset_1/derivatives/preview/content',
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
      url: '/api/user/media-assets/asset_1/derivatives/preview/content',
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
