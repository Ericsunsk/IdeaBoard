import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resolveTaskImageResultAtIndexDisplay,
  resolveTaskImageResultDisplay,
  type ReceiptDerivativeWaiter,
} from './resultDisplay'
import {
  getWorkspaceMediaAssetMetadata,
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaAssetDelivery'

vi.mock('../../../../../api/mediaAssetDelivery', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../../../api/mediaAssetDelivery')>()
  return { ...original, getWorkspaceMediaAssetMetadata: vi.fn() }
})

const metadataMock = vi.mocked(getWorkspaceMediaAssetMetadata)

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
    setMediaWorkspaceIdResolver(() => 'ws_test123')
  })

  afterEach(() => {
    resetMediaWorkspaceIdResolver()
  })

  it('resolves through an injected event waiter instead of polling', async () => {
    const waitForReceiptDerivative: ReceiptDerivativeWaiter = vi.fn(async () => {})
    const loadImageSize = vi.fn(async () => ({ width: 400, height: 200 }))

    await expect(
      resolveTaskImageResultDisplay(
        { workspaceId: 'ws_test123', assetId: 'asset_1' },
        1,
        loadImageSize,
        undefined,
        waitForReceiptDerivative,
      ),
    ).resolves.toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_1/derivatives/preview',
      ratio: 0.5,
    })
    expect(waitForReceiptDerivative).toHaveBeenCalledTimes(1)
    expect(waitForReceiptDerivative).toHaveBeenCalledWith('ws_test123', 'asset_1', undefined)
    // The injected waiter owns readiness: no metadata GET happens inside resolve.
    expect(metadataMock).not.toHaveBeenCalled()
    expect(loadImageSize).toHaveBeenCalled()
  })

  it('warns and resolves when an injected waiter rejects without abort', async () => {
    const failure = new Error('event wait unavailable')
    const waitForReceiptDerivative: ReceiptDerivativeWaiter = vi.fn(async () => {
      throw failure
    })
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    await expect(
      resolveTaskImageResultDisplay(
        { workspaceId: 'ws_test123', assetId: 'asset_1' },
        1,
        vi.fn(async () => ({ width: 400, height: 200 })),
        undefined,
        waitForReceiptDerivative,
      ),
    ).resolves.toMatchObject({ ratio: 0.5 })
    expect(warnSpy).toHaveBeenCalledWith('[canvas] failed to await receipt derivative readiness', {
      assetId: 'asset_1',
      operation: 'awaitReceiptDerivativeReady',
      error: failure,
    })

    warnSpy.mockRestore()
  })

  it('falls back to the bounded poll when no waiter is injected', async () => {
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
        { workspaceId: 'ws_test123', assetId: 'asset_1' },
        1,
        loadImageSize,
      ),
    ).resolves.toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_1/derivatives/preview',
      ratio: 0.5,
    })
    expect(metadataMock).toHaveBeenCalledWith('ws_test123', 'asset_1', { signal: undefined })
    expect(loadImageSize).toHaveBeenCalled()
  })

  it('warns and resolves when derivative readiness metadata fails', async () => {
    const metadataError = new Error('metadata unavailable')
    metadataMock.mockRejectedValueOnce(metadataError)
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    await expect(
      resolveTaskImageResultDisplay(
        { workspaceId: 'ws_test123', assetId: 'asset_1' },
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
        { workspaceId: 'ws_test123', assetId: 'asset_1', width: 400, height: 200 },
        1,
        loadImageSize,
      ),
    ).resolves.toEqual({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_1/derivatives/preview',
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
        { workspaceId: 'ws_test123', width: 400, height: 200, assetId: 'asset_42' },
        1,
        loadImageSize,
      ),
    ).resolves.toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_42/derivatives/preview',
      mediaRef: { assetId: 'asset_42', derivativeTier: 'preview' },
    })
  })

  it('falls back to ratio when dimensions cannot be probed', async () => {
    const loadImageSize = vi.fn(async () => null)

    await expect(
      resolveTaskImageResultDisplay(
        { workspaceId: 'ws_test123', assetId: 'asset_1' },
        1.25,
        loadImageSize,
      ),
    ).resolves.toEqual({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_1/derivatives/preview',
      mediaRef: { assetId: 'asset_1', derivativeTier: 'preview' },
      ratio: 1.25,
    })
    expect(loadImageSize).toHaveBeenCalledWith(
      '/api/v1/workspaces/ws_test123/media-assets/asset_1/derivatives/preview',
      undefined,
      undefined,
    )
  })

  it('resolves a stable batch slot by result index', async () => {
    await expect(
      resolveTaskImageResultAtIndexDisplay(
        [
          { workspaceId: 'ws_test123', assetId: 'asset_2', resultIndex: 1 },
          { workspaceId: 'ws_test123', assetId: 'asset_1', resultIndex: 0 },
        ],
        0,
        1,
        vi.fn(async () => ({ width: 100, height: 100 })),
      ),
    ).resolves.toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_1/derivatives/preview',
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
