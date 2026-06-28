import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import type { ImageSourceCache, ImageSourceCacheEntry } from './imageSourceCache'
import {
  createCpuMainImageProcessingBackend,
  createCpuWorkerImageProcessingBackend,
  createImageAdjustmentProcessingBackend,
} from './imageProcessingBackends'

const { disposeDefaultWorkerImageProcessingClientMock, executeWorkerImageAdjustmentPlanMock } =
  vi.hoisted(() => ({
    disposeDefaultWorkerImageProcessingClientMock: vi.fn(),
    executeWorkerImageAdjustmentPlanMock: vi.fn(),
  }))

vi.mock('../backends/worker/executeWorkerImageAdjustmentPlan', () => ({
  disposeDefaultWorkerImageProcessingClient: disposeDefaultWorkerImageProcessingClientMock,
  executeWorkerImageAdjustmentPlan: executeWorkerImageAdjustmentPlanMock,
}))

afterEach(() => {
  vi.restoreAllMocks()
  executeWorkerImageAdjustmentPlanMock.mockReset()
  disposeDefaultWorkerImageProcessingClientMock.mockReset()
})

describe('image processing backend factories', () => {
  it('creates a cpu-main backend that executes plans on the main-thread CPU executor', async () => {
    const source = createImageSource([10, 20, 30, 255])
    const sourceCache = createSourceCache({ preview: source })
    const onPlanExecutorEvent = vi.fn()
    installCanvasMock()
    const backend = createCpuMainImageProcessingBackend({
      sourceCache,
      onPlanExecutorEvent,
    })

    await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: createInvertPlan(),
      context: createTaskContext(),
    })

    expect(executeWorkerImageAdjustmentPlanMock).not.toHaveBeenCalled()
    expect(getLastCreatedCanvasContext()?.putImageData).toHaveBeenCalledWith(
      expect.objectContaining({
        data: new Uint8ClampedArray([245, 235, 225, 255]),
      }),
      0,
      0,
    )
    expect(onPlanExecutorEvent).toHaveBeenCalledWith({
      kind: 'cpu-plan-completed',
      planLength: 1,
      width: 1,
      height: 1,
    })
  })

  it('creates a cpu-worker backend that uses the worker plan executor and disposes it', async () => {
    const source = createImageSource([10, 20, 30, 255])
    const workerOutput = makeImageData([1, 2, 3, 255])
    const sourceCache = createSourceCache({ preview: source })
    const onPlanExecutorEvent = vi.fn()
    executeWorkerImageAdjustmentPlanMock.mockResolvedValue(workerOutput)
    installCanvasMock()
    const backend = createCpuWorkerImageProcessingBackend({
      sourceCache,
      onPlanExecutorEvent,
    })

    await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: createInvertPlan(),
      context: createTaskContext(),
    })
    backend.dispose?.()

    expect(executeWorkerImageAdjustmentPlanMock).toHaveBeenCalledTimes(1)
    expect(getLastCreatedCanvasContext()?.putImageData).toHaveBeenCalledWith(workerOutput, 0, 0)
    expect(disposeDefaultWorkerImageProcessingClientMock).toHaveBeenCalledTimes(1)
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(1, {
      kind: 'worker-plan-started',
      planLength: 1,
      width: 1,
      height: 1,
    })
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(2, {
      kind: 'worker-plan-completed',
      planLength: 1,
      width: 1,
      height: 1,
    })
  })

  it('selects the cpu-worker backend by default', async () => {
    const source = createImageSource([10, 20, 30, 255])
    const sourceCache = createSourceCache({ preview: source })
    executeWorkerImageAdjustmentPlanMock.mockResolvedValue(makeImageData([4, 5, 6, 255]))
    installCanvasMock()
    const backend = createImageAdjustmentProcessingBackend({ sourceCache })

    await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: createInvertPlan(),
      context: createTaskContext(),
    })

    expect(executeWorkerImageAdjustmentPlanMock).toHaveBeenCalledTimes(1)
  })
})

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

function installCanvasMock() {
  createdCanvasContexts.length = 0
  vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') {
      return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
    }
    return createTestCanvas(makeImageData([0, 0, 0, 255])).canvas
  })
}

function getLastCreatedCanvasContext() {
  return createdCanvasContexts.at(-1)
}

function createSourceCache(input: { preview?: TestImageSource | null }): ImageSourceCache {
  const sourceCache: TestSourceCache = {
    getPreviewSource: vi.fn(async () => input.preview ?? null),
    getFullSizeSource: vi.fn(async () => input.preview ?? null),
    releaseSource: vi.fn(),
    dispose: vi.fn(),
  }
  return sourceCache as ImageSourceCache
}

function createImageSource(values: number[]): TestImageSource {
  const imageData = makeImageData(values)
  const { canvas, context } = createTestCanvas(imageData)

  return {
    key: 'test-source',
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
      callback(new Blob(['rendered'], { type: type || 'image/webp' }))
    }),
  })
  createdCanvasContexts.push(context)
  return { canvas, context }
}

function createTaskContext() {
  return {
    taskId: 'task:1',
    revision: 1,
    signal: new AbortController().signal,
    outputPolicy: {
      resolveEncoding: () => ({ mimeType: 'image/webp' as const }),
      resolveFileName: () => 'image.webp',
    },
    reportProgress: vi.fn(),
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
  return new TestImageDataImpl(new Uint8ClampedArray(values), values.length / 4, 1) as ImageData
}

function createInvertPlan(): ImageAdjustmentPlan {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = 255 - index
  }
  return [{ kind: 'channel-lut', lut }]
}
