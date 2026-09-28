import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'

const canvasMediaIOMocks = vi.hoisted(() => ({
  canvasToBlob: vi.fn(),
  resolveFetchUrl: vi.fn((url: string) => url),
}))

vi.mock('../../infrastructure/media/canvasMediaIo.ts', () => ({
  canvasToBlob: canvasMediaIOMocks.canvasToBlob,
  resolveFetchUrl: canvasMediaIOMocks.resolveFetchUrl,
}))

vi.mock('../../../../shared/api/workspaceEventStream', () => ({
  subscribeWorkspaceMediaInvalidation: vi.fn(() => vi.fn()),
}))

// Workspace selection for media delivery resolves through the IoC seam
// (`setMediaWorkspaceIdResolver`), not the workspaceContext module.
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../shared/api/mediaWorkspaceScope'

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
    import('../../../../shared/api/mediaWorkspaceScope'),
  ])
  setMediaWorkspaceIdResolver(() => 'ws_test123')
  return { useDisplayImagePreview }
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
            JSON.stringify({
              data: {
                id: 'asset-42',
                workspace_id: 'ws_test123',
                status: 'ready',
                media_kind: 'image',
                mime_type: 'image/jpeg',
                width: 2048,
                height: 1024,
                size_bytes: 100,
                checksum_sha256:
                  'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
                content_url: '/api/v1/workspaces/ws_test123/media-assets/asset-42/content',
                derivatives: {
                  thumb: {
                    status: 'ready',
                    format: 'webp',
                    url: '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/thumb',
                  },
                  preview: {
                    status: 'ready',
                    format: 'webp',
                    url: '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview',
                  },
                  'large-preview': {
                    status: 'ready',
                    format: 'webp',
                    url: '/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/large-preview',
                  },
                },
                created_at: '2026-09-25T00:00:00.000Z',
                updated_at: '2026-09-25T00:00:00.000Z',
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
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
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
      '/uploads/canvas/U123/board/content/image.png',
      expect.objectContaining({ credentials: 'include', signal: expect.any(AbortSignal) }),
    )
    expect(createImageBitmap).toHaveBeenCalled()
    expect(canvasMediaIOMocks.canvasToBlob).toHaveBeenCalled()
    scope.stop()
  })

  it('uses the canonical media derivative and never falls back to the original asset', async () => {
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: 640, assetId: 'asset-42' }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(preview.displayUrl.value).toBe('/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview'))
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/workspaces/ws_test123/media-assets/asset-42',
      expect.objectContaining({ credentials: 'include', signal: expect.any(AbortSignal) }),
    )
    scope.stop()
  })

  it('keeps the current media derivative visible while a zoom tier refreshes', async () => {
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
    const maxSide = ref(256)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide, assetId: 'asset-42' }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(preview.displayUrl.value).toBe('/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/thumb'))

    maxSide.value = 640
    await nextTick()
    expect(preview.displayUrl.value).toBe('/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/thumb')

    await vi.waitFor(() => expect(preview.displayUrl.value).toBe('/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview'))
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

    const sourceUrl = ref('/uploads/canvas/U123/board/content/first.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: null, keepPreviousOnSourceChange: true }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    decodeResolvers.shift()?.()
    await vi.waitFor(() => expect(preview.displayUrl.value).toContain('first.png'))

    sourceUrl.value = '/uploads/canvas/U123/board/content/second.png'
    await nextTick()
    expect(preview.displayUrl.value).toContain('first.png')

    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    decodeResolvers.shift()?.()
    await vi.waitFor(() => expect(preview.displayUrl.value).toContain('second.png'))
    scope.stop()
  })

  it('keeps the decoded frame when the stage leaves the viewport', async () => {
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
    const isVisible = ref(true)
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, { maxSide: 640, assetId: 'asset-42', isVisible }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(preview.displayUrl.value).toBe('/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview'))
    isVisible.value = false
    await nextTick()
    expect(preview.displayUrl.value).toBe('/api/v1/workspaces/ws_test123/media-assets/asset-42/derivatives/preview')
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

    const sourceUrl = ref('/uploads/canvas/U123/board/content/first.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() => useDisplayImagePreview(sourceUrl, { maxSide: null }))
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    decodeResolvers.shift()?.()
    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toContain('/uploads/canvas/U123/board/content/first.png')
    })

    sourceUrl.value = '/uploads/canvas/U123/board/content/second.png'
    await nextTick()
    expect(preview.displayUrl.value).toBe('')
    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    const resolveSecond = decodeResolvers.shift()

    sourceUrl.value = '/uploads/canvas/U123/board/content/third.png'
    await nextTick()
    await vi.waitFor(() => expect(decodeResolvers).toHaveLength(1))
    resolveSecond?.()
    await flushPreviewTasks()
    expect(preview.displayUrl.value).toBe('')

    decodeResolvers.shift()?.()
    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toContain('/uploads/canvas/U123/board/content/third.png')
    })
    scope.stop()
  })

  it('falls back to the original managed upload when browser preview fails', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({ ok: false } as Response)
    const sourceUrl = ref('/uploads/canvas/U123/board/content/fallback.png')
    const { useDisplayImagePreview } = await importPreviewModule()
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
    const sourceUrl = ref('/uploads/canvas/U123/board/content/image.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('/uploads/canvas/U123/board/content/image.png')
    })
    scope.stop()
  })

  it('resolves local upload refs through the configured API origin', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()
    const sourceUrl = ref('/uploads/canvas/U123/board/content/local.png')
    const { useDisplayImagePreview } = await importPreviewModule()
    const scope = effectScope()
    const preview = scope.run(() =>
      useDisplayImagePreview(sourceUrl, {
        maxSide: null,
      }),
    )
    if (!preview) throw new Error('preview composable was not created')

    await vi.waitFor(() => {
      expect(preview.displayUrl.value).toBe('/uploads/canvas/U123/board/content/local.png')
    })
    scope.stop()
  })

  it('never revokes the preview URL backing a displayed image during cache eviction', async () => {
    let blobCounter = 0
    vi.mocked(URL.createObjectURL).mockImplementation(() => `blob:preview-${(blobCounter += 1)}`)
    const { useDisplayImagePreview } = await importPreviewModule()

    const heroSource = ref('https://example.com/hero.jpg')
    const heroScope = effectScope()
    const hero = heroScope.run(() => useDisplayImagePreview(heroSource, { maxSide: 640 }))
    if (!hero) throw new Error('preview composable was not created')
    await vi.waitFor(() => expect(hero.displayUrl.value).toMatch(/^blob:preview-\d+$/))
    const heroUrl = hero.displayUrl.value

    const fillerScopes: Array<ReturnType<typeof effectScope>> = []
    for (let index = 0; index < 85; index += 1) {
      const fillerScope = effectScope()
      const fillerSource = ref(`https://example.com/filler-${index}.jpg`)
      const filler = fillerScope.run(() =>
        useDisplayImagePreview(fillerSource, { maxSide: 640 }),
      )
      if (!filler) throw new Error('preview composable was not created')
      fillerScopes.push(fillerScope)
    }
    // 86 cached previews against a limit of 80 forces eviction.
    await vi.waitFor(() => {
      expect(vi.mocked(URL.revokeObjectURL).mock.calls.length).toBeGreaterThanOrEqual(6)
    })
    const revokedWhileDisplayed = vi
      .mocked(URL.revokeObjectURL)
      .mock.calls.map(([url]) => url)
    expect(revokedWhileDisplayed).not.toContain(heroUrl)

    // Once the image is no longer displayed, its URL becomes evictable again.
    heroScope.stop()
    const extraScope = effectScope()
    const extraSource = ref('https://example.com/filler-extra.jpg')
    const extra = extraScope.run(() => useDisplayImagePreview(extraSource, { maxSide: 640 }))
    if (!extra) throw new Error('preview composable was not created')
    await vi.waitFor(() => {
      expect(
        vi.mocked(URL.revokeObjectURL).mock.calls.map(([url]) => url),
      ).toContain(heroUrl)
    })
    for (const fillerScope of fillerScopes) fillerScope.stop()
    extraScope.stop()
  })
})
