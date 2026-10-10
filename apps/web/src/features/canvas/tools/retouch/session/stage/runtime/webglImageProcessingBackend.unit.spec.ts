import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageRetouchPlan } from '../../../core/engine.ts'
import type { ImageSourceCache, ImageSourceCacheEntry } from './imageSourceCache'
import { releaseImageSourceCanvas } from './imageSourceCache'
import { createImageProcessingOutputPolicy } from './imageOutputPolicy'
import { createMainThreadImageProcessingBackend } from './webglImageProcessingBackend'
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
  it('uses an injected WebGL renderer factory for final renders and renders uniforms to output canvas', async () => {
    const source = createImageSource('final-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    const finalPixels = makeImageData([10, 20, 30, 255])
    const renderer = createRendererMock({
      readPixels: vi.fn(() => finalPixels),
    })
    installCanvasMocks()
    const backend = createMainThreadImageProcessingBackend({
      sourceCache,
      rendererFactory: () => renderer,
    })

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
    expect(renderer.updateSourceImage).toHaveBeenCalledWith(source.canvas, 1, 1)
    expect(renderer.updateUniforms).toHaveBeenCalled()
    expect(renderer.render).toHaveBeenCalled()
    expect(renderer.readPixels).toHaveBeenCalled()
    expect(createdCanvasContexts.at(-1)?.putImageData).toHaveBeenCalledWith(finalPixels, 0, 0)
    expect(renderer.dispose).toHaveBeenCalled()
    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
  })

  it('exports transparent final output as png and names the file from the output mime', async () => {
    const { result } = await renderFinalWithAdjustedPixels({
      adjustedPixels: [4, 5, 6, 0],
      source: { url: 'https://example.com/source.webp', title: 'Transparent.webp' },
    })

    expect(result.file.name).toBe('Transparent-adjusted.png')
    expect(result.file.type).toBe('image/png')
    expect(result.mimeType).toBe('image/png')
  })

  it('exports opaque final output as webp with the configured quality', async () => {
    const { result } = await renderFinalWithAdjustedPixels({
      adjustedPixels: [4, 5, 6, 255],
      source: { url: 'https://example.com/source.png', title: 'Opaque.png' },
    })

    expect(result.file.name).toBe('Opaque-adjusted.webp')
    expect(result.file.type).toBe('image/webp')
    expect(result.mimeType).toBe('image/webp')
  })

  it('uses the actual encoded blob mime type for the final file name', async () => {
    const { result } = await renderFinalWithAdjustedPixels({
      adjustedPixels: [4, 5, 6, 255],
      canvas: { toBlobType: 'image/png' },
      source: { url: 'https://example.com/source.png', title: 'Actual Mime.png' },
    })

    expect(result.file.name).toBe('Actual-Mime-adjusted.png')
    expect(result.file.type).toBe('image/png')
    expect(result.mimeType).toBe('image/png')
  })

  it('releases the full-size source canvas when final encoding fails', async () => {
    installCanvasMocks()
    const source = createImageSource('encode-failure-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source, releaseCanvas: true })
    const renderer = createRendererMock({
      readPixels: vi.fn(() => makeImageData([10, 20, 30, 255])),
    })
    // Output canvas toBlob returns null
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      if (tagName.toLowerCase() !== 'canvas') {
        return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
      }
      return createTestCanvas(makeImageData([0, 0, 0, 255]), { toBlobReturnsNull: true }).canvas
    })
    const backend = createMainThreadImageProcessingBackend({
      sourceCache,
      rendererFactory: () => renderer,
    })

    await expect(
      backend.renderFinal({
        source: { url: 'https://example.com/source.png', title: 'Encode Failure.png' },
        plan: createChannelLutPlan(),
        context: createTaskContext({ outputPolicy: createImageProcessingOutputPolicy() }),
      }),
    ).rejects.toThrow('Canvas toBlob returned null')

    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
    expect(renderer.dispose).toHaveBeenCalled()
  })

  it('releases the full-size source canvas when final apply is cancelled', async () => {
    const source = createImageSource('cancelled-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source, releaseCanvas: true })
    const controller = new AbortController()
    const renderer = createRendererMock({
      render: vi.fn(() => {
        controller.abort()
      }),
    })
    installCanvasMocks()
    const backend = createMainThreadImageProcessingBackend({
      sourceCache,
      rendererFactory: () => renderer,
    })

    await expect(
      backend.renderFinal({
        source: { url: 'https://example.com/source.png', title: 'Cancelled.png' },
        plan: createChannelLutPlan(),
        context: createTaskContext({
          signal: controller.signal,
          outputPolicy: createImageProcessingOutputPolicy(),
        }),
      }),
    ).rejects.toThrow('Image processing cancelled')

    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
    expect(renderer.dispose).toHaveBeenCalled()
  })

  it('throws unsupported-canvas when WebGL renderer cannot be created', async () => {
    const source = createImageSource('final-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    installCanvasMocks()
    const backend = createMainThreadImageProcessingBackend({
      sourceCache,
      rendererFactory: () => null,
    })

    await expect(
      backend.renderFinal({
        source: { url: 'https://example.com/source.png' },
        plan: createChannelLutPlan(),
        context: createTaskContext(),
      }),
    ).rejects.toMatchObject({ code: 'unsupported-canvas' })
    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
  })

  it('renders preview output through WebGL renderer and returns the decoded source', async () => {
    const source = createImageSource('preview-source', [10, 20, 30, 255])
    const sourceCache = createSourceCache({ fullSize: source })
    const adjusted = makeImageData([20, 30, 40, 255])
    const renderer = createRendererMock({
      readPixels: vi.fn(() => adjusted),
    })
    installCanvasMocks()
    const backend = createMainThreadImageProcessingBackend({
      sourceCache,
      rendererFactory: () => renderer,
    })

    const result = await backend.renderPreview({
      source: { url: 'https://example.com/source.png' },
      plan: createChannelLutPlan(),
      sizeStrategy: { maxSide: 960 },
      context: createTaskContext(),
    })

    expect(sourceCache.getFullSizeSource).toHaveBeenCalledWith('https://example.com/source.png', {
      maxSide: 960,
    })
    expect(renderer.updateSourceImage).toHaveBeenCalledWith(source.canvas, 1, 1)
    expect(renderer.updateUniforms).toHaveBeenCalled()
    expect(renderer.render).toHaveBeenCalled()
    expect(result).toMatchObject({
      sourceImageData: source.imageData,
      previewImageData: adjusted,
      width: 1,
      height: 1,
    })
    expect(renderer.dispose).toHaveBeenCalled()
    expect(sourceCache.releaseSource).toHaveBeenCalledWith(source)
  })

  it('disposes the source cache', () => {
    const sourceCache = createSourceCache({})
    const backend = createMainThreadImageProcessingBackend({ sourceCache })

    expect(backend.dispose).toBeDefined()
    backend.dispose?.()

    expect(sourceCache.dispose).toHaveBeenCalledTimes(1)
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

function createRendererMock(overrides: Record<string, unknown> = {}) {
  return {
    updateSourceImage: vi.fn(),
    updateUniforms: vi.fn(),
    render: vi.fn(),
    readPixels: vi.fn(() => null),
    dispose: vi.fn(),
    isContextLost: vi.fn(() => false),
    ...overrides,
  }
}

async function renderFinalWithAdjustedPixels(input: {
  adjustedPixels: number[]
  canvas?: TestCanvasOptions
  source: { url: string; title: string }
}) {
  createdCanvasContexts.length = 0
  vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') {
      return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
    }
    return createTestCanvas(makeImageData([0, 0, 0, 255]), input.canvas).canvas
  })
  const source = createImageSource('output-policy-source', input.adjustedPixels, {
    canvas: input.canvas,
  })
  const sourceCache = createSourceCache({ fullSize: source })
  const renderer = createRendererMock({
    readPixels: vi.fn(() => makeImageData(input.adjustedPixels)),
  })
  const backend = createMainThreadImageProcessingBackend({
    sourceCache,
    rendererFactory: () => renderer,
  })

  const result = await backend.renderFinal({
    source: input.source,
    plan: createChannelLutPlan(),
    context: createTaskContext({ outputPolicy: createImageProcessingOutputPolicy() }),
  })

  // Final encoding must consume the readPixels round-trip (which carries the
  // orientation-checked bytes), not the raw WebGL drawing buffer.
  expect(renderer.readPixels).toHaveBeenCalled()
  expect(createdCanvasContexts.at(-1)?.putImageData).toHaveBeenCalledWith(
    expect.objectContaining({ data: expect.any(Uint8ClampedArray) }),
    0,
    0,
  )

  return { result, source }
}

function createImageSource(
  key: string,
  values: number[],
  options: {
    mimeType?: string
    canvas?: TestCanvasOptions | undefined
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
