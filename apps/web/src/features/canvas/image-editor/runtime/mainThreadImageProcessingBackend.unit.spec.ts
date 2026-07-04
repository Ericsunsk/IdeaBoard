import { afterEach, describe, expect, it, vi } from 'vitest'
import { WEBP_PREVIEW_QUALITY } from '../../utils/displayImagePreviewEncoding'
import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import type { ImageSourceCache, ImageSourceCacheEntry } from './imageSourceCache'
import { releaseImageSourceCanvas } from './imageSourceCache'
import { createImageProcessingOutputPolicy } from './imageOutputPolicy'
import { createMainThreadImageProcessingBackend } from './mainThreadImageProcessingBackend'
import type { ImageAdjustmentPlanExecutor } from './planExecutorTypes'
import type {
  ImageProcessingBackendTaskContext,
  ImageProcessingOutputPolicy,
  ImageProcessingProgress,
} from './types'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('createMainThreadImageProcessingBackend', () => {
  it('uses an injected plan executor for final renders and writes its ImageData to the source canvas', async () => {
    const source = createImageSource('final-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    const adjusted = makeImageData([4, 5, 6, 255])
    const planExecutor = createPlanExecutor(adjusted)
    installCanvasMocks()
    const backend = createMainThreadImageProcessingBackend({ sourceCache, planExecutor })

    const result = await backend.renderFinal({
      source: { url: 'https://example.com/source.png', title: 'source' },
      plan: createChannelLutPlan(),
      context: createTaskContext(),
    })

    expect(result).toMatchObject({
      width: 1,
      height: 1,
      mimeType: 'image/png',
    })
    expect(result.file.name).toBe('source.png')
    expect(planExecutor.execute).toHaveBeenCalledWith({
      imageData: source.imageData,
      plan: createChannelLutPlan(),
      signal: expect.any(AbortSignal),
      onProgress: expect.any(Function),
    })
    expect(source.context.putImageData).toHaveBeenCalledWith(adjusted, 0, 0)
    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
  })

  it('exports transparent final output as png and names the file from the output mime', async () => {
    const { result, source } = await renderFinalWithAdjustedPixels({
      adjustedPixels: [4, 5, 6, 0],
      source: { url: 'https://example.com/source.webp', title: 'Transparent.webp' },
    })

    expect(result.file.name).toBe('Transparent-adjusted.png')
    expect(result.file.type).toBe('image/png')
    expect(result.mimeType).toBe('image/png')
    expect(source.canvas.toBlob).toHaveBeenCalledWith(expect.any(Function), 'image/png', undefined)
  })

  it('exports opaque final output as webp with the configured quality', async () => {
    const { result, source } = await renderFinalWithAdjustedPixels({
      adjustedPixels: [4, 5, 6, 255],
      source: { url: 'https://example.com/source.png', title: 'Opaque.png' },
    })

    expect(result.file.name).toBe('Opaque-adjusted.webp')
    expect(result.file.type).toBe('image/webp')
    expect(result.mimeType).toBe('image/webp')
    expect(source.canvas.toBlob).toHaveBeenCalledWith(
      expect.any(Function),
      'image/webp',
      WEBP_PREVIEW_QUALITY,
    )
  })

  it('uses the actual encoded blob mime type for the final file name', async () => {
    const { result, source } = await renderFinalWithAdjustedPixels({
      adjustedPixels: [4, 5, 6, 255],
      canvas: { toBlobType: 'image/png' },
      source: { url: 'https://example.com/source.png', title: 'Actual Mime.png' },
    })

    expect(source.canvas.toBlob).toHaveBeenCalledWith(
      expect.any(Function),
      'image/webp',
      WEBP_PREVIEW_QUALITY,
    )
    expect(result.file.name).toBe('Actual-Mime-adjusted.png')
    expect(result.file.type).toBe('image/png')
    expect(result.mimeType).toBe('image/png')
  })

  it('releases the full-size source canvas when final encoding fails', async () => {
    const source = createImageSource('encode-failure-source', [10, 20, 30, 255], {
      canvas: { toBlobReturnsNull: true },
    })
    const sourceCache = createSourceCache({ fullSize: source, releaseCanvas: true })
    const adjusted = makeImageData([4, 5, 6, 255])
    const backend = createMainThreadImageProcessingBackend({
      sourceCache,
      planExecutor: createPlanExecutor(adjusted),
    })

    await expect(
      backend.renderFinal({
        source: { url: 'https://example.com/source.png', title: 'Encode Failure.png' },
        plan: createChannelLutPlan(),
        context: createTaskContext({ outputPolicy: createImageProcessingOutputPolicy() }),
      }),
    ).rejects.toThrow('Canvas toBlob returned null')

    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
    expect(source.canvas.width).toBe(0)
    expect(source.canvas.height).toBe(0)
  })

  it('releases the full-size source canvas when final apply is cancelled before writeback', async () => {
    const source = createImageSource('cancelled-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source, releaseCanvas: true })
    const controller = new AbortController()
    const adjusted = makeImageData([4, 5, 6, 255])
    const planExecutor: ImageAdjustmentPlanExecutor = {
      execute: vi.fn(() => {
        controller.abort()
        return adjusted
      }),
      dispose: vi.fn(),
    }
    const backend = createMainThreadImageProcessingBackend({ sourceCache, planExecutor })

    await expect(
      backend.renderFinal({
        source: { url: 'https://example.com/source.png', title: 'Cancelled.png' },
        plan: createChannelLutPlan(),
        context: createTaskContext({
          signal: controller.signal,
          outputPolicy: createImageProcessingOutputPolicy(),
        }),
      }),
    ).rejects.toThrow('图像处理已取消')

    expect(source.context.putImageData).not.toHaveBeenCalled()
    expect(source.canvas.toBlob).not.toHaveBeenCalled()
    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
    expect(source.canvas.width).toBe(0)
    expect(source.canvas.height).toBe(0)
  })

  it('uses a CPU plan executor by default', async () => {
    const source = createImageSource('final-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    installCanvasMocks()
    const backend = createMainThreadImageProcessingBackend({ sourceCache })

    const result = await backend.renderFinal({
      source: { url: 'https://example.com/source.png' },
      plan: createChannelLutPlan(),
      context: createTaskContext(),
    })

    expect(result.file.name).toBe('image.png')
    expect(source.context.putImageData).toHaveBeenCalledWith(
      expect.objectContaining({
        data: new Uint8ClampedArray([20, 30, 40, 255]),
      }),
      0,
      0,
    )
  })

  it('renders preview output through the injected plan executor and returns the decoded source', async () => {
    const source = createImageSource('preview-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    const adjusted = makeImageData([20, 30, 40, 255])
    const planExecutor = createPlanExecutor(adjusted)
    const backend = createMainThreadImageProcessingBackend({ sourceCache, planExecutor })

    const result = await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: createChannelLutPlan(),
      sizeStrategy: { maxSide: 960 },
      context: createTaskContext(),
    })

    expect(sourceCache.getFullSizeSource).toHaveBeenCalledWith('https://example.com/source.png', {
      maxSide: 960,
    })
    expect(planExecutor.execute).toHaveBeenCalledWith({
      imageData: expect.objectContaining({
        data: new Uint8ClampedArray([10, 20, 30, 255]),
      }),
      plan: createChannelLutPlan(),
      signal: expect.any(AbortSignal),
      onProgress: expect.any(Function),
    })
    expect(result).toMatchObject({
      sourceImageData: source.imageData,
      previewImageData: adjusted,
      width: 1,
      height: 1,
    })
    expect(source.context.putImageData).not.toHaveBeenCalled()
    expect(sourceCache.releaseSource).not.toHaveBeenCalled()
  })

  it('decodes preview source without executing a neutral plan', async () => {
    const source = createImageSource('preview-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    const planExecutor = createPlanExecutor(makeImageData([20, 30, 40, 255]))
    const backend = createMainThreadImageProcessingBackend({ sourceCache, planExecutor })

    const result = await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: [],
      context: createTaskContext(),
    })

    expect(planExecutor.execute).not.toHaveBeenCalled()
    expect(result.sourceImageData).toBe(source.imageData)
    expect(result.previewImageData).toMatchObject({
      data: new Uint8ClampedArray([10, 20, 30, 255]),
    })
    expect(result.previewImageData).not.toBe(source.imageData)
    expect(sourceCache.releaseSource).not.toHaveBeenCalled()
  })

  it('disposes the source cache with the default CPU executor', () => {
    const sourceCache = createSourceCache({})
    const backend = createMainThreadImageProcessingBackend({ sourceCache })

    expect(backend.dispose).toBeDefined()
    backend.dispose?.()

    expect(sourceCache.dispose).toHaveBeenCalledTimes(1)
  })

  it('disposes an injected plan executor when provided', () => {
    const sourceCache = createSourceCache({})
    const planExecutor = createPlanExecutor(makeImageData([1, 2, 3, 255]))
    const backend = createMainThreadImageProcessingBackend({
      sourceCache,
      planExecutor,
    })

    backend.dispose?.()

    expect(sourceCache.dispose).toHaveBeenCalledTimes(1)
    expect(planExecutor.dispose).toHaveBeenCalledTimes(1)
  })
})

type TestImageData = ImageData & {
  data: Uint8ClampedArray
  width: number
  height: number
}

type TestCanvasContext = {
  drawImage: ReturnType<typeof vi.fn>
  getImageData: ReturnType<typeof vi.fn<() => ImageData>>
  putImageData: ReturnType<typeof vi.fn<(imageData: ImageData) => void>>
}

type TestImageSource = ImageSourceCacheEntry & {
  imageData: ImageData
  context: CanvasRenderingContext2D & TestCanvasContext
}

type TestSourceCache = Pick<ImageSourceCache, 'dispose' | 'getFullSizeSource' | 'releaseSource'>

const createdCanvasContexts: TestCanvasContext[] = []

function installCanvasMocks() {
  createdCanvasContexts.length = 0
  vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') {
      return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
    }
    return createTestCanvas(makeImageData([0, 0, 0, 255])).canvas
  })
}

function createSourceCache(input: {
  fullSize?: TestImageSource | null
  releaseCanvas?: boolean
}): ImageSourceCache {
  const sourceCache: TestSourceCache = {
    getFullSizeSource: vi.fn(async () => input.fullSize ?? null),
    releaseSource: vi.fn((source) => {
      if (input.releaseCanvas) releaseImageSourceCanvas(source as ImageSourceCacheEntry)
    }),
    dispose: vi.fn(),
  }
  return sourceCache as ImageSourceCache
}

async function renderFinalWithAdjustedPixels(input: {
  adjustedPixels: number[]
  canvas?: TestCanvasOptions
  source: { url: string; title: string }
}) {
  const source = createImageSource('output-policy-source', [10, 20, 30, 255], {
    canvas: input.canvas,
  })
  const sourceCache = createSourceCache({ fullSize: source })
  const backend = createMainThreadImageProcessingBackend({
    sourceCache,
    planExecutor: createPlanExecutor(makeImageData(input.adjustedPixels)),
  })

  const result = await backend.renderFinal({
    source: input.source,
    plan: createChannelLutPlan(),
    context: createTaskContext({ outputPolicy: createImageProcessingOutputPolicy() }),
  })

  return { result, source }
}

function createImageSource(
  key: string,
  values: number[],
  options: {
    mimeType?: string
    canvas?: TestCanvasOptions
  } = {},
): TestImageSource {
  const imageData = makeImageData(values)
  const { canvas, context } = createTestCanvas(imageData, options.canvas)
  return {
    key,
    sourceUrl: 'https://example.com/source.png',
    fetchUrl: 'https://example.com/source.png',
    width: imageData.width,
    height: imageData.height,
    mimeType: options.mimeType ?? 'image/png',
    canvas,
    context: context as CanvasRenderingContext2D & TestCanvasContext,
    imageData,
  }
}

type TestCanvasOptions = {
  toBlobType?: string
  toBlobReturnsNull?: boolean
}

function createTestCanvas(initialImageData: ImageData, options: TestCanvasOptions = {}) {
  let currentImageData = initialImageData
  const canvas = document.createElementNS(
    'http://www.w3.org/1999/xhtml',
    'canvas',
  ) as HTMLCanvasElement
  canvas.width = initialImageData.width
  canvas.height = initialImageData.height
  const context: TestCanvasContext = {
    drawImage: vi.fn(),
    getImageData: vi.fn(() => currentImageData),
    putImageData: vi.fn((imageData: ImageData) => {
      currentImageData = imageData
    }),
  }
  Object.defineProperty(canvas, 'getContext', {
    value: () => context,
  })
  Object.defineProperty(canvas, 'toBlob', {
    value: vi.fn((callback: BlobCallback, type?: string) => {
      if (options.toBlobReturnsNull) {
        callback(null)
        return
      }
      callback(new Blob(['rendered'], { type: options.toBlobType ?? type ?? 'image/png' }))
    }),
  })
  createdCanvasContexts.push(context)
  return { canvas, context }
}

function createTaskContext(
  input: { signal?: AbortSignal; outputPolicy?: ImageProcessingOutputPolicy } = {},
): ImageProcessingBackendTaskContext {
  return {
    taskId: 'task:1',
    revision: 1,
    signal: input.signal ?? new AbortController().signal,
    outputPolicy: input.outputPolicy ?? createOutputPolicy(),
    reportProgress: vi.fn<(progress: ImageProcessingProgress) => void>(),
  }
}

function createOutputPolicy(): ImageProcessingOutputPolicy {
  return {
    resolveEncoding: () => ({ mimeType: 'image/png' }),
    resolveFileName: ({ title }) => `${title ?? 'image'}.png`,
  }
}

function createPlanExecutor(output: ImageData): ImageAdjustmentPlanExecutor {
  return {
    execute: vi.fn(() => output),
    dispose: vi.fn(),
  }
}

function makeImageData(values: number[]): ImageData {
  vi.stubGlobal('ImageData', TestImageDataImpl)
  return new TestImageDataImpl(new Uint8ClampedArray(values), values.length / 4, 1) as TestImageData
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

function createChannelLutPlan(): ImageAdjustmentPlan {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = Math.min(255, index + 10)
  }
  return [{ kind: 'channel-lut', luts: createRgbChannelLutsForTest(lut) }]
}

function createRgbChannelLutsForTest(lut: Uint8ClampedArray) {
  return {
    red: lut,
    green: lut,
    blue: lut,
  }
}
