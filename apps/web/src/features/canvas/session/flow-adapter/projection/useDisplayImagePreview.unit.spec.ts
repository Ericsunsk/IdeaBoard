import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

describe('useDisplayImagePreview', () => {
  beforeEach(() => {
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

  it('keeps live resources usable when cache capacity is exceeded', async () => {
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
