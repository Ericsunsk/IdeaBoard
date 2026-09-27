import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RetouchEffectLayer } from '../model.ts'
import type { ImageProcessingRuntime } from '../runtime/types'
import * as runtimeModule from '../runtime/processingRuntime.ts'
import { buildImageRetouchPlan } from '../processing/planPipeline.ts'
import { cancelImageRetouchFinalRender, renderImageRetouchFile } from './renderer'

const rendererMocks = vi.hoisted(() => ({
  updateSourceImage: vi.fn(),
  updateUniforms: vi.fn(),
  render: vi.fn(),
  readPixels: vi.fn(),
  readHistogramSample: vi.fn(),
  dispose: vi.fn(),
  isContextLost: vi.fn(() => false),
}))

vi.mock('../processing/webglEngine.ts', async (importOriginal) => {
  const original = await importOriginal<typeof import('../processing/webglEngine.ts')>()
  return {
    ...original,
    isWebGLRetouchSupported: () => true,
    createWebGLRetouchRenderer: () => ({ ...rendererMocks }),
  }
})

const IMAGE_RETOUCH_FINAL_TASK_ID = 'image-retouch-final'

const fetchMock = vi.fn()
const createImageBitmapMock = vi.fn()
const ANIMATED_WEBP_BYTES = new Uint8Array([
  0x52, 0x49, 0x46, 0x46, 0x12, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x41, 0x4e, 0x49, 0x4d,
  0x00, 0x00, 0x00, 0x00,
])

const hslLayer: RetouchEffectLayer = {
  id: 'retouch:hsl',
  type: 'effect',
  kind: 'hsl',
  name: '色相/饱和度 1',
  visible: true,
  params: { hue: 35, saturation: 22, lightness: 8 },
}

const thresholdLayer: RetouchEffectLayer = {
  id: 'retouch:threshold',
  type: 'effect',
  kind: 'threshold',
  name: '阈值 1',
  visible: true,
  params: { threshold: 150, invert: true },
}

const curvesLayer: RetouchEffectLayer = {
  id: 'retouch:curves',
  type: 'effect',
  kind: 'curves',
  name: '曲线 1',
  visible: true,
  params: {
    channels: {
      rgb: [{ id: 'point:midtone', input: 42, output: 70 }],
    },
  },
}

const levelsLayer: RetouchEffectLayer = {
  id: 'retouch:levels',
  type: 'effect',
  kind: 'levels',
  name: '色阶 1',
  visible: true,
  params: { shadows: 20, midtones: 112, highlights: 235, outputBlack: 0, outputWhite: 255 },
}

const sharpenLayer: RetouchEffectLayer = {
  id: 'retouch:sharpen',
  type: 'effect',
  kind: 'sharpen',
  name: '锐化 1',
  visible: true,
  params: { amount: 120, radius: 1.5, threshold: 4, protectColor: true },
}

function createRuntimeMock(
  renderFinal: ImageProcessingRuntime['renderFinal'] = vi.fn(async () => ({
    status: 'completed' as const,
    taskId: IMAGE_RETOUCH_FINAL_TASK_ID,
    revision: 1,
    output: {
      file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
    },
  })),
): ImageProcessingRuntime {
  return {
    renderFinal,
    renderPreview: vi.fn<ImageProcessingRuntime['renderPreview']>(),
    cancel: vi.fn(),
    dispose: vi.fn(),
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  fetchMock.mockReset()
  createImageBitmapMock.mockReset()
  rendererMocks.updateSourceImage.mockReset()
  rendererMocks.updateUniforms.mockReset()
  rendererMocks.render.mockReset()
  rendererMocks.readPixels.mockReset()
  rendererMocks.readHistogramSample.mockReset()
  rendererMocks.dispose.mockReset()
  rendererMocks.isContextLost.mockReset().mockReturnValue(false)
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

describe('renderImageRetouchFile', () => {
  it('uses the default final task id and internal latest-only scheduling', async () => {
    const renderFinal = vi.fn<ImageProcessingRuntime['renderFinal']>(async () => ({
      status: 'completed',
      taskId: IMAGE_RETOUCH_FINAL_TASK_ID,
      revision: 1,
      output: {
        file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
      },
    }))
    vi.spyOn(runtimeModule, 'getImageRetouchProcessingRuntime').mockReturnValue(
      createRuntimeMock(renderFinal),
    )

    await renderImageRetouchFile({ url: 'https://example.com/source.png' }, [thresholdLayer])

    expect(renderFinal).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: IMAGE_RETOUCH_FINAL_TASK_ID,
        latestOnly: true,
      }),
    )
  })

  it('uses scoped final task ids while keeping runtime options internal', async () => {
    const onProgress = vi.fn()
    const renderFinal = vi.fn<ImageProcessingRuntime['renderFinal']>(async () => ({
      status: 'completed',
      taskId: 'image-retouch-final:node-1',
      revision: 1,
      output: {
        file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
      },
    }))
    vi.spyOn(runtimeModule, 'getImageRetouchProcessingRuntime').mockReturnValue(
      createRuntimeMock(renderFinal),
    )

    await renderImageRetouchFile(
      { url: 'https://example.com/source.png', title: 'Sample.png' },
      [thresholdLayer],
      {
        scopeId: 'node-1',
        onProgress,
      },
    )

    expect(renderFinal).toHaveBeenCalledWith(
      expect.objectContaining({
        source: { url: 'https://example.com/source.png', title: 'Sample.png' },
        taskId: 'image-retouch-final:node-1',
        latestOnly: true,
        onProgress,
      }),
    )
  })

  it('passes the compiled retouch plan to final rendering without a separate final normalization path', async () => {
    const layers = [hslLayer, curvesLayer, levelsLayer, sharpenLayer, thresholdLayer]
    const renderFinal = vi.fn<ImageProcessingRuntime['renderFinal']>(async () => ({
      status: 'completed',
      taskId: IMAGE_RETOUCH_FINAL_TASK_ID,
      revision: 1,
      output: {
        file: new File(['rendered'], 'rendered.webp', { type: 'image/webp' }),
      },
    }))
    vi.spyOn(runtimeModule, 'getImageRetouchProcessingRuntime').mockReturnValue(
      createRuntimeMock(renderFinal),
    )

    await renderImageRetouchFile({ url: 'https://example.com/source.png' }, layers)

    expect(renderFinal).toHaveBeenCalledWith(
      expect.objectContaining({
        plan: buildImageRetouchPlan(layers),
      }),
    )
  })

  it('cancels final render tasks through the runtime facade', () => {
    const runtime = createRuntimeMock()
    vi.spyOn(runtimeModule, 'getImageRetouchProcessingRuntime').mockReturnValue(runtime)

    cancelImageRetouchFinalRender()
    cancelImageRetouchFinalRender('node-1')

    expect(runtime.cancel).toHaveBeenNthCalledWith(1, IMAGE_RETOUCH_FINAL_TASK_ID)
    expect(runtime.cancel).toHaveBeenNthCalledWith(2, 'image-retouch-final:node-1')
  })

  it('renders visible retouch layers to an uploadable file', async () => {
    installCanvasMocks()
    rendererMocks.readPixels.mockReturnValue({
      data: new Uint8ClampedArray([10, 20, 30, 255]),
      width: 1,
      height: 1,
    })
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

    const result = await renderImageRetouchFile(
      { url: 'https://example.com/source.png', title: 'Sample.png' },
      [thresholdLayer],
    )

    expect(fetchMock).toHaveBeenCalledWith(
      'https://example.com/source.png',
      expect.objectContaining({ credentials: 'include' }),
    )
    expect(result.name).toBe('Sample-adjusted.webp')
    expect(result.type).toBe('image/webp')
    expect(rendererMocks.render).toHaveBeenCalled()
    expect(rendererMocks.dispose).toHaveBeenCalled()
  })

  it('rejects empty visible retouch stacks', async () => {
    await expect(
      renderImageRetouchFile({ url: 'https://example.com/source.png' }, [
        { ...thresholdLayer, visible: false },
      ]),
    ).rejects.toThrow('No editable layers to apply.')
  })

  it('rejects visually neutral retouch stacks before loading the image', async () => {
    await expect(
      renderImageRetouchFile({ url: 'https://example.com/source.png' }, [
        { ...hslLayer, params: { hue: 0, saturation: 0, lightness: 0 } },
      ]),
    ).rejects.toThrow('No editable layers to apply.')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('rejects runtime final rendering failures with the runtime message', async () => {
    const renderFinal = vi.fn<ImageProcessingRuntime['renderFinal']>(async () => ({
      status: 'failed',
      taskId: IMAGE_RETOUCH_FINAL_TASK_ID,
      revision: 1,
      error: {
        code: 'backend-failed',
        message: '渲染服务暂不可用',
      },
    }))
    vi.spyOn(runtimeModule, 'getImageRetouchProcessingRuntime').mockReturnValue(
      createRuntimeMock(renderFinal),
    )

    await expect(
      renderImageRetouchFile({ url: 'https://example.com/source.png' }, [thresholdLayer]),
    ).rejects.toThrow('渲染服务暂不可用')
  })

  it('rejects animated images because retouch output is a still frame', async () => {
    installCanvasMocks()
    fetchMock.mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['source'], { type: 'image/gif' }),
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      renderImageRetouchFile({ url: 'https://example.com/source.gif' }, [thresholdLayer]),
    ).rejects.toThrow('Animated images are not supported for editing.')
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
      renderImageRetouchFile({ url: 'https://example.com/source.webp' }, [thresholdLayer]),
    ).rejects.toThrow('Animated images are not supported for editing.')
    expect(createImageBitmapMock).not.toHaveBeenCalled()
  })
})
