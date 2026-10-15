import { effectScope, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { clearImageDimensionCacheForTesting } from '../../../../core/media/imageDimensions'
import { createDeferred } from '../../../../test/deferred'
import {
  invalidateMediaAssetMetadata,
  resetWorkspaceMediaDeliveryOnIdentitySwitch,
  type MediaAssetDeliveryMetadata,
} from '../../../../api/mediaAssetMetadata'
import {
  isTerminalThumbnailMediaError,
  THUMBNAIL_MEDIA_MAX_ATTEMPTS,
  THUMBNAIL_MEDIA_READ_TIMEOUT_MS,
  useSceneThumbnailMedia,
  type SceneThumbnailMediaTransport,
} from './thumbnailMedia'

const production = vi.hoisted(() => ({ read: vi.fn(), subscribe: vi.fn(() => () => {}) }))
vi.mock('../../../../api/generated-query/public/workspace-media/workspace-media', () => ({
  getWorkspaceMediaAsset: production.read,
}))
vi.mock('../../../../api/workspaceEventStream', () => ({
  subscribeWorkspaceMediaInvalidation: production.subscribe,
}))

const WORKSPACE_ID = 'ws_test123'

function readyMetadata(assetId: string, url?: string): MediaAssetDeliveryMetadata {
  const fallback = `/api/v1/workspaces/${WORKSPACE_ID}/media-assets/${assetId}/derivatives/preview`
  return {
    asset_id: assetId,
    status: 'ready',
    mime_type: 'image/jpeg',
    width: 120,
    height: 80,
    size_bytes: 1024,
    original: { status: 'ready', url: fallback },
    derivatives: { preview: { status: 'ready', url: url ?? fallback } },
  }
}

function pendingMetadata(assetId: string): MediaAssetDeliveryMetadata {
  return {
    ...readyMetadata(assetId),
    status: 'pending',
    derivatives: { preview: { status: 'pending' } },
  }
}

type ReadCall = { assetId: string; shouldRefresh: boolean }

function createTransport() {
  const calls: ReadCall[] = []
  const listeners = new Map<string, Set<() => void>>()
  let impl: (assetId: string) => Promise<MediaAssetDeliveryMetadata> = async (assetId) =>
    readyMetadata(assetId)
  const transport: SceneThumbnailMediaTransport = {
    loadImage: vi.fn(async () => true),
    readMetadata: vi.fn(async (_workspaceId: string, assetId: string, _signal: AbortSignal) => {
      calls.push({ assetId, shouldRefresh: false })
      return impl(assetId)
    }),
    refreshMetadata: vi.fn(async (_workspaceId: string, assetId: string, _signal: AbortSignal) => {
      calls.push({ assetId, shouldRefresh: true })
      return impl(assetId)
    }),
    ensureInvalidation: vi.fn(() => () => {}),
    subscribeInvalidation: vi.fn((assetId: string, listener: () => void) => {
      let set = listeners.get(assetId)
      if (!set) {
        set = new Set()
        listeners.set(assetId, set)
      }
      set.add(listener)
      return () => {
        set?.delete(listener)
      }
    }),
  }
  return {
    calls,
    transport,
    setImpl(next: (assetId: string) => Promise<MediaAssetDeliveryMetadata>) {
      impl = next
    },
    fireInvalidation(assetId: string) {
      for (const listener of [...(listeners.get(assetId) ?? [])]) listener()
    },
  }
}

function mountMedia(
  overrides: {
    assetId?: string | null
    isEnabled?: boolean
    isPageVisible?: () => boolean
    transport?: SceneThumbnailMediaTransport
  } = {},
) {
  const scope = effectScope()
  const assetId = ref(overrides.assetId ?? 'asset_1')
  const isEnabled = ref(overrides.isEnabled ?? true)
  const state = scope.run(() =>
    useSceneThumbnailMedia({
      assetId,
      workspaceId: WORKSPACE_ID,
      boardId: 'board-1',
      contentVersion: 'cv-1',
      isEnabled,
      transport: overrides.transport,
      isPageVisible: overrides.isPageVisible,
    }),
  )!
  return { scope, assetId, isEnabled, state }
}

async function flushLoads() {
  await nextTick()
  for (let i = 0; i < 12; i += 1) await Promise.resolve()
}

beforeEach(() => {
  production.read.mockReset()
  vi.useFakeTimers()
  vi.stubGlobal(
    'Image',
    class {
      naturalWidth = 120
      naturalHeight = 80
      get complete() {
        return false
      }
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      set src(value: string) {
        if (value) Promise.resolve().then(() => this.onload?.())
      }
    },
  )
})

afterEach(() => {
  clearImageDimensionCacheForTesting()
  resetWorkspaceMediaDeliveryOnIdentitySwitch()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('useSceneThumbnailMedia', () => {
  test('production refresh retries and invalidation join sibling reads without rebroadcast', async () => {
    const assetId = 'asset_production'
    const envelope = (status: string) => ({
      data: {
        id: assetId,
        status: 'ready',
        mime_type: 'image/jpeg',
        width: 120,
        height: 80,
        size_bytes: 1024,
        content_url: `/api/v1/${assetId}/content`,
        derivatives: { preview: { status, url: `/api/v1/${assetId}/preview` } },
      },
    })
    production.read.mockResolvedValueOnce(envelope('pending')).mockResolvedValue(envelope('ready'))
    const first = mountMedia({ assetId })
    const second = mountMedia({ assetId })
    await vi.advanceTimersByTimeAsync(0)
    expect(production.read).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(800)
    expect(production.read).toHaveBeenCalledTimes(2)
    expect(first.state.status.value).toBe('ready')
    invalidateMediaAssetMetadata(assetId)
    await vi.advanceTimersByTimeAsync(0)
    expect(production.read).toHaveBeenCalledTimes(3)
    expect(second.state.status.value).toBe('ready')
    first.scope.stop()
    second.scope.stop()
  })

  test('invalidation replaces an in-flight pre-event metadata read', async () => {
    const initial = createDeferred<never>()
    production.read
      .mockReset()
      .mockReturnValueOnce(initial.promise)
      .mockRejectedValue({ status: 403 })
    const mounted = mountMedia({ assetId: 'asset_stale_event' })
    await flushLoads()
    invalidateMediaAssetMetadata('asset_stale_event')
    await flushLoads()
    expect(production.read).toHaveBeenCalledTimes(2)
    expect(mounted.state.status.value).toBe('error')
    expect(mounted.state.url.value).toBe('')
    initial.reject(new DOMException('Aborted', 'AbortError'))
    await flushLoads()
    mounted.scope.stop()
  })

  test('bounds real metadata and binary reads across cards and cancels hidden queue entries', async () => {
    const reads: Array<ReturnType<typeof createDeferred<{ data: unknown }>>> = []
    let activeReads = 0
    let maximum = 0
    const finishImages: Array<() => void> = []
    vi.stubGlobal(
      'Image',
      class {
        naturalWidth = 120
        naturalHeight = 80
        get complete() {
          return false
        }
        onload: (() => void) | null = null
        onerror: (() => void) | null = null
        set src(value: string) {
          if (!value) return
          activeReads += 1
          maximum = Math.max(maximum, activeReads)
          finishImages.push(() => {
            activeReads -= 1
            this.onload?.()
          })
        }
      },
    )
    production.read.mockReset().mockImplementation(() => {
      const read = createDeferred<{ data: unknown }>()
      reads.push(read)
      activeReads += 1
      maximum = Math.max(maximum, activeReads)
      return read.promise.finally(() => {
        activeReads -= 1
      })
    })
    const cards = Array.from({ length: 10 }, (_, i) =>
      mountMedia({ assetId: `asset_concurrency_${i}` }),
    )
    await flushLoads()
    expect(reads).toHaveLength(4)
    cards[9]!.isEnabled.value = false
    await flushLoads()
    for (let i = 0; i < 9; i += 1) {
      const id = `asset_concurrency_${i}`
      reads[i]!.resolve({
        data: {
          id,
          status: 'ready',
          mime_type: 'image/jpeg',
          width: 120,
          height: 80,
          size_bytes: 1,
          content_url: `/api/v1/${id}/content`,
          derivatives: { preview: { status: 'ready', url: `/api/v1/${id}/preview` } },
        },
      })
      await flushLoads()
    }
    for (let i = 0; i < 9; i += 1) {
      finishImages[i]!()
      await flushLoads()
    }
    expect(reads).toHaveLength(9)
    expect(maximum).toBeLessThanOrEqual(4)
    expect(cards[8]!.state.status.value).toBe('ready')
    for (const card of cards) card.scope.stop()
  })

  test('SVG binary errors retry without repeatedly resetting the attempt budget', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    for (let i = 0; i < THUMBNAIL_MEDIA_MAX_ATTEMPTS; i += 1) {
      mounted.state.images.value.at(-1)!.onError()
      await vi.advanceTimersByTimeAsync(2_000)
    }
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS)
    expect(mounted.state.status.value).toBe('error')
    mounted.scope.stop()
  })

  test('ignores SVG callbacks after scope disposal', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    const image = mounted.state.images.value[0]!
    mounted.scope.stop()
    const status = mounted.state.status.value
    image.onError()
    image.onLoad()
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.calls).toHaveLength(1)
    expect(mounted.state.status.value).toBe(status)
    expect(mounted.state.images.value).toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  test('ignores old SVG callbacks after identity replacement and same-URL remount', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    const oldIdentity = mounted.state.images.value[0]!
    mounted.assetId.value = 'asset_2'
    oldIdentity.onLoad()
    expect(mounted.state.images.value[0]!.isVisible).toBe(false)
    await flushLoads()
    const oldMount = mounted.state.images.value[0]!
    oldIdentity.onError()
    oldIdentity.onLoad()
    expect(vi.getTimerCount()).toBe(0)
    oldMount.onError()
    await vi.advanceTimersByTimeAsync(800)
    const current = mounted.state.images.value[0]!
    expect(current.url).toBe(oldMount.url)
    expect(current.version).not.toBe(oldMount.version)
    oldMount.onError()
    oldMount.onLoad()
    current.onError()
    await vi.advanceTimersByTimeAsync(1_600)
    mounted.state.images.value[0]!.onError()
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.calls).toHaveLength(4)
    expect(mounted.state.status.value).toBe('error')
    mounted.scope.stop()
  })

  test('cancels unconfirmed SVGs while hidden and resumes when visible', async () => {
    const harness = createTransport()
    let isVisible = true
    const mounted = mountMedia({
      transport: harness.transport,
      isPageVisible: () => isVisible,
    })
    await flushLoads()
    const image = mounted.state.images.value[0]!
    isVisible = false
    document.dispatchEvent(new Event('visibilitychange'))
    image.onError()
    image.onLoad()
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.calls).toHaveLength(1)
    expect(mounted.state.images.value).toEqual([])
    isVisible = true
    document.dispatchEvent(new Event('visibilitychange'))
    await flushLoads()
    expect(harness.calls).toHaveLength(2)
    expect(mounted.state.images.value[0]!.version).not.toBe(image.version)
    mounted.scope.stop()
  })

  test('ignores SVG callbacks when the card becomes disabled', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    const image = mounted.state.images.value[0]!
    mounted.isEnabled.value = false
    image.onError()
    image.onLoad()
    await flushLoads()
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.calls).toHaveLength(1)
    expect(mounted.state.images.value).toEqual([])
    expect(vi.getTimerCount()).toBe(0)
    mounted.scope.stop()
  })

  test('retains the displayed SVG until its replacement confirms load', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    const original = mounted.state.images.value[0]!
    original.onLoad()
    harness.setImpl(async (id) => readyMetadata(id, '/api/changed-preview'))
    harness.fireInvalidation('asset_1')
    await flushLoads()
    expect(mounted.state.images.value.map((image) => image.url)).toEqual([
      original.url,
      '/api/changed-preview',
    ])
    mounted.state.images.value[1]!.onError()
    expect(mounted.state.images.value).toMatchObject([{ url: original.url, isVisible: true }])
    await vi.advanceTimersByTimeAsync(800)
    mounted.state.images.value[1]!.onLoad()
    expect(mounted.state.images.value).toMatchObject([
      { url: '/api/changed-preview', isVisible: true },
    ])
    mounted.scope.stop()
  })

  test('binary failures retry with a bound and preserve the last loaded image', async () => {
    const harness = createTransport()
    const loadImage = vi.fn(async () => true)
    harness.transport.loadImage = loadImage
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    mounted.state.images.value.at(-1)!.onLoad()
    const usable = mounted.state.url.value
    loadImage.mockResolvedValue(false)
    harness.setImpl(async (id) => readyMetadata(id, '/api/changed-preview'))
    harness.fireInvalidation('asset_1')
    await vi.advanceTimersByTimeAsync(5_000)
    expect(loadImage).toHaveBeenCalledTimes(4)
    expect(mounted.state.status.value).toBe('error')
    expect(mounted.state.url.value).toBe(usable)
    loadImage.mockResolvedValue(true)
    harness.fireInvalidation('asset_1')
    await flushLoads()
    expect(mounted.state.status.value).toBe('ready')
    expect(mounted.state.url.value).toBe('/api/changed-preview')
    mounted.scope.stop()
  })

  test('does not commit metadata URLs before image load and drops late binary completion', async () => {
    const harness = createTransport()
    const image = createDeferred<boolean>()
    harness.transport.loadImage = vi.fn(() => image.promise)
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    expect(mounted.state.url.value).toBe('')
    expect(mounted.state.status.value).toBe('loading')
    mounted.scope.stop()
    image.resolve(true)
    await flushLoads()
    expect(mounted.state.url.value).toBe('')
  })

  test('commits ready rasters without scheduling retries', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()

    expect(mounted.state.status.value).toBe('ready')
    expect(mounted.state.url.value).toContain('asset_1')
    expect(harness.calls).toEqual([{ assetId: 'asset_1', shouldRefresh: false }])
    expect(vi.getTimerCount()).toBe(0)
    mounted.scope.stop()
  })

  test('retries pending states with a bound, then surfaces the error', async () => {
    const harness = createTransport()
    harness.setImpl(async (assetId) => pendingMetadata(assetId))
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    expect(mounted.state.status.value).toBe('pending')
    expect(mounted.state.url.value).toBe('')

    await vi.advanceTimersByTimeAsync(800)
    await vi.advanceTimersByTimeAsync(1_600)
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS)

    await vi.advanceTimersByTimeAsync(30_000)
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS)
    expect(mounted.state.status.value).toBe('error')
    expect(vi.getTimerCount()).toBe(0)
    mounted.scope.stop()
  })

  test('recovers through invalidation events without polling', async () => {
    const harness = createTransport()
    harness.setImpl(async (assetId) => pendingMetadata(assetId))
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    expect(harness.calls).toHaveLength(1)

    harness.setImpl(async (assetId) => readyMetadata(assetId))
    harness.fireInvalidation('asset_1')
    await flushLoads()

    expect(harness.calls).toEqual([
      { assetId: 'asset_1', shouldRefresh: false },
      { assetId: 'asset_1', shouldRefresh: true },
    ])
    expect(mounted.state.status.value).toBe('ready')
    expect(mounted.state.url.value).toContain('asset_1')
    mounted.scope.stop()
  })

  test('treats 403 and permanent deletes as terminal without retry', async () => {
    for (const failure of [{ status: 403 }, { status: 404 }, { code: 'media_not_found' }]) {
      const harness = createTransport()
      harness.setImpl(async () => {
        throw failure
      })
      const mounted = mountMedia({ transport: harness.transport })
      await flushLoads()

      expect(isTerminalThumbnailMediaError(failure)).toBe(true)
      expect(mounted.state.status.value).toBe('error')
      expect(mounted.state.url.value).toBe('')
      await vi.advanceTimersByTimeAsync(30_000)
      await flushLoads()
      expect(harness.calls).toHaveLength(1)
      mounted.scope.stop()
    }
  })

  test('keeps the usable image across same-identity transient failures', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    mounted.state.images.value.at(-1)!.onLoad()
    const usable = mounted.state.url.value
    expect(mounted.state.status.value).toBe('ready')

    harness.setImpl(async () => {
      throw new Error('network down')
    })
    harness.fireInvalidation('asset_1')
    await vi.advanceTimersByTimeAsync(5_000)
    await flushLoads()

    expect(mounted.state.url.value).toBe(usable)
    mounted.scope.stop()
  })

  test('stops the stale image when permission is lost for the identity', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    expect(mounted.state.url.value).not.toBe('')

    harness.setImpl(async () => {
      throw { status: 403 }
    })
    harness.fireInvalidation('asset_1')
    await flushLoads()

    expect(mounted.state.url.value).toBe('')
    expect(mounted.state.status.value).toBe('error')
    mounted.scope.stop()
  })

  test('late resolves never overwrite a newer identity', async () => {
    const harness = createTransport()
    const first = createDeferred<MediaAssetDeliveryMetadata>()
    const second = createDeferred<MediaAssetDeliveryMetadata>()
    let readCount = 0
    harness.setImpl(async (_assetId) => {
      readCount += 1
      return readCount === 1 ? first.promise : second.promise
    })
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()

    mounted.assetId.value = 'asset_2'
    await flushLoads()
    first.resolve(readyMetadata('asset_1'))
    second.resolve(readyMetadata('asset_2'))
    await flushLoads()

    expect(mounted.state.url.value).toContain('asset_2')
    mounted.scope.stop()
  })

  test('destroy cancels the load and drops late commits', async () => {
    const harness = createTransport()
    const pending = createDeferred<MediaAssetDeliveryMetadata>()
    harness.setImpl(() => pending.promise)
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()

    mounted.scope.stop()
    pending.resolve(readyMetadata('asset_1'))
    await flushLoads()

    expect(mounted.state.url.value).toBe('')
    expect(mounted.state.status.value).not.toBe('ready')
  })

  test('post-exhaustion events recover with a fresh budget', async () => {
    const harness = createTransport()
    harness.setImpl(async (assetId) => pendingMetadata(assetId))
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    await vi.advanceTimersByTimeAsync(800)
    await vi.advanceTimersByTimeAsync(1_600)
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS)
    expect(mounted.state.status.value).toBe('error')

    // A later invalidation re-opens a bounded budget instead of staying stuck.
    harness.fireInvalidation('asset_1')
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS + 1)
    expect(mounted.state.status.value).toBe('pending')

    harness.setImpl(async (assetId) => readyMetadata(assetId))
    harness.fireInvalidation('asset_1')
    await flushLoads()
    expect(mounted.state.status.value).toBe('ready')
    expect(mounted.state.url.value).toContain('asset_1')
    mounted.scope.stop()
  })

  test('visible exhaustion resumes once the tab turns visible', async () => {
    const harness = createTransport()
    harness.setImpl(async (assetId) => pendingMetadata(assetId))
    let isVisible = true
    const mounted = mountMedia({
      transport: harness.transport,
      isPageVisible: () => isVisible,
    })
    await flushLoads()
    await vi.advanceTimersByTimeAsync(800)
    await vi.advanceTimersByTimeAsync(1_600)
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS)
    expect(mounted.state.status.value).toBe('error')

    // No terminal failure happened, so returning to the tab re-opens recovery
    // instead of staying stuck on the exhausted error.
    isVisible = false
    isVisible = true
    document.dispatchEvent(new Event('visibilitychange'))
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS + 1)
    mounted.scope.stop()
  })

  test('hidden tabs schedule no retries and resume when visible', async () => {
    const harness = createTransport()
    harness.setImpl(async (assetId) => pendingMetadata(assetId))
    let isVisible = false
    const mounted = mountMedia({
      transport: harness.transport,
      isPageVisible: () => isVisible,
    })
    await flushLoads()
    expect(mounted.state.status.value).toBe('pending')

    await vi.advanceTimersByTimeAsync(30_000)
    await flushLoads()
    expect(harness.calls).toHaveLength(0)

    isVisible = true
    document.dispatchEvent(new Event('visibilitychange'))
    await flushLoads()
    expect(harness.calls).toHaveLength(1)
    mounted.scope.stop()
  })

  test('only enabled cards start media reads', async () => {
    const harness = createTransport()
    const mounted = mountMedia({ isEnabled: false, transport: harness.transport })
    await flushLoads()
    expect(harness.calls).toHaveLength(0)

    mounted.isEnabled.value = true
    await flushLoads()
    expect(harness.calls).toHaveLength(1)
    expect(mounted.state.status.value).toBe('ready')
    mounted.scope.stop()
  })

  test('aborts hung reads and retries within the bound', async () => {
    const harness = createTransport()
    harness.setImpl(() => new Promise(() => {}))
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()
    expect(harness.calls).toHaveLength(1)

    // Each hung attempt ends at the read timeout; the retry follows after
    // its backoff (800ms, then 1600ms).
    await vi.advanceTimersByTimeAsync(THUMBNAIL_MEDIA_READ_TIMEOUT_MS)
    await vi.advanceTimersByTimeAsync(800)
    await flushLoads()
    expect(harness.calls).toHaveLength(2)

    await vi.advanceTimersByTimeAsync(THUMBNAIL_MEDIA_READ_TIMEOUT_MS)
    await vi.advanceTimersByTimeAsync(1_600)
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS)

    // Retries stay bounded even when every read hangs.
    await vi.advanceTimersByTimeAsync(THUMBNAIL_MEDIA_READ_TIMEOUT_MS * 2)
    await flushLoads()
    expect(harness.calls).toHaveLength(THUMBNAIL_MEDIA_MAX_ATTEMPTS)
    expect(mounted.state.status.value).toBe('error')
    expect(vi.getTimerCount()).toBe(0)
    mounted.scope.stop()
  })

  test('missing rasters never commit as success facts', async () => {
    const harness = createTransport()
    harness.setImpl(async (assetId) => ({ ...readyMetadata(assetId), derivatives: {} }))
    const mounted = mountMedia({ transport: harness.transport })
    await flushLoads()

    expect(mounted.state.status.value).toBe('failed')
    expect(mounted.state.url.value).toBe('')
    expect(vi.getTimerCount()).toBe(0)
    mounted.scope.stop()
  })
})
