import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import type { ImageSourceCache, ImageSourceCacheEntry } from './imageSourceCache'
import {
  createMainThreadImageProcessingBackend,
  type ImageAdjustmentPlanExecutor,
} from './mainThreadImageProcessingBackend'
import type {
  ImageProcessingBackendTaskContext,
  ImageProcessingOutputPolicy,
  ImageProcessingProgress,
} from './types'

const createObjectUrlMock = vi.fn()
const revokeObjectUrlMock = vi.fn()

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  createObjectUrlMock.mockReset()
  revokeObjectUrlMock.mockReset()
})

describe('createMainThreadImageProcessingBackend', () => {
  it('uses an injected plan executor for previews and writes its ImageData to the canvas', async () => {
    const source = createImageSource('preview-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ preview: source })
    const adjusted = makeImageData([1, 2, 3, 255])
    const planExecutor = createPlanExecutor(adjusted)
    installCanvasMocks()
    installUrlMocks('blob:preview')
    const backend = createMainThreadImageProcessingBackend({ sourceCache, planExecutor })

    const result = await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: createInvertPlan(),
      context: createTaskContext(),
    })

    expect(result).toMatchObject({
      objectUrl: 'blob:preview',
      width: 1,
      height: 1,
      mimeType: 'image/webp',
    })
    expect(planExecutor.execute).toHaveBeenCalledWith({
      imageData: source.imageData,
      plan: createInvertPlan(),
      signal: expect.any(AbortSignal),
      onProgress: expect.any(Function),
    })
    const renderContext = getLastCreatedCanvasContext()
    expect(renderContext?.putImageData).toHaveBeenCalledWith(adjusted, 0, 0)
  })

  it('uses an injected plan executor for final renders and writes its ImageData to the source canvas', async () => {
    const source = createImageSource('final-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    const adjusted = makeImageData([4, 5, 6, 255])
    const planExecutor = createPlanExecutor(adjusted)
    installCanvasMocks()
    const backend = createMainThreadImageProcessingBackend({ sourceCache, planExecutor })

    const result = await backend.renderFinal({
      source: { url: 'https://example.com/source.png', title: 'source' },
      plan: createInvertPlan(),
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
      plan: createInvertPlan(),
      signal: expect.any(AbortSignal),
      onProgress: expect.any(Function),
    })
    expect(source.context.putImageData).toHaveBeenCalledWith(adjusted, 0, 0)
    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
  })

  it('uses a CPU plan executor by default', async () => {
    const source = createImageSource('preview-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ preview: source })
    installCanvasMocks()
    installUrlMocks('blob:preview')
    const backend = createMainThreadImageProcessingBackend({ sourceCache })

    const result = await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: createInvertPlan(),
      context: createTaskContext(),
    })

    expect(result.objectUrl).toBe('blob:preview')
    expect(getLastCreatedCanvasContext()?.putImageData).toHaveBeenCalledWith(
      expect.objectContaining({
        data: new Uint8ClampedArray([245, 235, 225, 255]),
      }),
      0,
      0,
    )
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

type TestSourceCache = Pick<
  ImageSourceCache,
  'dispose' | 'getFullSizeSource' | 'getPreviewSource' | 'releaseSource'
>

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

function installUrlMocks(objectUrl: string) {
  createObjectUrlMock.mockReturnValue(objectUrl)
  vi.spyOn(URL, 'createObjectURL').mockImplementation(createObjectUrlMock)
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(revokeObjectUrlMock)
}

function getLastCreatedCanvasContext() {
  return createdCanvasContexts.at(-1)
}

function createSourceCache(input: {
  preview?: TestImageSource | null
  fullSize?: TestImageSource | null
}): ImageSourceCache {
  const sourceCache: TestSourceCache = {
    getPreviewSource: vi.fn(async () => input.preview ?? null),
    getFullSizeSource: vi.fn(async () => input.fullSize ?? null),
    releaseSource: vi.fn(),
    dispose: vi.fn(),
  }
  return sourceCache as ImageSourceCache
}

function createImageSource(key: string, values: number[]): TestImageSource {
  const imageData = makeImageData(values)
  const { canvas, context } = createTestCanvas(imageData)
  return {
    key,
    purpose: 'preview',
    sourceUrl: 'https://example.com/source.png',
    fetchUrl: 'https://example.com/source.png',
    width: imageData.width,
    height: imageData.height,
    mimeType: 'image/png',
    canvas,
    context: context as CanvasRenderingContext2D & TestCanvasContext,
    imageData,
  }
}

function createTestCanvas(initialImageData: ImageData) {
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
      callback(new Blob(['rendered'], { type: type || 'image/png' }))
    }),
  })
  createdCanvasContexts.push(context)
  return { canvas, context }
}

function createTaskContext(input: { signal?: AbortSignal } = {}): ImageProcessingBackendTaskContext {
  return {
    taskId: 'task:1',
    revision: 1,
    signal: input.signal ?? new AbortController().signal,
    outputPolicy: createOutputPolicy(),
    reportProgress: vi.fn<(progress: ImageProcessingProgress) => void>(),
  }
}

function createOutputPolicy(): ImageProcessingOutputPolicy {
  return {
    resolveEncoding: ({ kind }) => ({
      mimeType: kind === 'preview' ? 'image/webp' : 'image/png',
    }),
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
  return new TestImageDataImpl(new Uint8ClampedArray(values), values.length / 4, 1) as TestImageData
}

function createInvertPlan(): ImageAdjustmentPlan {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = 255 - index
  }
  return [{ kind: 'channel-lut', lut }]
}
