import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  subscribe: vi.fn(),
}))

vi.mock('./client', () => ({
  api: { request: mocks.request },
}))

vi.mock('./userEventStream', () => ({
  subscribeUserEvent: mocks.subscribe,
}))

function metadata(status: string, url = '/api/preview.webp') {
  return {
    code: 0,
    msg: '',
    data: {
      asset_id: 'asset-42',
      status: 'ready',
      mime_type: 'image/jpeg',
      width: 2048,
      height: 1024,
      size_bytes: 100,
      original: { status: 'ready', url: '/api/original.jpg' },
      derivatives: {
        thumb: { status, url },
        preview: { status, url },
        'large-preview': { status, url: '/api/large.webp' },
        processing: { status, url: '/api/processing.webp' },
      },
    },
  }
}

describe('useMediaAssetDelivery', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    mocks.subscribe.mockReturnValue(vi.fn())
  })

  it('resolves a stable API content URL for an asset', async () => {
    const { resolveMediaAssetOriginalURL } = await import('./mediaAssetDelivery')
    expect(resolveMediaAssetOriginalURL(42)).toBe('/api/user/media-assets/42/original/content')
  })

  it('loads the selected tier and refreshes it from a media derivative event', async () => {
    mocks.request
      .mockResolvedValueOnce(metadata('pending'))
      .mockResolvedValueOnce(metadata('ready'))
    const { useMediaAssetDelivery, invalidateMediaAssetMetadata } =
      await import('./mediaAssetDelivery')
    invalidateMediaAssetMetadata()
    const assetId = ref(42)
    const tier = ref<'preview'>('preview')
    const scope = effectScope()
    const delivery = scope.run(() => useMediaAssetDelivery(assetId, tier))
    if (!delivery) throw new Error('delivery composable was not created')

    await vi.waitFor(() => expect(delivery.status.value).toBe('pending'))
    expect(delivery.url.value).toBe('')
    const listener = mocks.subscribe.mock.calls[0]?.[1] as (event: MessageEvent<string>) => void
    listener(
      new MessageEvent('media.derivative.updated', {
        data: JSON.stringify({ media_asset_id: 42 }),
      }),
    )

    await vi.waitFor(() => expect(delivery.url.value).toBe('/api/preview.webp'))
    expect(mocks.request).toHaveBeenCalledTimes(2)
    scope.stop()
  })

  it('cancels delivery while the asset is not visible', async () => {
    const visibility = ref(false)
    mocks.request.mockResolvedValue(metadata('ready'))
    const { useMediaAssetDelivery } = await import('./mediaAssetDelivery')
    const scope = effectScope()
    const delivery = scope.run(() =>
      useMediaAssetDelivery(ref(42), ref<'thumb'>('thumb'), { enabled: visibility }),
    )
    if (!delivery) throw new Error('delivery composable was not created')
    await Promise.resolve()
    expect(mocks.request).not.toHaveBeenCalled()

    visibility.value = true
    await vi.waitFor(() => expect(delivery.url.value).toBe('/api/preview.webp'))
    scope.stop()
  })

  it('requests large preview generation only when that tier is selected', async () => {
    mocks.request.mockResolvedValue(metadata('ready'))
    const { useMediaAssetDelivery, invalidateMediaAssetMetadata } =
      await import('./mediaAssetDelivery')
    invalidateMediaAssetMetadata()
    const scope = effectScope()
    const delivery = scope.run(() =>
      useMediaAssetDelivery(ref(42), ref<'large-preview'>('large-preview')),
    )
    if (!delivery) throw new Error('delivery composable was not created')

    await vi.waitFor(() => expect(delivery.url.value).toBe('/api/large.webp'))
    expect(mocks.request).toHaveBeenCalledWith(
      '/user/media-assets/42?ensure_tier=large-preview',
      expect.objectContaining({ method: 'GET' }),
    )
    scope.stop()
  })

  it('requests processing generation and never falls back to the original', async () => {
    mocks.request
      .mockResolvedValueOnce(metadata('pending'))
      .mockResolvedValueOnce(metadata('ready'))
    const { resolveMediaAssetProcessingURL, invalidateMediaAssetMetadata } =
      await import('./mediaAssetDelivery')
    invalidateMediaAssetMetadata()

    await expect(resolveMediaAssetProcessingURL(42)).resolves.toBe('/api/processing.webp')
    expect(mocks.request).toHaveBeenCalledWith(
      '/user/media-assets/42',
      expect.objectContaining({ method: 'GET' }),
    )
    expect(mocks.request).toHaveBeenCalledWith(
      '/user/media-assets/42?ensure_tier=processing',
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('loads metadata for multiple assets in one request and hydrates cache', async () => {
    mocks.request.mockResolvedValueOnce({
      code: 0,
      msg: '',
      data: {
        items: [
          { ...metadata('ready').data, asset_id: '42' },
          { ...metadata('ready').data, asset_id: '43' },
        ],
      },
    })
    const { getMediaAssetMetadataBatch, getMediaAssetMetadata, invalidateMediaAssetMetadata } =
      await import('./mediaAssetDelivery')
    invalidateMediaAssetMetadata()
    const result = await getMediaAssetMetadataBatch([43, 42, 42])
    expect([...result.keys()]).toEqual([42, 43])
    expect(mocks.request).toHaveBeenCalledWith(
      '/user/media-assets/delivery/batch',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ asset_ids: ['43', '42'] }),
      }),
    )
    await getMediaAssetMetadata(42)
    expect(mocks.request).toHaveBeenCalledTimes(1)
  })
})
