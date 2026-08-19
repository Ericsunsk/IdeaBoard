import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'

const flowMediaIOMocks = vi.hoisted(() => ({
  canvasToBlob: vi.fn(),
  resolveFetchUrl: vi.fn((url: string) => url),
}))

vi.mock('../services/flowMediaIO', () => ({
  canvasToBlob: flowMediaIOMocks.canvasToBlob,
  resolveFetchUrl: flowMediaIOMocks.resolveFetchUrl,
}))

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
    vi.resetModules()
    const createElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      if (tagName === 'canvas') return createCanvasMock() as unknown as HTMLCanvasElement
      return createElement(tagName)
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = typeof input === 'string' ? input : input.toString()
        if (url.includes('/user/media-assets/42')) {
          return new Response(
            JSON.stringify({
              code: 0,
              msg: '',
              data: {
                asset_id: 'asset-42',
                status: 'ready',
                mime_type: 'image/jpeg',
                width: 2048,
                height: 1024,
                size_bytes: 100,
                original: { status: 'ready', url: '/api/user/media-assets/42/original/content' },
                derivatives: {
                  thumb: { status: 'ready', format: 'webp', url: '/api/thumb.webp' },
                  preview: { status: 'ready', format: 'webp', url: '/api/preview.webp' },
                  'large-preview': { status: 'ready', format: 'webp', url: '/api/large.webp' },
                },
              },
            }),
            { headers: { 'content-type': 'application/json' } },
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
    flowMediaIOMocks.canvasToBlob.mockResolvedValue(new Blob(['preview'], { type: 'image/webp' }))
    flowMediaIOMocks.resolveFetchUrl.mockImplementation((url: string) => url)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('regenerates preview only when the resolved max side tier changes', async () => {
    const sourceUrl = ref('https://example.com/image.jpg')
    const maxSide = ref<number | null>(320)
    const { useDisplayImagePreview } = await import('./useDisplayImagePreview')
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

    expect(flowMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(1)

    maxSide.value = 320
    await nextTick()
    await waitForPreviewWork()
    expect(flowMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(1)

    maxSide.value = 640
    await nextTick()
    await vi.waitFor(() => {
      expect(flowMediaIOMocks.canvasToBlob).toHaveBeenCalledTimes(2)
    })
    scope.stop()
    await waitForPreviewWork()
  })

  it('uses the source URL directly when preview max side is null', async () => {
    flowMediaIOMocks.canvasToBlob.mockClear()
    vi.mocked(fetch).mockClear()
    const sourceUrl = ref('https://example.com/full.jpg')
    const { useDisplayImagePreview } = await import('./useDisplayImagePreview')
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
    expect(flowMediaIOMocks.canvasToBlob).not.toHaveBeenCalled()
    scope.stop()
  })

  it('resizes managed uploads in the browser when no media asset id is available', async () => {
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
    const { useDisplayImagePreview } = await import('./useDisplayImagePreview')
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
      '/uploads/canvas/U123/board/content/image.png',
      { credentials: 'include' },
    )
    expect(createImageBitmap).toHaveBeenCalled()
    expect(flowMediaIOMocks.canvasToBlob).toHaveBeenCalled()
    scope.stop()
  })

  it('uses the canonical media derivative and never falls back to the original asset', async () => {
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
    const { useDisplayImagePreview } = await import('./useDisplayImagePreview')
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: 640, mediaAssetId: 42 }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(preview.displayUrl.value).toBe('/api/preview.webp'))
    expect(fetch).toHaveBeenCalledWith(
      '/api/user/media-assets/42',
      expect.objectContaining({ credentials: 'include', signal: expect.any(AbortSignal) }),
    )
    scope.stop()
  })

  it('falls back to the original managed upload when browser preview fails', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({ ok: false } as Response)
    const sourceUrl = ref('/uploads/canvas/U123/board/content/fallback.png')
    const { useDisplayImagePreview } = await import('./useDisplayImagePreview')
    const scope = effectScope()
    const preview = scope.run(() => useDisplayImagePreview(sourceUrl, { maxSide: 640 }))
    if (!preview) throw new Error('preview composable was not created')

    expect(preview.displayUrl.value).toBe('')
    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toContain('/uploads/canvas/U123/board/content/fallback.png')
    })
    scope.stop()
  })

  it('resolves upload refs before the first image render', async () => {
    vi.stubEnv('VITE_PUBLIC_ASSET_BASE_URL', 'https://assets.example.com')
    vi.resetModules()
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
    const { useDisplayImagePreview } = await import('./useDisplayImagePreview')
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    expect(preview.displayUrl.value).toBe(
      'https://assets.example.com/uploads/canvas/U123/board/content/image.png',
    )
    scope.stop()
  })

  it('resolves local upload refs through the configured API origin', async () => {
    vi.stubEnv('VITE_PUBLIC_ASSET_BASE_URL', '')
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()
    const sourceUrl = ref('/uploads/canvas/U123/board/content/local.png')
    const { useDisplayImagePreview } = await import('./useDisplayImagePreview')
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    expect(preview.displayUrl.value).toBe(
      'https://api.example.com/uploads/canvas/U123/board/content/local.png',
    )
    scope.stop()
  })
})
