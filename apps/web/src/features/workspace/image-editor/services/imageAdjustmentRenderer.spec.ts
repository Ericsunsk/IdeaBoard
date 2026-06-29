import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageEditorAdjustmentLayer } from '../types'
import type { ImageProcessingRuntime } from '../runtime/types'
import * as runtimeModule from '../runtime/imageAdjustmentProcessingRuntime'
import { buildImageAdjustmentPlan } from '../utils/imageAdjustmentPlanPipeline'
import {
  cancelImageAdjustmentFinalRender,
  IMAGE_ADJUSTMENT_FINAL_TASK_ID,
  renderImageAdjustmentFile,
} from './imageAdjustmentRenderer'

const fetchMock = vi.fn()
const createImageBitmapMock = vi.fn()
const ANIMATED_WEBP_BYTES = new Uint8Array([
  0x52, 0x49, 0x46, 0x46, 0x12, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x41, 0x4e, 0x49, 0x4d,
  0x00, 0x00, 0x00, 0x00,
])

const invertLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:invert',
  type: 'adjustment',
  kind: 'invert',
  name: '反相 1',
  visible: true,
  params: { enabled: true },
}

const hslLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:hsl',
  type: 'adjustment',
  kind: 'hsl',
  name: '色相/饱和度 1',
  visible: true,
  params: { hue: 35, saturation: 22, brightness: 8, contrast: 14 },
}

const thresholdLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:threshold',
  type: 'adjustment',
  kind: 'threshold',
  name: '阈值 1',
  visible: true,
  params: { threshold: 150, invert: true },
}

const curvesLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:curves',
  type: 'adjustment',
  kind: 'curves',
  name: '曲线 1',
  visible: true,
  params: {
    channels: {
      rgb: [{ id: 'point:midtone', input: 42, output: 70 }],
    },
  },
}

const levelsLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:levels',
  type: 'adjustment',
  kind: 'levels',
  name: '色阶 1',
  visible: true,
  params: { shadows: 20, midtones: 112, highlights: 235, outputBlack: 0, outputWhite: 255 },
}

function createRuntimeMock(
  renderFinal: ImageProcessingRuntime['renderFinal'] = vi.fn(async () => ({
    status: 'completed' as const,
    taskId: IMAGE_ADJUSTMENT_FINAL_TASK_ID,
    revision: 1,
    output: {
      file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
    },
  })),
): ImageProcessingRuntime {
  return {
    renderFinal,
    cancel: vi.fn(),
    dispose: vi.fn(),
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  fetchMock.mockReset()
  createImageBitmapMock.mockReset()
})

function installCanvasMocks() {
  class TestImageData {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }

  const sourceImageData = new TestImageData(new Uint8ClampedArray([10, 20, 30, 255]), 1, 1)
  const contexts: Array<{
    drawImage: ReturnType<typeof vi.fn>
    getImageData: ReturnType<typeof vi.fn>
    putImageData: ReturnType<typeof vi.fn>
  }> = []

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
    Object.defineProperty(canvas, 'toBlob', {
      value: vi.fn((callback: BlobCallback) => {
        callback(new Blob(['rendered'], { type: 'image/webp' }))
      }),
    })
    contexts.push(context)
    return canvas
  })

  return { contexts }
}

describe('renderImageAdjustmentFile', () => {
  it('uses the fixed final task id without overriding the runtime latestOnly default', async () => {
    const renderFinal = vi.fn<ImageProcessingRuntime['renderFinal']>(async () => ({
      status: 'completed',
      taskId: IMAGE_ADJUSTMENT_FINAL_TASK_ID,
      revision: 1,
      output: {
        file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
      },
    }))
    vi.spyOn(runtimeModule, 'getImageAdjustmentProcessingRuntime').mockReturnValue(
      createRuntimeMock(renderFinal),
    )

    await renderImageAdjustmentFile({ url: 'https://example.com/source.png' }, [invertLayer])

    expect(renderFinal).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: IMAGE_ADJUSTMENT_FINAL_TASK_ID,
      }),
    )
    expect(renderFinal.mock.calls[0]?.[0]).not.toHaveProperty('latestOnly')
  })

  it('passes task runtime options through to final rendering', async () => {
    const onProgress = vi.fn()
    const renderFinal = vi.fn<ImageProcessingRuntime['renderFinal']>(async () => ({
      status: 'completed',
      taskId: 'custom-final-task',
      revision: 1,
      output: {
        file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
      },
    }))
    vi.spyOn(runtimeModule, 'getImageAdjustmentProcessingRuntime').mockReturnValue(
      createRuntimeMock(renderFinal),
    )

    await renderImageAdjustmentFile(
      { url: 'https://example.com/source.png', title: 'Sample.png' },
      [invertLayer],
      {
        taskId: 'custom-final-task',
        latestOnly: true,
        onProgress,
      },
    )

    expect(renderFinal).toHaveBeenCalledWith(
      expect.objectContaining({
        source: { url: 'https://example.com/source.png', title: 'Sample.png' },
        taskId: 'custom-final-task',
        latestOnly: true,
        onProgress,
      }),
    )
  })

  it('passes the compiled adjustment plan to final rendering without a separate final normalization path', async () => {
    const layers = [hslLayer, curvesLayer, levelsLayer, thresholdLayer, invertLayer]
    const renderFinal = vi.fn<ImageProcessingRuntime['renderFinal']>(async () => ({
      status: 'completed',
      taskId: IMAGE_ADJUSTMENT_FINAL_TASK_ID,
      revision: 1,
      output: {
        file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
      },
    }))
    vi.spyOn(runtimeModule, 'getImageAdjustmentProcessingRuntime').mockReturnValue(
      createRuntimeMock(renderFinal),
    )

    await renderImageAdjustmentFile({ url: 'https://example.com/source.png' }, layers)

    expect(renderFinal).toHaveBeenCalledWith(
      expect.objectContaining({
        plan: buildImageAdjustmentPlan(layers),
      }),
    )
  })

  it('cancels final render tasks through the runtime facade', () => {
    const runtime = createRuntimeMock()
    vi.spyOn(runtimeModule, 'getImageAdjustmentProcessingRuntime').mockReturnValue(runtime)

    cancelImageAdjustmentFinalRender()
    cancelImageAdjustmentFinalRender('custom-final-task')

    expect(runtime.cancel).toHaveBeenNthCalledWith(1, IMAGE_ADJUSTMENT_FINAL_TASK_ID)
    expect(runtime.cancel).toHaveBeenNthCalledWith(2, 'custom-final-task')
  })

  it('renders visible adjustment layers to an uploadable file', async () => {
    const { contexts } = installCanvasMocks()
    fetchMock.mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['source'], { type: 'image/png' }),
    })
    createImageBitmapMock.mockResolvedValue({
      width: 2,
      height: 1,
      close: vi.fn(),
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('createImageBitmap', createImageBitmapMock)

    const result = await renderImageAdjustmentFile(
      { url: 'https://example.com/source.png', title: 'Sample.png' },
      [invertLayer],
    )

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/user/download?url=https%3A%2F%2Fexample.com%2Fsource.png',
      expect.objectContaining({ credentials: 'include' }),
    )
    expect(result.name).toBe('Sample-adjusted.webp')
    expect(result.type).toBe('image/webp')
    expect(contexts.at(-1)?.putImageData).toHaveBeenCalled()
  })

  it('rejects empty visible adjustment stacks', async () => {
    await expect(
      renderImageAdjustmentFile({ url: 'https://example.com/source.png' }, [
        { ...invertLayer, visible: false },
      ]),
    ).rejects.toThrow('没有可应用的编辑图层')
  })

  it('rejects visually neutral adjustment stacks before loading the image', async () => {
    await expect(
      renderImageAdjustmentFile({ url: 'https://example.com/source.png' }, [
        { ...invertLayer, params: { enabled: false } },
      ]),
    ).rejects.toThrow('没有可应用的编辑图层')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('rejects animated images because adjustment output is a still frame', async () => {
    installCanvasMocks()
    fetchMock.mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['source'], { type: 'image/gif' }),
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      renderImageAdjustmentFile({ url: 'https://example.com/source.gif' }, [invertLayer]),
    ).rejects.toThrow('暂不支持编辑动图')
    expect(createImageBitmapMock).not.toHaveBeenCalled()
  })

  it('rejects animated webp images instead of exporting only the first frame', async () => {
    installCanvasMocks()
    fetchMock.mockResolvedValue({
      ok: true,
      blob: async () => new Blob([ANIMATED_WEBP_BYTES], { type: 'image/webp' }),
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      renderImageAdjustmentFile({ url: 'https://example.com/source.webp' }, [invertLayer]),
    ).rejects.toThrow('暂不支持编辑动图')
    expect(createImageBitmapMock).not.toHaveBeenCalled()
  })
})
