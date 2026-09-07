import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageRetouchPlan } from '../utils/imageRetouchEngine'
import type { ImageSourceCache, ImageSourceCacheEntry } from './imageSourceCache'
import {
  createCpuMainImageProcessingBackend,
  createCpuWorkerImageProcessingBackend,
  createImageRetouchProcessingBackend,
} from './imageProcessingBackends'

const { disposeDefaultWorkerImageProcessingClientMock, executeWorkerImageRetouchPlanMock } =
  vi.hoisted(() => ({
    disposeDefaultWorkerImageProcessingClientMock: vi.fn(),
    executeWorkerImageRetouchPlanMock: vi.fn(),
  }))

vi.mock('../backends/worker/executeWorkerImageRetouchPlan', () => ({
  disposeDefaultWorkerImageProcessingClient: disposeDefaultWorkerImageProcessingClientMock,
  executeWorkerImageRetouchPlan: executeWorkerImageRetouchPlanMock,
}))

afterEach(() => {
  vi.restoreAllMocks()
  executeWorkerImageRetouchPlanMock.mockReset()
  disposeDefaultWorkerImageProcessingClientMock.mockReset()
})

describe('image processing backend factories', () => {
  it('creates a cpu-main backend that executes plans on the main-thread CPU executor', async () => {
    const source = createImageSource([10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    const onPlanExecutorEvent = vi.fn()
    installCanvasMock()
    const backend = createCpuMainImageProcessingBackend({
      sourceCache,
      onPlanExecutorEvent,
    })

    await backend.renderFinal({
      source: { url: 'https://example.com/source.png' },
      plan: createChannelLutPlan(),
      context: createTaskContext(),
    })

    expect(executeWorkerImageRetouchPlanMock).not.toHaveBeenCalled()
    expect(source.context.putImageData).toHaveBeenCalledWith(
      expect.objectContaining({
        data: new Uint8ClampedArray([20, 30, 40, 255]),
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
    const sourceCache = createSourceCache({ fullSize: source })
    const onPlanExecutorEvent = vi.fn()
    executeWorkerImageRetouchPlanMock.mockResolvedValue(workerOutput)
    installCanvasMock()
    const backend = createCpuWorkerImageProcessingBackend({
      sourceCache,
      onPlanExecutorEvent,
    })

    await backend.renderFinal({
      source: { url: 'https://example.com/source.png' },
      plan: createChannelLutPlan(),
      context: createTaskContext(),
    })
    backend.dispose?.()

    expect(executeWorkerImageRetouchPlanMock).toHaveBeenCalledTimes(1)
    expect(source.context.putImageData).toHaveBeenCalledWith(workerOutput, 0, 0)
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

  it('selects the hybrid backend by default and keeps small work on the main thread', async () => {
    const source = createImageSource([10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    executeWorkerImageRetouchPlanMock.mockResolvedValue(makeImageData([4, 5, 6, 255]))
    installCanvasMock()
    const backend = createImageRetouchProcessingBackend({ sourceCache })

    await backend.renderFinal({
      source: { url: 'https://example.com/source.png' },
      plan: createChannelLutPlan(),
      context: createTaskContext(),
    })

    expect(executeWorkerImageRetouchPlanMock).not.toHaveBeenCalled()
    expect(source.context.putImageData).toHaveBeenCalledWith(
      expect.objectContaining({
        data: new Uint8ClampedArray([20, 30, 40, 255]),
      }),
      0,
      0,
    )
  })

  it('can still create an explicit cpu-worker backend for heavy image work', async () => {
    const source = createImageSource([10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    executeWorkerImageRetouchPlanMock.mockResolvedValue(makeImageData([4, 5, 6, 255]))
    installCanvasMock()
    const backend = createImageRetouchProcessingBackend({ backend: 'cpu-worker', sourceCache })

    await backend.renderFinal({
      source: { url: 'https://example.com/source.png' },
      plan: createChannelLutPlan(),
      context: createTaskContext(),
    })

    expect(executeWorkerImageRetouchPlanMock).toHaveBeenCalledTimes(1)
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

type TestSourceCache = Pick<ImageSourceCache, 'dispose' | 'getFullSizeSource' | 'releaseSource'>

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

function createSourceCache(input: { fullSize?: TestImageSource | null }): ImageSourceCache {
  const sourceCache: TestSourceCache = {
    getFullSizeSource: vi.fn(async () => input.fullSize ?? null),
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

function createChannelLutPlan(): ImageRetouchPlan {
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
