import { nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageEditorLayer } from '../types'
import { useImageAdjustmentFilterPreviews } from './useImageAdjustmentFilterPreviews'

const fetchMock = vi.fn()
const createImageBitmapMock = vi.fn()

function createHslLayer(params = { hue: 0, saturation: 0, brightness: 0, contrast: 0 }) {
  return {
    id: 'adjustment:hsl',
    type: 'adjustment',
    kind: 'hsl',
    name: '色相/饱和度 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createThresholdLayer(params = { threshold: 128, invert: false }) {
  return {
    id: 'adjustment:threshold',
    type: 'adjustment',
    kind: 'threshold',
    name: '阈值 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createCurvesLayer(params = { input: 50, output: 50 }) {
  return {
    id: 'adjustment:curves',
    type: 'adjustment',
    kind: 'curves',
    name: '曲线 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createLevelsLayer(params = { shadows: 0, midtones: 50, highlights: 100 }) {
  return {
    id: 'adjustment:levels',
    type: 'adjustment',
    kind: 'levels',
    name: '色阶 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createInvertLayer(params = { enabled: true }) {
  return {
    id: 'adjustment:invert',
    type: 'adjustment',
    kind: 'invert',
    name: '反相 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

async function flushPreviewLoad() {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  fetchMock.mockReset()
  createImageBitmapMock.mockReset()
})

describe('useImageAdjustmentFilterPreviews', () => {
  it('renders previews through the same adjustment engine used by final apply', async () => {
    installBrowserImageMocks([10, 20, 30, 255])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([createInvertLayer()]),
    })

    await flushPreviewLoad()

    expect(preview.imageAdjustmentPlan.value).toEqual([expect.objectContaining({ kind: 'channel-lut' })])
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([
      {
        type: 'rendered-image',
        imageData: expect.objectContaining({
          data: new Uint8ClampedArray([245, 235, 225, 255]),
          width: 1,
          height: 1,
        }),
        width: 1,
        height: 1,
      },
    ])
  })

  it('does not load or render hidden, inactive, or neutral adjustment stacks', async () => {
    installBrowserImageMocks([10, 20, 30, 255])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([
        { ...createHslLayer(), visible: false },
        createCurvesLayer(),
        createLevelsLayer(),
        createInvertLayer({ enabled: false }),
      ]),
    })

    await flushPreviewLoad()

    expect(preview.imageAdjustmentPlan.value).toEqual([])
    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([])
  })

  it('normalizes non-finite params before rendering previews', async () => {
    installBrowserImageMocks([120, 120, 120, 255])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([
        createThresholdLayer({ threshold: Number.NaN, invert: false }),
        createHslLayer({
          hue: Number.NaN,
          saturation: Number.POSITIVE_INFINITY,
          brightness: Number.NEGATIVE_INFINITY,
          contrast: Number.NaN,
        }),
      ]),
    })

    await flushPreviewLoad()

    expect(preview.imageAdjustmentPlan.value).toEqual([
      { kind: 'threshold', threshold: 128, invert: false },
    ])
    expect(preview.imageAdjustmentFilterPreviews.value[0]).toMatchObject({
      type: 'rendered-image',
      width: 1,
      height: 1,
    })
  })

  it('coalesces rapid layer changes to the latest animation frame', async () => {
    const raf = installBrowserImageMocks([120, 120, 120, 255], { autoFlushRaf: false })
    const layers = ref<ImageEditorLayer[]>([createThresholdLayer({ threshold: 32, invert: false })])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers,
    })

    await flushPreviewLoad()
    layers.value = [createThresholdLayer({ threshold: 224, invert: true })]
    await nextTick()
    layers.value = [createThresholdLayer({ threshold: 64, invert: false })]
    await nextTick()

    raf.flush()

    expect(preview.imageAdjustmentPlan.value).toEqual([
      { kind: 'threshold', threshold: 64, invert: false },
    ])
    expect(preview.imageAdjustmentFilterPreviews.value[0]).toMatchObject({
      type: 'rendered-image',
      width: 1,
      height: 1,
    })
  })
})

function installBrowserImageMocks(
  sourceValues: number[],
  options: { autoFlushRaf?: boolean } = {},
) {
  const autoFlushRaf = options.autoFlushRaf ?? true
  const rafCallbacks = new Map<number, FrameRequestCallback>()
  let rafId = 0

  class TestImageDataImpl {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }

  const sourceImageData = new TestImageDataImpl(
    new Uint8ClampedArray(sourceValues),
    sourceValues.length / 4,
    1,
  ) as ImageData

  vi.stubGlobal('ImageData', TestImageDataImpl)
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('createImageBitmap', createImageBitmapMock)
  fetchMock.mockResolvedValue({
    ok: true,
    blob: async () => new Blob(['source'], { type: 'image/png' }),
  })
  createImageBitmapMock.mockResolvedValue({
    width: sourceImageData.width,
    height: sourceImageData.height,
    close: vi.fn(),
  })

  vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') {
      return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
    }

    const canvas = document.createElementNS(
      'http://www.w3.org/1999/xhtml',
      'canvas',
    ) as HTMLCanvasElement
    const context = {
      drawImage: vi.fn(),
      getImageData: vi.fn(() => sourceImageData),
      putImageData: vi.fn(),
    }
    Object.defineProperty(canvas, 'getContext', {
      value: () => context,
    })
    return canvas
  })

  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    rafId += 1
    rafCallbacks.set(rafId, callback)
    if (autoFlushRaf) {
      callback(performance.now())
      rafCallbacks.delete(rafId)
    }
    return rafId
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id) => {
    rafCallbacks.delete(id)
  })

  return {
    flush() {
      const callbacks = Array.from(rafCallbacks.values())
      rafCallbacks.clear()
      for (const callback of callbacks) {
        callback(performance.now())
      }
    },
  }
}
