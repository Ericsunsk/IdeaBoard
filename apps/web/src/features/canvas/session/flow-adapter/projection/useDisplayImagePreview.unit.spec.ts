import { afterEach, beforeEach, describe, expect, it, test, vi } from 'vitest'
import { createDeferred } from '../../../../../test/deferred'
import { effectScope, nextTick, ref } from 'vue'

const canvasMediaIOMocks = vi.hoisted(() => ({
  canvasToBlob: vi.fn(),
  resolveFetchUrl: vi.fn((url: string) => url),
}))

vi.mock('../../media/canvasMediaIo.ts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../media/canvasMediaIo.ts')>()
  return {
    ...actual,
    canvasToBlob: canvasMediaIOMocks.canvasToBlob,
    resolveFetchUrl: canvasMediaIOMocks.resolveFetchUrl,
  }
})

vi.mock('../../../../../api/workspaceEventStream', () => ({
  subscribeWorkspaceMediaInvalidation: vi.fn(() => vi.fn()),
}))

// Workspace selection for media delivery resolves through the IoC seam
// (`setMediaWorkspaceIdResolver`), not the workspaceContext module.
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaWorkspaceScope'

function createCanvasMock() {
  return {
    width: 0,
    height: 0,
    getContext: vi.fn(() => ({
      drawImage: vi.fn(),
      getImageData: vi.fn(() => ({ data: new Uint8ClampedArray([255, 255, 255, 255]) })),
    })),
  }
}

// `vi.resetModules()` in beforeEach gives every test fresh module
// instances, so the media-workspace seam must be wired on the fresh copy
// after each dynamic import — static wiring would hit a discarded instance.
async function importPreviewModule() {
  const [{ useDisplayImagePreview }, { setMediaWorkspaceIdResolver }] = await Promise.all([
    import('./useDisplayImagePreview'),
    import('../../../../../api/mediaWorkspaceScope'),
  ])
  setMediaWorkspaceIdResolver(() => 'ws_test123')
  return { useDisplayImagePreview }
}

function mediaAssetEnvelope(
  assetId: string,
  derivatives: Record<string, { status: string; format?: string; url?: string }>,
) {
  return {
    data: {
      id: assetId,
      workspace_id: 'ws_test123',
      status: 'ready',
      media_kind: 'image',
      mime_type: 'image/jpeg',
      width: 2048,
      height: 1024,
      size_bytes: 100,
      checksum_sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
      content_url: `/api/v1/workspaces/ws_test123/media-assets/${assetId}/content`,
      derivatives,
      created_at: '2026-09-25T00:00:00.000Z',
      updated_at: '2026-09-25T00:00:00.000Z',
    },
  }
}

function readyDerivatives(assetId: string) {
  return {
    thumb: {
      status: 'ready',
      format: 'webp',
      url: `/api/v1/workspaces/ws_test123/media-assets/${assetId}/derivatives/thumb`,
    },
    preview: {
      status: 'ready',
      format: 'webp',
      url: `/api/v1/workspaces/ws_test123/media-assets/${assetId}/derivatives/preview`,
    },
    'large-preview': {
      status: 'ready',
      format: 'webp',
      url: `/api/v1/workspaces/ws_test123/media-assets/${assetId}/derivatives/large-preview`,
    },
  }
}
async function flushPreviewTasks() {
  for (let index = 0; index < 10; index += 1) {
    await Promise.resolve()
  }
}

async function waitForPreviewWork() {
  for (let index = 0; index < 10; index += 1) {
    await flushPreviewTasks()
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
}

function stubDeferredImageDecode() {
  const decodes: Array<() => void> = []
  vi.stubGlobal(
    'Image',
    class {
      src = ''
      decode() {
        return new Promise<void>((resolve) => decodes.push(resolve))
      }
    },
  )
  return decodes
}

describe('useDisplayImagePreview', () => {
  beforeEach(() => {
    delete window.__canvasInteractionBenchmarkCounters
    setMediaWorkspaceIdResolver(() => 'ws_test123')
    vi.resetModules()
    vi.stubGlobal('Image', class {})
    const createElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      if (tagName === 'canvas') return createCanvasMock() as unknown as HTMLCanvasElement
      return createElement(tagName)
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = typeof input === 'string' ? input : input.toString()
        if (url.includes('/v1/workspaces/ws_test123/media-assets/asset-42')) {
          return new Response(
            JSON.stringify(mediaAssetEnvelope('asset-42', readyDerivatives('asset-42'))),
            {
              headers: { 'content-type': 'application/json' },
            },
          )
        }
        return {
          ok: true,
          blob: async () => new Blob(['image'], { type: 'image/jpeg' }),
        }
      }),
    )
    vi.stubGlobal(
      'createImageBitmap',
      vi.fn(async () => ({
        width: 2048,
        height: 1024,
        close: vi.fn(),
      })),
    )
    vi.spyOn(URL, 'createObjectURL').mockImplementation((object: Blob | MediaSource) =>
      object instanceof Blob ? `blob:${object.size}` : 'blob:media-source',
    )
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    canvasMediaIOMocks.canvasToBlob.mockReset()
    canvasMediaIOMocks.canvasToBlob.mockResolvedValue(new Blob(['preview'], { type: 'image/webp' }))
    canvasMediaIOMocks.resolveFetchUrl.mockImplementation((url: string) => url)
  })

  afterEach(() => {
    vi.useRealTimers()
    delete window.__canvasInteractionBenchmarkCounters
    resetMediaWorkspaceIdResolver()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('regenerates preview only when the resolved max side tier changes', async () => {
    const sourceUrl = ref('https://example.com/image.jpg')
    const maxSide = ref<number | null>(320)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('blob:7')
    })

    expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(1)

    maxSide.value = 320
    await nextTick()
    await waitForPreviewWork()
    expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(1)

    maxSide.value = 640
    await nextTick()
    await vi.waitFor(() => {
      expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(2)
    })
    scope.stop()
    await waitForPreviewWork()
  })

  it('uses the source URL directly when preview max side is null', async () => {
    canvasMediaIOMocks.canvasToBlob.mockClear()
    vi.mocked(fetch).mockClear()
    const sourceUrl = ref('https://example.com/full.jpg')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await waitForPreviewWork()

    expect(preview.displayUrl.value).toBe('https://example.com/full.jpg')
    expect(fetch).not.toHaveBeenCalled()
    expect(canvasMediaIOMocks.canvasToBlob).not.toHaveBeenCalled()
    scope.stop()
  })

  it('resizes managed uploads in the browser when no media asset id is available', async () => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: 640,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    expect(preview.displayUrl.value).toBe('')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('blob:7')
    })

    expect(fetch).toHaveBeenCalledWith(
      '/uploads/tasks/U123/board/content/image.png',
      expect.objectContaining({ credentials: 'include', signal: expect.any(AbortSignal) }),
    )
    expect(createImageBitmap).toHaveBeenCalled()
    expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalled()
    scope.stop()
  })

  it('gates v1 media-asset delivery URLs like managed uploads', async () => {
    const sourceUrl = ref('/api/v1/workspaces/ws_test123/media-assets/asset-99/content')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: 640,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    expect(preview.displayUrl.value).toBe('')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('blob:7')
    })

    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/workspaces/ws_test123/media-assets/asset-99/content',
      expect.objectContaining({ credentials: 'include', signal: expect.any(AbortSignal) }),
    )
    expect(createImageBitmap).toHaveBeenCalled()
    expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalled()
    scope.stop()
  })

  it('uses the canonical media derivative and never falls back to the original asset', async () => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: 640, assetId: 'asset-42' }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() =>
      expect(preview.displayUrl.value).toBe(
        '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview',
      ),
    )
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/workspaces/ws_test123/media-assets/asset-42',
      expect.objectContaining({ credentials: 'include', signal: expect.any(AbortSignal) }),
    )
    scope.stop()
  })

  it('keeps the current media derivative visible while a zoom tier refreshes', async () => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const maxSide = ref(256)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide, assetId: 'asset-42' }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() =>
      expect(preview.displayUrl.value).toBe(
        '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/thumb',
      ),
    )

    maxSide.value = 640
    await nextTick()
    expect(preview.displayUrl.value).toBe(
      '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/thumb',
    )

    await vi.waitFor(() =>
      expect(preview.displayUrl.value).toBe(
        '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview',
      ),
    )
    scope.stop()
  })

  it('holds a late decoded derivative until display commits resume without decoding again', async () => {
    const decodes = stubDeferredImageDecode()
    const { useDisplayImagePreview } = await importPreviewModule()
    const maxSide = ref<number | null>(256)
    const isDisplayCommitEnabled = ref(true)
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/uploads/image.png', {
        maxSide,
        assetId: 'asset-42',
        isDisplayCommitEnabled,
      }),
    )!
    await vi.waitFor(() => expect(decodes).toHaveLength(1))
    decodes[0]!()
    await flushPreviewTasks()
    const previousUrl = preview.displayUrl.value
    expect(previousUrl).toContain('/derivatives/thumb')

    maxSide.value = null
    await vi.waitFor(() => expect(decodes).toHaveLength(2))
    isDisplayCommitEnabled.value = false
    decodes[1]!()
    await flushPreviewTasks()
    expect(preview.displayUrl.value).toBe(previousUrl)

    isDisplayCommitEnabled.value = true
    expect(preview.displayUrl.value).toContain('/content')
    expect(decodes).toHaveLength(2)
    scope.stop()
  })

  it('discards a held decoded candidate when the requested tier changes before resuming', async () => {
    const decodes = stubDeferredImageDecode()
    const { useDisplayImagePreview } = await importPreviewModule()
    const maxSide = ref<number | null>(256)
    const isDisplayCommitEnabled = ref(true)
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/uploads/image.png', {
        maxSide,
        assetId: 'asset-42',
        isDisplayCommitEnabled,
      }),
    )!
    await vi.waitFor(() => expect(decodes).toHaveLength(1))
    decodes[0]!()
    await flushPreviewTasks()
    const previousUrl = preview.displayUrl.value

    maxSide.value = null
    await vi.waitFor(() => expect(decodes).toHaveLength(2))
    isDisplayCommitEnabled.value = false
    decodes[1]!()
    await flushPreviewTasks()
    maxSide.value = 640
    isDisplayCommitEnabled.value = true
    expect(preview.displayUrl.value).toBe(previousUrl)
    await vi.waitFor(() => expect(decodes).toHaveLength(3))
    decodes[2]!()
    await flushPreviewTasks()
    expect(preview.displayUrl.value).toContain('/derivatives/preview')
    scope.stop()
  })

  it('holds an entry decode throughout exit movement and settle, then keeps the final zoom tier', async () => {
    const decodes = stubDeferredImageDecode()
    const { useDisplayImagePreview } = await importPreviewModule()
    const { useCanvasImageQualityZoom } = await import('../viewport/useCanvasImageQualityZoom')
    const { useCanvasImagePreviewQuality } =
      await import('../viewport/useCanvasImagePreviewQuality')
    const zoomLevel = ref(30)
    const shouldForceFullQuality = ref(false)
    const scope = effectScope()
    const state = scope.run(() => {
      const quality = useCanvasImageQualityZoom(zoomLevel, () => 'board-a')
      const { maxSide } = useCanvasImagePreviewQuality({
        imageQuality: quality.imageQuality,
        shouldForceFullQuality,
      })
      return {
        ...quality,
        preview: useDisplayImagePreview('/uploads/image.png', {
          maxSide,
          assetId: 'asset-42',
          isDisplayCommitEnabled: quality.imageQuality.isSettled,
        }),
      }
    })!
    await vi.waitFor(() => expect(decodes).toHaveLength(1))
    decodes[0]!()
    await flushPreviewTasks()
    const previousUrl = state.preview.displayUrl.value
    expect(previousUrl).toContain('/derivatives/thumb')

    vi.useFakeTimers()
    const finishEntry = state.beginMovement()
    shouldForceFullQuality.value = true
    zoomLevel.value = 200
    finishEntry()
    await vi.advanceTimersByTimeAsync(80)
    expect(decodes).toHaveLength(2)

    const finishExit = state.beginMovement()
    shouldForceFullQuality.value = false
    zoomLevel.value = 30
    decodes[1]!()
    await flushPreviewTasks()
    expect(state.preview.displayUrl.value).toBe(previousUrl)
    finishExit()
    await vi.advanceTimersByTimeAsync(79)
    expect(state.imageQuality.isSettled.value).toBe(false)
    expect(state.preview.displayUrl.value).toBe(previousUrl)
    await vi.advanceTimersByTimeAsync(1)
    expect(state.imageQuality.isSettled.value).toBe(true)
    expect(state.preview.displayUrl.value).toBe(previousUrl)
    expect(decodes).toHaveLength(2)
    scope.stop()
  })

  it.each([2048, null])('resolves full quality (%s) to the content route', async (maxSide) => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide, assetId: 'asset-42' }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() =>
      expect(preview.displayUrl.value).toBe(
        '/api/v1/workspaces/ws_test123/media-assets/asset-42/content',
      ),
    )
    scope.stop()
  })

  it('decodes the ready original when the initial derivative cannot load', async () => {
    class DerivativeFailureImage {
      src = ''

      decode() {
        return this.src.includes('/derivatives/')
          ? Promise.reject(new Error('Image failed to load'))
          : Promise.resolve()
      }
    }
    vi.stubGlobal('Image', DerivativeFailureImage)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/api/v1/workspaces/ws_test123/media-assets/asset-42/content', {
        maxSide: 640,
        assetId: 'asset-42',
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() =>
      expect(preview.displayUrl.value).toBe(
        '/api/v1/workspaces/ws_test123/media-assets/asset-42/content',
      ),
    )
    scope.stop()
  })

  it('retries a temporarily failed zoom-tier decode while retaining the previous frame', async () => {
    let originalAttempts = 0
    vi.stubGlobal(
      'Image',
      class {
        src = ''
        decode() {
          if (this.src.endsWith('/content') && originalAttempts++ === 0) {
            return Promise.reject(new Error('Image temporarily unavailable'))
          }
          return Promise.resolve()
        }
      },
    )
    const { useDisplayImagePreview } = await importPreviewModule()
    const maxSide = ref<number | null>(256)
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/uploads/image.png', { maxSide, assetId: 'asset-42' }),
    )!
    await vi.waitFor(() => expect(preview.displayUrl.value).toContain('/derivatives/thumb'))
    const previousUrl = preview.displayUrl.value

    vi.useFakeTimers()
    maxSide.value = null
    await flushPreviewTasks()
    expect(originalAttempts).toBe(1)
    expect(preview.displayUrl.value).toBe(previousUrl)
    await vi.advanceTimersByTimeAsync(800)
    expect(originalAttempts).toBe(2)
    expect(preview.displayUrl.value).toContain('/content')
    scope.stop()
  })

  it('recovers the requested derivative after it and the original fail without swapping during movement', async () => {
    const decodedUrls: string[] = []
    let isRecovered = false
    vi.stubGlobal(
      'Image',
      class {
        src = ''
        decode() {
          decodedUrls.push(this.src)
          return this.src.endsWith('/thumb') || isRecovered
            ? Promise.resolve()
            : Promise.reject(new Error('Image temporarily unavailable'))
        }
      },
    )
    const { useDisplayImagePreview } = await importPreviewModule()
    const maxSide = ref<number | null>(256)
    const isDisplayCommitEnabled = ref(true)
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/uploads/image.png', {
        maxSide,
        assetId: 'asset-42',
        isDisplayCommitEnabled,
      }),
    )!
    await vi.waitFor(() => expect(preview.displayUrl.value).toContain('/derivatives/thumb'))
    const previousUrl = preview.displayUrl.value
    const previewUrl = previousUrl.replace('/thumb', '/preview')
    const originalUrl = previousUrl.replace('/derivatives/thumb', '/content')

    vi.useFakeTimers()
    maxSide.value = 640
    await flushPreviewTasks()
    expect(decodedUrls).toEqual([previousUrl, previewUrl, originalUrl])
    expect(preview.displayUrl.value).toBe(previousUrl)

    isDisplayCommitEnabled.value = false
    isRecovered = true
    await vi.advanceTimersByTimeAsync(800)
    expect(decodedUrls).toEqual([previousUrl, previewUrl, originalUrl, previewUrl])
    expect(preview.displayUrl.value).toBe(previousUrl)

    isDisplayCommitEnabled.value = true
    expect(preview.displayUrl.value).toBe(previewUrl)
    scope.stop()
  })

  it.each(['tier change', 'source change', 'dispose'])(
    'cancels a pending decode retry on %s',
    async (action) => {
      let originalAttempts = 0
      vi.stubGlobal(
        'Image',
        class {
          src = ''
          decode() {
            if (this.src.endsWith('/content')) {
              originalAttempts += 1
              return Promise.reject(new Error('Image temporarily unavailable'))
            }
            return Promise.resolve()
          }
        },
      )
      const { useDisplayImagePreview } = await importPreviewModule()
      const maxSide = ref<number | null>(256)
      const sourceUrl = ref('/uploads/image.png')
      const scope = effectScope()
      const preview = scope.run(() =>
        useDisplayImagePreview(sourceUrl, { maxSide, assetId: 'asset-42' }),
      )!
      await vi.waitFor(() => expect(preview.displayUrl.value).toContain('/derivatives/thumb'))
      vi.useFakeTimers()
      maxSide.value = null
      await flushPreviewTasks()
      expect(originalAttempts).toBe(1)

      if (action === 'dispose') scope.stop()
      else if (action === 'source change') sourceUrl.value = ''
      else maxSide.value = 640
      await vi.advanceTimersByTimeAsync(5000)
      expect(originalAttempts).toBe(1)
      if (action === 'tier change') {
        expect(preview.displayUrl.value).toContain('/derivatives/preview')
      }
      scope.stop()
    },
  )

  it('bounds retries when an image remains unavailable', async () => {
    const decode = vi.fn().mockRejectedValue(new Error('Image unavailable'))
    vi.stubGlobal(
      'Image',
      class {
        src = ''
        decode() {
          return decode()
        }
      },
    )
    const { useDisplayImagePreview } = await importPreviewModule()
    vi.useFakeTimers()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/uploads/image.png', { maxSide: null, assetId: 'asset-42' }),
    )!
    await vi.advanceTimersByTimeAsync(10_000)
    expect(decode).toHaveBeenCalledTimes(4)
    expect(preview.displayUrl.value).toBe('')
    expect(vi.getTimerCount()).toBe(0)
    scope.stop()
  })

  it('exposes the ready original as the 409 content fallback', async () => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: 640, assetId: 'asset-42' }),
    )
    if (!preview) throw new Error('preview composable was not created')

    expect(preview.contentFallbackUrl.value).toBe('')
    await vi.waitFor(() =>
      expect(preview.contentFallbackUrl.value).toBe(
        '/api/v1/workspaces/ws_test123/media-assets/asset-42/content',
      ),
    )
    scope.stop()
  })

  it('keeps the content fallback empty without a media asset', async () => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('/uploads/tasks/U123/board/content/image.png')
    })
    expect(preview.contentFallbackUrl.value).toBe('')
    scope.stop()
  })

  it('keeps the previous stage frame until a replacement source decodes', async () => {
    const decodeResolvers: Array<() => void> = []
    class DeferredImage {
      src = ''

      decode() {
        return new Promise<void>((resolve) => decodeResolvers.push(resolve))
      }
    }
    vi.stubGlobal('Image', DeferredImage)

    const sourceUrl = ref('/uploads/tasks/U123/board/content/first.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: null, shouldKeepPreviousOnSourceChange: true }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    decodeResolvers.shift()?.()
    await vi.waitFor(() => expect(preview.displayUrl.value).toContain('first.png'))

    sourceUrl.value = '/uploads/tasks/U123/board/content/second.png'
    await nextTick()
    expect(preview.displayUrl.value).toContain('first.png')

    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    decodeResolvers.shift()?.()
    await vi.waitFor(() => expect(preview.displayUrl.value).toContain('second.png'))
    scope.stop()
  })

  it('keeps the decoded frame when the stage leaves the viewport', async () => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const isVisible = ref(true)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: 640, assetId: 'asset-42', isVisible }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() =>
      expect(preview.displayUrl.value).toBe(
        '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview',
      ),
    )
    isVisible.value = false
    await nextTick()
    expect(preview.displayUrl.value).toBe(
      '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview',
    )
    scope.stop()
  })

  it('clears the previous image on a source change and ignores stale decode races', async () => {
    const decodeResolvers: Array<() => void> = []
    class DeferredImage {
      src = ''

      decode() {
        return new Promise<void>((resolve) => decodeResolvers.push(resolve))
      }
    }
    vi.stubGlobal('Image', DeferredImage)

    const sourceUrl = ref('/uploads/tasks/U123/board/content/first.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() => useDisplayImagePreview(sourceUrl, { maxSide: null }))
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    decodeResolvers.shift()?.()
    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toContain('/uploads/tasks/U123/board/content/first.png')
    })

    sourceUrl.value = '/uploads/tasks/U123/board/content/second.png'
    await nextTick()
    expect(preview.displayUrl.value).toBe('')
    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    const resolveSecond = decodeResolvers.shift()

    sourceUrl.value = '/uploads/tasks/U123/board/content/third.png'
    await nextTick()
    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    resolveSecond?.()
    await flushPreviewTasks()
    expect(preview.displayUrl.value).toBe('')

    decodeResolvers.shift()?.()
    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toContain('/uploads/tasks/U123/board/content/third.png')
    })
    scope.stop()
  })

  it('clears obsolete decode sources immediately and preserves the last-good frame', async () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const images: Array<{ src: string; finish: () => void }> = []
    class DeferredImage {
      src = ''
      finish: () => void = () => undefined

      decode() {
        images.push(this)
        return new Promise<void>((resolve) => {
          this.finish = resolve
        })
      }
    }
    vi.stubGlobal('Image', DeferredImage)
    const { useDisplayImagePreview } = await importPreviewModule()
    const source = ref('/uploads/first.png')
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(source, { maxSide: null, shouldKeepPreviousOnSourceChange: true }),
    )
    images[0]?.finish()
    await flushPreviewTasks()
    expect(preview?.displayUrl.value).toBe('/uploads/first.png')

    source.value = '/uploads/second.png'
    await nextTick()
    expect(images[1]?.src).toBe('/uploads/second.png')
    source.value = '/uploads/third.png'
    await nextTick()
    expect(images[1]?.src).toBe('')
    expect(preview?.displayUrl.value).toBe('/uploads/first.png')
    images[1]?.finish()
    await flushPreviewTasks()
    expect(preview?.displayUrl.value).toBe('/uploads/first.png')
    scope.stop()
    expect(images[2]?.src).toBe('')
    expect(window.__canvasInteractionBenchmarkCounters.displayImageFallbackDecodeReady).toBe(1)
    expect(window.__canvasInteractionBenchmarkCounters.displayImageFallbackDecodeCancelled).toBe(2)
    images[2]?.finish()
    await flushPreviewTasks()
    expect(preview?.displayUrl.value).toBe('/uploads/first.png')
  })

  test.each(
    ['replace', 'delete-reinstall', 'enable-later'].flatMap((reset) =>
      ['ready', 'failed', 'cancelled'].map((outcome) => ({ reset, outcome })),
    ),
  )('isolates deferred decode $outcome metrics after $reset', async ({ reset, outcome }) => {
    if (reset !== 'enable-later') window.__canvasInteractionBenchmarkCounters = {}
    const decode = createDeferred<void>()
    const decodeStarted = vi.fn()
    class DeferredImage {
      src = ''
      decode() {
        decodeStarted()
        return decode.promise
      }
    }
    vi.stubGlobal('Image', DeferredImage)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/uploads/deferred.png', { maxSide: null }),
    )
    expect(decodeStarted).toHaveBeenCalledTimes(1)
    if (reset === 'delete-reinstall') delete window.__canvasInteractionBenchmarkCounters
    const replacement = (window.__canvasInteractionBenchmarkCounters = {})

    if (outcome === 'cancelled') scope.stop()
    if (outcome === 'failed') decode.reject(new Error('decode failed'))
    else decode.resolve()
    await flushPreviewTasks()

    expect(replacement).not.toHaveProperty('runtimeTimingMs')
    for (const suffix of ['Ready', 'Failed', 'Cancelled']) {
      expect(replacement).not.toHaveProperty(`displayImageFallbackDecode${suffix}`)
    }
    expect(preview?.displayUrl.value).toBe(outcome === 'ready' ? '/uploads/deferred.png' : '')
    scope.stop()
  })

  test('isolates managed delivery decode completion after collection replacement', async () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const decode = createDeferred<void>()
    const decodeStarted = vi.fn()
    class DeferredImage {
      src = ''
      decode() {
        decodeStarted()
        return decode.promise
      }
    }
    vi.stubGlobal('Image', DeferredImage)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview('/uploads/managed.png', { maxSide: 640, assetId: 'asset-42' }),
    )
    await vi.waitFor(() => expect(decodeStarted).toHaveBeenCalledTimes(1))
    const replacement = (window.__canvasInteractionBenchmarkCounters = {})
    decode.resolve()
    await vi.waitFor(() => expect(preview?.displayUrl.value).toContain('/derivatives/preview'))
    expect(replacement).not.toHaveProperty('runtimeTimingMs')
    expect(replacement).not.toHaveProperty('displayImageManagedDecodeReady')
    scope.stop()
  })

  test.each(['replace', 'delete-reinstall', 'enable-later'])(
    'isolates deferred preview generation timing after %s',
    async (reset) => {
      if (reset !== 'enable-later') window.__canvasInteractionBenchmarkCounters = {}
      const encoding = createDeferred<Blob>()
      canvasMediaIOMocks.canvasToBlob.mockReturnValueOnce(encoding.promise)
      const { useDisplayImagePreview } = await importPreviewModule()
      const scope = effectScope()
      const preview = scope.run(() =>
        useDisplayImagePreview('/uploads/deferred-encode.png', { maxSide: 640 }),
      )
      await vi.waitFor(() => expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(1))
      if (reset === 'delete-reinstall') delete window.__canvasInteractionBenchmarkCounters
      window.__canvasInteractionBenchmarkCounters = {}
      encoding.resolve(new Blob(['preview'], { type: 'image/webp' }))
      await vi.waitFor(() => expect(preview?.displayUrl.value).toBe('blob:7'))
      expect(window.__canvasInteractionBenchmarkCounters.runtimeTimingMs).toBeUndefined()
      expect(
        window.__canvasInteractionBenchmarkCounters.gauges?.displayPreviewRetainedResources,
      ).toBe(1)
      scope.stop()
    },
  )

  it('falls back to the original managed upload when browser preview fails', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({ ok: false } as Response)
    const sourceUrl = ref('/uploads/tasks/U123/board/content/fallback.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() => useDisplayImagePreview(sourceUrl, { maxSide: 640 }))
    if (!preview) throw new Error('preview composable was not created')

    expect(preview.displayUrl.value).toBe('')
    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toContain('/uploads/tasks/U123/board/content/fallback.png')
    })
    scope.stop()
  })

  it('resolves upload refs before the first image render', async () => {
    const sourceUrl = ref('/uploads/tasks/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('/uploads/tasks/U123/board/content/image.png')
    })
    scope.stop()
  })

  it('resolves local upload refs through the configured API origin', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()
    const sourceUrl = ref('/uploads/tasks/U123/board/content/local.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('/uploads/tasks/U123/board/content/local.png')
    })
    scope.stop()
  })

  it('reports shared preview work and retained pixel estimates only when opted in', async () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const counters = window.__canvasInteractionBenchmarkCounters
    const { createDisplayPreviewResources } = await import('./displayImagePreviewResources')
    const first = createDisplayPreviewResources()
    const second = createDisplayPreviewResources()
    const firstLoad = first.load('https://example.com/measured.jpg', 640)
    const secondLoad = second.load('https://example.com/measured.jpg', 640)
    const url = await firstLoad
    expect(await secondLoad).toBe(url)
    await flushPreviewTasks()
    expect(counters.displayPreviewCacheMiss).toBe(1)
    expect(counters.displayPreviewGenerationJoin).toBe(1)
    expect(counters.gauges).toMatchObject({
      displayPreviewRetainedResources: 1,
      displayPreviewPinnedResources: 1,
      displayPreviewReusableResources: 0,
      displayPreviewEstimatedPixels: 640 * 320,
      displayPreviewEncodedBytes: 7,
      displayPreviewActiveTasks: 0,
      displayPreviewQueuedTasks: 0,
    })
    expect(canvasMediaIOMocks.canvasToBlob.mock.calls[0]?.[0]).toMatchObject({
      width: 0,
      height: 0,
    })
    first.display(url ?? '')
    await first.load('', null)
    second.dispose()
    expect(counters.gauges?.displayPreviewPinnedResources).toBe(1)
    first.dispose()
    expect(counters.gauges).toMatchObject({
      displayPreviewPinnedResources: 0,
      displayPreviewReusableResources: 1,
    })
    const cached = createDisplayPreviewResources()
    expect(await cached.load('https://example.com/measured.jpg', 640)).toBe(url)
    expect(counters.displayPreviewCacheHit).toBe(1)
    cached.dispose()
  })

  it('keeps live resources usable when cache capacity is exceeded', async () => {
    window.__canvasInteractionBenchmarkCounters = {}
    let blobCounter = 0
    vi.mocked(URL.createObjectURL).mockImplementation(() => `blob:owned-${(blobCounter += 1)}`)
    const { createDisplayPreviewResources } = await import('./displayImagePreviewResources')
    const consumers: Array<ReturnType<typeof createDisplayPreviewResources>> = []
    for (let index = 0; index < 85; index += 1) {
      const consumer = createDisplayPreviewResources()
      consumers.push(consumer)
      await expect(consumer.load(`https://example.com/owned-${index}.jpg`, 640)).resolves.toBe(
        `blob:owned-${index + 1}`,
      )
    }
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()
    for (const consumer of consumers) consumer.dispose()
    expect(window.__canvasInteractionBenchmarkCounters.displayPreviewCacheEviction).toBe(5)
    expect(window.__canvasInteractionBenchmarkCounters.gauges).toMatchObject({
      displayPreviewRetainedResources: 80,
      displayPreviewPinnedResources: 0,
      displayPreviewReusableResources: 80,
    })
    expect(URL.revokeObjectURL).toHaveBeenCalled()
  })

  it('does not publish a resource whose encoding finishes after disposal', async () => {
    let finishEncoding: ((blob: Blob) => void) | undefined
    canvasMediaIOMocks.canvasToBlob.mockImplementationOnce(
      () =>
        new Promise<Blob>((resolve) => {
          finishEncoding = resolve
        }),
    )
    const { createDisplayPreviewResources } = await import('./displayImagePreviewResources')
    const consumer = createDisplayPreviewResources()
    const result = consumer.load('https://example.com/late.jpg', 640)
    await vi.waitFor(() => expect(finishEncoding).toBeTypeOf('function'))
    consumer.dispose()
    finishEncoding?.(new Blob(['late'], { type: 'image/webp' }))
    await expect(result).resolves.toBeNull()
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })

  async function fillPreviewCache() {
    const { useDisplayImagePreview } = await importPreviewModule()
    for (let index = 0; index < 85; index += 1) {
      const scope = effectScope()
      const preview = scope.run(() =>
        useDisplayImagePreview(`https://example.com/filler-${index}.jpg`, { maxSide: 640 }),
      )
      await vi.waitFor(() => expect(preview?.displayUrl.value).toMatch(/^blob:preview-\d+$/), {
        interval: 1,
      })
      scope.stop()
    }
  }

  it('pins an outgoing frame through cache pressure until its transition releases it', async () => {
    let previewNumber = 0
    vi.mocked(URL.createObjectURL).mockImplementation(() => `blob:preview-${++previewNumber}`)
    const { createDisplayPreviewResources } = await import('./displayImagePreviewResources')
    const consumer = createDisplayPreviewResources()
    const outgoing = await consumer.load('/uploads/transition.jpg', 256)
    if (!outgoing) throw new Error('outgoing preview was not generated')
    consumer.display(outgoing)
    const release = consumer.retain(outgoing)
    const next = await consumer.load('/uploads/transition.jpg', 640)
    consumer.display(next ?? '')
    await fillPreviewCache()
    expect(URL.revokeObjectURL).not.toHaveBeenCalledWith(outgoing)
    release()
    release()
    await fillPreviewCache()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(outgoing)
    consumer.dispose()
  })

  it('keeps shared generation alive when its first consumer is disposed', async () => {
    let finishFetch: ((response: Response) => void) | undefined
    vi.mocked(fetch).mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finishFetch = resolve
        }),
    )
    const { useDisplayImagePreview } = await importPreviewModule()
    const firstScope = effectScope()
    firstScope.run(() => useDisplayImagePreview('/uploads/shared.jpg', { maxSide: 640 }))
    const secondScope = effectScope()
    const second = secondScope.run(() =>
      useDisplayImagePreview('/uploads/shared.jpg', { maxSide: 640 }),
    )

    expect(fetch).toHaveBeenCalledTimes(1)
    const signal = vi.mocked(fetch).mock.calls[0]?.[1]?.signal
    firstScope.stop()
    expect(signal?.aborted).toBe(false)
    finishFetch?.(new Response(new Blob(['image'], { type: 'image/jpeg' })))
    await vi.waitFor(() => expect(second?.displayUrl.value).toBe('blob:7'))
    expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(1)
    secondScope.stop()
  })

  it('aborts shared generation only after its last consumer leaves', async () => {
    vi.mocked(fetch).mockImplementationOnce(
      (_input, options) =>
        new Promise<Response>((_resolve, reject) => {
          options?.signal?.addEventListener('abort', () =>
            reject(new DOMException('Aborted', 'AbortError')),
          )
        }),
    )
    const { useDisplayImagePreview } = await importPreviewModule()
    const firstScope = effectScope()
    firstScope.run(() => useDisplayImagePreview('/uploads/shared.jpg', { maxSide: 640 }))
    const secondScope = effectScope()
    secondScope.run(() => useDisplayImagePreview('/uploads/shared.jpg', { maxSide: 640 }))
    const signal = vi.mocked(fetch).mock.calls[0]?.[1]?.signal

    firstScope.stop()
    expect(signal?.aborted).toBe(false)
    secondScope.stop()
    expect(signal?.aborted).toBe(true)
    await flushPreviewTasks()
    expect(canvasMediaIOMocks.canvasToBlob).not.toHaveBeenCalled()

    const retryScope = effectScope()
    const retry = retryScope.run(() =>
      useDisplayImagePreview('/uploads/shared.jpg', { maxSide: 640 }),
    )
    await vi.waitFor(() => expect(retry?.displayUrl.value).toBe('blob:7'))
    retryScope.stop()
  })

  it('removes an unused queued preview without starting its fetch', async () => {
    const waitForAbort = (_input: RequestInfo | URL, options?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        options?.signal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError')),
        )
      })
    vi.mocked(fetch).mockImplementationOnce(waitForAbort).mockImplementationOnce(waitForAbort)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scopes = [effectScope(), effectScope(), effectScope()]
    for (const [index, scope] of scopes.entries()) {
      scope.run(() => useDisplayImagePreview(`/uploads/queued-${index}.jpg`, { maxSide: 640 }))
    }
    expect(fetch).toHaveBeenCalledTimes(2)
    scopes[2]?.stop()
    scopes[0]?.stop()
    scopes[1]?.stop()
    await flushPreviewTasks()
    expect(fetch).toHaveBeenCalledTimes(2)

    const retryScope = effectScope()
    const retry = retryScope.run(() =>
      useDisplayImagePreview('/uploads/queued-2.jpg', { maxSide: 640 }),
    )
    await vi.waitFor(() => expect(retry?.displayUrl.value).toBe('blob:7'))
    expect(fetch).toHaveBeenCalledTimes(3)
    retryScope.stop()
  })

  it('retains a shared displayed URL until its last consumer is disposed', async () => {
    let blobCounter = 0
    vi.mocked(URL.createObjectURL).mockImplementation(() => `blob:preview-${(blobCounter += 1)}`)
    const { useDisplayImagePreview } = await importPreviewModule()
    const firstScope = effectScope()
    const first = firstScope.run(() =>
      useDisplayImagePreview('https://example.com/hero.jpg', { maxSide: 640 }),
    )
    const secondScope = effectScope()
    const second = secondScope.run(() =>
      useDisplayImagePreview('https://example.com/hero.jpg', { maxSide: 640 }),
    )
    await vi.waitFor(() => expect(first?.displayUrl.value).toMatch(/^blob:preview-\d+$/))
    const heroUrl = first?.displayUrl.value
    expect(second?.displayUrl.value).toBe(heroUrl)
    firstScope.stop()

    await fillPreviewCache()
    expect(URL.revokeObjectURL).toHaveBeenCalled()
    expect(vi.mocked(URL.revokeObjectURL).mock.calls.map(([url]) => url)).not.toContain(heroUrl)
    expect(second?.displayUrl.value).toBe(heroUrl)

    secondScope.stop()
    const extraScope = effectScope()
    const extra = extraScope.run(() =>
      useDisplayImagePreview('https://example.com/extra.jpg', { maxSide: 640 }),
    )
    await vi.waitFor(() => {
      expect(extra?.displayUrl.value).toMatch(/^blob:preview-\d+$/)
      expect(vi.mocked(URL.revokeObjectURL).mock.calls.map(([url]) => url)).toContain(heroUrl)
    })
    extraScope.stop()
  })

  it('retains the previous frame while its replacement is decoding under cache pressure', async () => {
    let blobCounter = 0
    vi.mocked(URL.createObjectURL).mockImplementation(() => `blob:preview-${(blobCounter += 1)}`)
    let blockedUrl = ''
    let finishDecode: (() => void) | undefined
    class DeferredImage {
      src = ''

      decode() {
        return this.src === blockedUrl
          ? new Promise<void>((resolve) => {
              finishDecode = resolve
            })
          : Promise.resolve()
      }
    }
    vi.stubGlobal('Image', DeferredImage)
    const { useDisplayImagePreview } = await importPreviewModule()
    const source = ref('/uploads/hero.jpg')
    const heroScope = effectScope()
    const hero = heroScope.run(() =>
      useDisplayImagePreview(source, { maxSide: 640, shouldKeepPreviousOnSourceChange: true }),
    )
    await vi.waitFor(() => expect(hero?.displayUrl.value).toBe('blob:preview-1'))

    blockedUrl = 'blob:preview-2'
    source.value = '/uploads/replacement.jpg'
    await vi.waitFor(() => expect(finishDecode).toBeTypeOf('function'))
    await fillPreviewCache()
    const revoked = vi.mocked(URL.revokeObjectURL).mock.calls.map(([url]) => url)
    expect(revoked).not.toContain('blob:preview-1')
    expect(revoked).not.toContain('blob:preview-2')
    expect(hero?.displayUrl.value).toBe('blob:preview-1')

    finishDecode?.()
    await vi.waitFor(() => expect(hero?.displayUrl.value).toBe('blob:preview-2'))
    heroScope.stop()
  })
})
