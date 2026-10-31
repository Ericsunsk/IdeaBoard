import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useMediaAssetDelivery } from './useMediaAssetDelivery'
import {
  invalidateMediaAssetMetadata,
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '@/api/media/mediaAssetDelivery'

const WORKSPACE_ID = 'ws_test123'
const ASSET_ID = 'asset_uid42'

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  subscribeInvalidation: vi.fn(),
}))

vi.mock('@/api/http/client', () => ({
  api: { request: mocks.request },
}))

vi.mock('@/api/realtime/workspaceEventStream', () => ({
  subscribeWorkspaceMediaInvalidation: mocks.subscribeInvalidation,
}))

function assetEnvelope(status: string, derivativeUrl = '/api/v1/preview.webp') {
  return {
    data: {
      id: ASSET_ID,
      workspace_id: WORKSPACE_ID,
      status: 'ready',
      media_kind: 'image',
      mime_type: 'image/jpeg',
      width: 2048,
      height: 1024,
      size_bytes: 100,
      checksum_sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
      content_url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
      derivatives: {
        thumb: { status, url: derivativeUrl, width: 256, height: 128 },
        preview: { status, url: derivativeUrl, width: 1024, height: 512 },
        'large-preview': { status, url: '/api/v1/large.webp', width: 2048, height: 1024 },
      },
    },
  }
}

describe('useMediaAssetDelivery', () => {
  beforeEach(() => {
    invalidateMediaAssetMetadata()
    mocks.request.mockReset()
    mocks.subscribeInvalidation.mockReset()
    mocks.subscribeInvalidation.mockReturnValue(() => {})
    setMediaWorkspaceIdResolver(() => WORKSPACE_ID)
  })

  afterEach(() => {
    resetMediaWorkspaceIdResolver()
    invalidateMediaAssetMetadata()
  })

  it('loads the selected tier through the workspace asset endpoint', async () => {
    mocks.request.mockResolvedValue(assetEnvelope('ready'))
    const scope = effectScope()
    const delivery = scope.run(() =>
      useMediaAssetDelivery(ref(ASSET_ID), ref<'large-preview'>('large-preview')),
    )
    if (!delivery) throw new Error('delivery composable was not created')

    await vi.waitFor(() => expect(delivery.url.value).toBe('/api/v1/large.webp'))
    expect(mocks.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}`,
      expect.objectContaining({ method: 'GET' }),
    )
    scope.stop()
  })

  it('refreshes the tier from a workspace media invalidation event', async () => {
    mocks.request
      .mockResolvedValueOnce(assetEnvelope('pending'))
      .mockResolvedValueOnce(assetEnvelope('ready'))
    const scope = effectScope()
    const delivery = scope.run(() =>
      useMediaAssetDelivery(ref(ASSET_ID), ref<'preview'>('preview')),
    )
    if (!delivery) throw new Error('delivery composable was not created')

    await vi.waitFor(() => expect(delivery.status.value).toBe('pending'))
    expect(delivery.url.value).toBe('')
    const onInvalidated = mocks.subscribeInvalidation.mock.calls[0]?.[1] as (
      assetId: string,
    ) => void
    onInvalidated(ASSET_ID)

    await vi.waitFor(() => expect(delivery.url.value).toBe('/api/v1/preview.webp'))
    expect(mocks.request).toHaveBeenCalledTimes(2)
    scope.stop()
  })

  it('recovers a zoom-tier read after a transient metadata failure', async () => {
    vi.useFakeTimers()
    const tier = ref<'thumb' | 'large-preview'>('thumb')
    mocks.request
      .mockResolvedValueOnce(assetEnvelope('ready'))
      .mockRejectedValueOnce(Object.assign(new Error('Service unavailable'), { status: 503 }))
      .mockResolvedValueOnce(assetEnvelope('ready'))
    const scope = effectScope()
    try {
      const delivery = scope.run(() => useMediaAssetDelivery(ref(ASSET_ID), tier))
      if (!delivery) throw new Error('delivery composable was not created')
      await vi.advanceTimersByTimeAsync(0)
      expect(delivery.url.value).toBe('/api/v1/preview.webp')

      invalidateMediaAssetMetadata()
      tier.value = 'large-preview'
      await nextTick()
      await vi.advanceTimersByTimeAsync(0)
      expect(delivery.status.value).toBe('error')

      await vi.advanceTimersByTimeAsync(2_000)
      expect(delivery.url.value).toBe('/api/v1/large.webp')
      expect(mocks.request).toHaveBeenCalledTimes(3)
    } finally {
      scope.stop()
      vi.useRealTimers()
    }
  })

  it.each([undefined, 408, 429, 503])('bounds retries for delivery status %s', async (status) => {
    vi.useFakeTimers()
    mocks.request.mockRejectedValue(Object.assign(new Error('Read failed'), { status }))
    const scope = effectScope()
    try {
      const delivery = scope.run(() => useMediaAssetDelivery(ref(ASSET_ID), ref('preview')))
      if (!delivery) throw new Error('delivery composable was not created')
      await vi.advanceTimersByTimeAsync(30_000)
      expect(delivery.status.value).toBe('error')
      expect(mocks.request).toHaveBeenCalledTimes(4)
    } finally {
      scope.stop()
      vi.useRealTimers()
    }
  })

  it.each([401, 403, 404, 422])('does not retry permanent delivery status %s', async (status) => {
    vi.useFakeTimers()
    mocks.request.mockRejectedValue(Object.assign(new Error('Read denied'), { status }))
    const scope = effectScope()
    try {
      const delivery = scope.run(() => useMediaAssetDelivery(ref(ASSET_ID), ref('preview')))
      if (!delivery) throw new Error('delivery composable was not created')
      await vi.advanceTimersByTimeAsync(30_000)
      expect(delivery.status.value).toBe('error')
      expect(mocks.request).toHaveBeenCalledTimes(1)
    } finally {
      scope.stop()
      vi.useRealTimers()
    }
  })

  it('cancels a scheduled delivery retry when disabled', async () => {
    vi.useFakeTimers()
    mocks.request.mockRejectedValue(Object.assign(new Error('Read failed'), { status: 503 }))
    const scope = effectScope()
    const isEnabled = ref(true)
    try {
      const delivery = scope.run(() =>
        useMediaAssetDelivery(ref(ASSET_ID), ref('preview'), { isEnabled }),
      )
      if (!delivery) throw new Error('delivery composable was not created')
      await vi.advanceTimersByTimeAsync(0)
      expect(delivery.status.value).toBe('error')
      isEnabled.value = false
      await nextTick()
      await vi.advanceTimersByTimeAsync(30_000)
      expect(delivery.status.value).toBe('idle')
      expect(mocks.request).toHaveBeenCalledTimes(1)
    } finally {
      scope.stop()
      vi.useRealTimers()
    }
  })

  it('cancels delivery while the asset is not visible', async () => {
    const isVisible = ref(false)
    mocks.request.mockResolvedValue(assetEnvelope('ready'))
    const scope = effectScope()
    const delivery = scope.run(() =>
      useMediaAssetDelivery(ref(ASSET_ID), ref<'thumb'>('thumb'), { isEnabled: isVisible }),
    )
    if (!delivery) throw new Error('delivery composable was not created')
    await Promise.resolve()
    expect(mocks.request).not.toHaveBeenCalled()

    isVisible.value = true
    await vi.waitFor(() => expect(delivery.url.value).toBe('/api/v1/preview.webp'))
    scope.stop()
  })
})
