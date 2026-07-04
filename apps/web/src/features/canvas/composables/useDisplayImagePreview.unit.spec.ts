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
      vi.fn(async () => ({
        ok: true,
        blob: async () => new Blob(['image'], { type: 'image/jpeg' }),
      })),
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
})
