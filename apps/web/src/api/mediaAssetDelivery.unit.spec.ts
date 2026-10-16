import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import {
  MediaWorkspaceUnavailableError,
  WORKSPACE_MEDIA_BATCH_MAX_ITEMS,
  getMediaAssetMetadata,
  getWorkspaceMediaAssetMetadata,
  getWorkspaceMediaAssetMetadataBatch,
  invalidateMediaAssetMetadata,
  resetMediaWorkspaceIdResolver,
  resolveMediaAssetDerivativeURL,
  resolveMediaAssetOriginalURL,
  resolveMediaAssetProcessingURL,
  setMediaWorkspaceIdResolver,
  useMediaAssetDelivery,
} from './mediaAssetDelivery'

const WORKSPACE_ID = 'ws_test123'
const ASSET_ID = 'asset_uid42'
const refreshOptions: NonNullable<Parameters<typeof getWorkspaceMediaAssetMetadata>[2]> = {}
refreshOptions.force = true

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  subscribeInvalidation: vi.fn(),
  selectedWorkspaceId: { current: 'ws_test123' as string | null },
}))

vi.mock('./client', () => ({
  api: { request: mocks.request },
}))

vi.mock('./workspaceEventStream', () => ({
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
        thumb: { status, url: derivativeUrl },
        preview: { status, url: derivativeUrl },
        'large-preview': { status, url: '/api/v1/large.webp' },
      },
      created_at: '2026-09-25T00:00:00.000Z',
      updated_at: '2026-09-25T00:00:00.000Z',
    },
  }
}

function batchAssetPayload(assetId: string) {
  return {
    id: assetId,
    workspace_id: WORKSPACE_ID,
    status: 'ready',
    media_kind: 'image',
    mime_type: 'image/jpeg',
    width: 2048,
    height: 1024,
    size_bytes: 100,
    checksum_sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    content_url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${assetId}/content`,
    derivatives: {
      thumb: { status: 'ready', url: '/api/v1/thumb.webp' },
      preview: { status: 'ready', url: '/api/v1/preview.webp' },
      'large-preview': { status: 'ready', url: '/api/v1/large.webp' },
    },
    created_at: '2026-09-25T00:00:00.000Z',
    updated_at: '2026-09-25T00:00:00.000Z',
  }
}

describe('workspace mediaAssetDelivery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.selectedWorkspaceId.current = WORKSPACE_ID
    mocks.subscribeInvalidation.mockReturnValue(vi.fn())
    invalidateMediaAssetMetadata()
    setMediaWorkspaceIdResolver(() => mocks.selectedWorkspaceId.current)
  })

  afterEach(() => {
    resetMediaWorkspaceIdResolver()
  })

  it('resolves a stable v1 content URL for an asset', () => {
    expect(resolveMediaAssetOriginalURL(ASSET_ID)).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
    )
  })

  it('resolves a format-agnostic v1 derivative URL', () => {
    expect(resolveMediaAssetDerivativeURL(ASSET_ID, 'preview')).toBe(
      `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/derivatives/preview`,
    )
  })

  it('fails URL builders closed without a workspace selection', () => {
    mocks.selectedWorkspaceId.current = null
    expect(() => resolveMediaAssetOriginalURL(ASSET_ID)).toThrow(MediaWorkspaceUnavailableError)
    expect(() => resolveMediaAssetDerivativeURL(ASSET_ID)).toThrow(MediaWorkspaceUnavailableError)
    expect(mocks.request).not.toHaveBeenCalled()
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
    const visibility = ref(false)
    mocks.request.mockResolvedValue(assetEnvelope('ready'))
    const scope = effectScope()
    const delivery = scope.run(() =>
      useMediaAssetDelivery(ref(ASSET_ID), ref<'thumb'>('thumb'), { isEnabled: visibility }),
    )
    if (!delivery) throw new Error('delivery composable was not created')
    await Promise.resolve()
    expect(mocks.request).not.toHaveBeenCalled()

    visibility.value = true
    await vi.waitFor(() => expect(delivery.url.value).toBe('/api/v1/preview.webp'))
    scope.stop()
  })

  it('reads workspace metadata once and serves repeats from cache', async () => {
    mocks.request.mockResolvedValueOnce(assetEnvelope('ready'))

    const first = await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    const second = await getMediaAssetMetadata(ASSET_ID)
    expect(second).toBe(first)
    expect(first.original.url).toContain(`/media-assets/${ASSET_ID}/content`)
    expect(mocks.request).toHaveBeenCalledTimes(1)
  })

  it('keeps a joined load alive when the first caller aborts', async () => {
    let resolveTransport!: (envelope: unknown) => void
    mocks.request.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveTransport = resolve
      }),
    )
    const firstController = new AbortController()
    const secondController = new AbortController()
    const first = getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID, {
      signal: firstController.signal,
    })
    const second = getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID, {
      signal: secondController.signal,
    })
    expect(mocks.request).toHaveBeenCalledTimes(1)

    firstController.abort()
    await expect(first).rejects.toMatchObject({ name: 'AbortError' })

    resolveTransport(assetEnvelope('ready'))
    await expect(second).resolves.toMatchObject({ asset_id: ASSET_ID })
  })

  it('starts a fresh force transport when the previous only consumer aborts in the same turn', async () => {
    mocks.request
      .mockImplementationOnce(
        (_path: string, options: { signal: AbortSignal }) =>
          new Promise((_resolve, reject) => {
            options.signal.addEventListener(
              'abort',
              () => reject(new DOMException('Aborted', 'AbortError')),
              { once: true },
            )
          }),
      )
      .mockResolvedValueOnce(assetEnvelope('ready'))
    const controller = new AbortController()
    const first = getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID, {
      ...refreshOptions,
      signal: controller.signal,
    })
    controller.abort()
    const second = getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID, refreshOptions)
    await expect(first).rejects.toMatchObject({ name: 'AbortError' })
    await expect(second).resolves.toMatchObject({ asset_id: ASSET_ID })
    expect(mocks.request).toHaveBeenCalledTimes(2)
  })

  it('replaces a stale read and shares concurrent force reads', async () => {
    const resolvers: Array<(envelope: unknown) => void> = []
    const deferred = () =>
      new Promise<unknown>((resolve) => {
        resolvers.push(resolve)
      })
    mocks.request.mockReturnValueOnce(deferred()).mockReturnValueOnce(deferred())
    const stale = getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    const fresh = getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID, refreshOptions)
    const joined = getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID, refreshOptions)
    expect(mocks.request).toHaveBeenCalledTimes(2)

    const staleSignal = mocks.request.mock.calls[0]?.[1] as { signal?: AbortSignal }
    expect(staleSignal.signal?.aborted).toBe(true)

    resolvers[1]?.(assetEnvelope('ready'))
    await expect(fresh).resolves.toMatchObject({ asset_id: ASSET_ID })
    await expect(joined).resolves.toMatchObject({ asset_id: ASSET_ID })
    // The superseded transport ignores its abort in the mock; settling it
    // late must neither reject the fresh read nor poison the cache.
    resolvers[0]?.(assetEnvelope('ready'))
    await expect(stale).resolves.toMatchObject({ asset_id: ASSET_ID })
    const reread = await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)
    expect(reread).toMatchObject({ asset_id: ASSET_ID })
    expect(mocks.request).toHaveBeenCalledTimes(2)
  })

  it('fails metadata reads closed without a workspace selection', async () => {
    mocks.selectedWorkspaceId.current = null

    await expect(getMediaAssetMetadata(ASSET_ID)).rejects.toBeInstanceOf(
      MediaWorkspaceUnavailableError,
    )
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('resolves the original content as the processing source', async () => {
    mocks.request.mockResolvedValueOnce(assetEnvelope('ready'))

    await expect(resolveMediaAssetProcessingURL(ASSET_ID)).resolves.toContain(
      `/media-assets/${ASSET_ID}/content`,
    )
    expect(mocks.request).toHaveBeenCalledTimes(1)
    expect(mocks.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}`,
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('fails processing resolution closed while the asset is not ready', async () => {
    mocks.request.mockResolvedValueOnce({
      data: {
        id: ASSET_ID,
        workspace_id: WORKSPACE_ID,
        status: 'processing',
        media_kind: 'image',
        mime_type: 'image/jpeg',
        width: 0,
        height: 0,
        size_bytes: 0,
        checksum_sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
        content_url: `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${ASSET_ID}/content`,
        derivatives: {},
        created_at: '2026-09-25T00:00:00.000Z',
        updated_at: '2026-09-25T00:00:00.000Z',
      },
    })

    await expect(resolveMediaAssetProcessingURL(ASSET_ID)).rejects.toMatchObject({
      name: 'MediaAssetDerivativeNotReadyError',
    })
  })

  it('reads a batch through one generated batch transport in request order', async () => {
    const secondId = 'asset_uid43'
    mocks.request.mockResolvedValueOnce({
      data: {
        items: [batchAssetPayload(ASSET_ID), batchAssetPayload(secondId)],
      },
    })

    const items = await getWorkspaceMediaAssetMetadataBatch(WORKSPACE_ID, [ASSET_ID, secondId])

    expect(items.map((item) => item.asset_id)).toEqual([ASSET_ID, secondId])
    expect(mocks.request).toHaveBeenCalledTimes(1)
    expect(mocks.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/media-assets/batch`,
      expect.objectContaining({ method: 'POST' }),
    )
    const body = mocks.request.mock.calls[0]?.[1] as { body?: string }
    expect(JSON.parse(body.body as string)).toEqual({
      data: { asset_ids: [ASSET_ID, secondId] },
    })
  })

  it('serves batch repeats from the shared cache without transport', async () => {
    mocks.request.mockResolvedValueOnce({
      data: { items: [batchAssetPayload(ASSET_ID)] },
    })

    const first = await getWorkspaceMediaAssetMetadataBatch(WORKSPACE_ID, [ASSET_ID])
    const second = await getWorkspaceMediaAssetMetadataBatch(WORKSPACE_ID, [ASSET_ID])

    expect(second[0]).toBe(first[0])
    expect(mocks.request).toHaveBeenCalledTimes(1)
  })

  it('hydrates the single-read cache from batch results', async () => {
    mocks.request.mockResolvedValueOnce({
      data: { items: [batchAssetPayload(ASSET_ID)] },
    })

    await getWorkspaceMediaAssetMetadataBatch(WORKSPACE_ID, [ASSET_ID])
    const single = await getWorkspaceMediaAssetMetadata(WORKSPACE_ID, ASSET_ID)

    expect(single.asset_id).toBe(ASSET_ID)
    expect(mocks.request).toHaveBeenCalledTimes(1)
  })

  it('fails batch reads closed on empty or blank ids', async () => {
    await expect(getWorkspaceMediaAssetMetadataBatch(WORKSPACE_ID, [])).rejects.toThrow()
    await expect(
      getWorkspaceMediaAssetMetadataBatch(WORKSPACE_ID, [ASSET_ID, '   ']),
    ).rejects.toThrow()
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('fails batch reads closed without a workspace selection', async () => {
    mocks.selectedWorkspaceId.current = null

    await expect(getWorkspaceMediaAssetMetadataBatch(null, [ASSET_ID])).rejects.toBeInstanceOf(
      MediaWorkspaceUnavailableError,
    )
    expect(mocks.request).not.toHaveBeenCalled()
  })

  it('exposes the backend batch bound', () => {
    expect(WORKSPACE_MEDIA_BATCH_MAX_ITEMS).toBe(50)
  })
})
