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
})
