import { nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageProcessingRuntime } from '../runtime/types'
import type { ImageEditorLayer } from '../types'
import { buildImageAdjustmentPlan } from '../utils/imageAdjustmentPlanPipeline'
import { useImageAdjustmentRenderedPreview } from './useImageAdjustmentRenderedPreview'

const { getImageAdjustmentProcessingRuntimeMock } = vi.hoisted(() => ({
  getImageAdjustmentProcessingRuntimeMock: vi.fn(),
}))

vi.mock('../runtime/imageAdjustmentProcessingRuntime', () => ({
  getImageAdjustmentProcessingRuntime: getImageAdjustmentProcessingRuntimeMock,
}))

function createHslLayer(params = { hue: 0, saturation: 0, lightness: 0 }) {
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

function createCurvesLayer(params = { channels: {} }) {
  return {
    id: 'adjustment:curves',
    type: 'adjustment',
    kind: 'curves',
    name: '曲线 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createLevelsLayer(
  params = { shadows: 0, midtones: 128, highlights: 255, outputBlack: 0, outputWhite: 255 },
) {
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
  getImageAdjustmentProcessingRuntimeMock.mockReset()
})

describe('useImageAdjustmentRenderedPreview', () => {
  it('routes inline preview rendering through the shared image processing runtime', async () => {
    const runtime = installPreviewRuntimeMock({
      sourceValues: [10, 20, 30, 255],
      previewValues: [245, 235, 225, 255],
    })
    const preview = useImageAdjustmentRenderedPreview({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([createInvertLayer()]),
    })

    await flushPreviewLoad()

    expect(runtime.renderPreview).toHaveBeenCalledWith(
      expect.objectContaining({
        source: { url: 'https://example.com/source.png' },
        plan: buildImageAdjustmentPlan([createInvertLayer()]),
        taskId: 'image-adjustment:inline-preview',
        latestOnly: true,
        sizeStrategy: { maxSide: 960 },
      }),
    )
    expect(preview.imageEditorRenderedPreview.value).toEqual({
      type: 'rendered-image',
      imageData: expect.objectContaining({
        data: new Uint8ClampedArray([245, 235, 225, 255]),
        width: 1,
        height: 1,
      }),
      width: 1,
      height: 1,
    })
  })

  it('exposes the decoded preview source for histogram consumers', async () => {
    installPreviewRuntimeMock({
      sourceValues: [10, 20, 30, 255],
      previewValues: [245, 235, 225, 255],
    })
    const preview = useImageAdjustmentRenderedPreview({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref([createInvertLayer()]),
    })

    await flushPreviewLoad()

    expect(preview.imageEditorPreviewSource.value).toMatchObject({
      type: 'rendered-image',
      imageData: expect.objectContaining({
        data: new Uint8ClampedArray([10, 20, 30, 255]),
      }),
      width: 1,
      height: 1,
    })
  })

  it('passes the compiled layered adjustment plan to preview rendering', async () => {
    const sourceValues = [16, 48, 112, 255, 80, 140, 220, 240, 196, 88, 24, 192, 240, 240, 96, 128]
    const runtime = installPreviewRuntimeMock({
      sourceValues,
      previewValues: [255, 255, 255, 255, 0, 0, 0, 255, 255, 255, 255, 255, 0, 0, 0, 255],
    })
    const layers = [
      createHslLayer({ hue: 35, saturation: 22, lightness: 8 }),
      createCurvesLayer({
        channels: {
          rgb: [{ id: 'point:midtone', input: 42, output: 70 }],
        },
      }),
      createLevelsLayer({
        shadows: 20,
        midtones: 112,
        highlights: 235,
        outputBlack: 0,
        outputWhite: 255,
      }),
      createThresholdLayer({ threshold: 150, invert: true }),
      createInvertLayer(),
    ] satisfies ImageEditorLayer[]
    const finalPlan = buildImageAdjustmentPlan(layers)
    const preview = useImageAdjustmentRenderedPreview({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref(layers),
    })

    await flushPreviewLoad()

    expect(runtime.renderPreview).toHaveBeenCalledWith(
      expect.objectContaining({
        plan: finalPlan,
      }),
    )
    expect(preview.imageEditorRenderedPreview.value).toMatchObject({
      type: 'rendered-image',
      width: sourceValues.length / 4,
      height: 1,
    })
  })

  it('loads the preview source but does not expose rendered output for neutral adjustment stacks', async () => {
    const runtime = installPreviewRuntimeMock({
      sourceValues: [10, 20, 30, 255],
      previewValues: [10, 20, 30, 255],
    })
    const preview = useImageAdjustmentRenderedPreview({
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

    expect(runtime.renderPreview).toHaveBeenCalledWith(
      expect.objectContaining({
        plan: [],
      }),
    )
    expect(preview.imageEditorRenderedPreview.value).toBeNull()
    expect(preview.imageEditorPreviewSource.value).toMatchObject({
      type: 'rendered-image',
      imageData: expect.objectContaining({
        data: new Uint8ClampedArray([10, 20, 30, 255]),
      }),
    })
  })

  it('normalizes non-finite params before rendering previews', async () => {
    const runtime = installPreviewRuntimeMock({
      sourceValues: [120, 120, 120, 255],
      previewValues: [0, 0, 0, 255],
    })
    const layers = [
      createThresholdLayer({ threshold: Number.NaN, invert: false }),
      createHslLayer({
        hue: Number.NaN,
        saturation: Number.POSITIVE_INFINITY,
        lightness: Number.NEGATIVE_INFINITY,
      }),
    ] satisfies ImageEditorLayer[]
    const preview = useImageAdjustmentRenderedPreview({
      isAdjustMode: ref(true),
      sourceUrl: ref('https://example.com/source.png'),
      layers: ref(layers),
    })

    await flushPreviewLoad()

    expect(runtime.renderPreview).toHaveBeenCalledWith(
      expect.objectContaining({
        plan: buildImageAdjustmentPlan(layers),
      }),
    )
    expect(preview.imageEditorRenderedPreview.value).toMatchObject({
      type: 'rendered-image',
      imageData: expect.objectContaining({
        data: new Uint8ClampedArray([0, 0, 0, 255]),
      }),
      width: 1,
      height: 1,
    })
  })

  it('coalesces rapid layer changes to the latest animation frame', async () => {
    const raf = installAnimationFrameMock({ autoFlushRaf: false })
    const runtime = installPreviewRuntimeMock({
      sourceValues: [120, 120, 120, 255],
      previewValues: [255, 255, 255, 255],
      installRaf: false,
    })
    const layers = ref<ImageEditorLayer[]>([createThresholdLayer({ threshold: 32, invert: false })])
    const preview = useImageAdjustmentRenderedPreview({
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
    await flushPreviewLoad()

    expect(runtime.renderPreview).toHaveBeenCalledTimes(1)
    expect(runtime.renderPreview).toHaveBeenCalledWith(
      expect.objectContaining({
        plan: buildImageAdjustmentPlan([createThresholdLayer({ threshold: 64, invert: false })]),
      }),
    )
    expect(preview.imageEditorRenderedPreview.value).toMatchObject({
      type: 'rendered-image',
      imageData: expect.objectContaining({
        data: new Uint8ClampedArray([255, 255, 255, 255]),
      }),
      width: 1,
      height: 1,
    })
  })
})

function installPreviewRuntimeMock(input: {
  sourceValues: number[]
  previewValues: number[]
  installRaf?: boolean
}) {
  installImageDataMock()
  if (input.installRaf ?? true) installAnimationFrameMock()
  const sourceImageData = makeImageData(input.sourceValues)
  const previewImageData = makeImageData(input.previewValues)
  const renderPreview = vi.fn<ImageProcessingRuntime['renderPreview']>(async () => ({
    status: 'completed',
    taskId: 'image-adjustment:inline-preview',
    revision: 1,
    output: {
      sourceImageData,
      previewImageData,
      width: sourceImageData.width,
      height: sourceImageData.height,
    },
  }))
  const runtime: ImageProcessingRuntime = {
    renderFinal: vi.fn<ImageProcessingRuntime['renderFinal']>(),
    renderPreview,
    cancel: vi.fn<ImageProcessingRuntime['cancel']>(),
    dispose: vi.fn<ImageProcessingRuntime['dispose']>(),
  }
  getImageAdjustmentProcessingRuntimeMock.mockReturnValue(runtime)
  return runtime
}

function installAnimationFrameMock(options: { autoFlushRaf?: boolean } = {}) {
  const autoFlushRaf = options.autoFlushRaf ?? true
  const rafCallbacks = new Map<number, FrameRequestCallback>()
  let rafId = 0

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

function installImageDataMock() {
  vi.stubGlobal('ImageData', TestImageDataImpl)
}

function makeImageData(values: number[]): ImageData {
  installImageDataMock()
  return new ImageData(new Uint8ClampedArray(values), values.length / 4, 1)
}

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
