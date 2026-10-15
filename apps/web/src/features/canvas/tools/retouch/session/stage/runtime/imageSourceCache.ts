import {
  beginCanvasInteractionTiming,
  recordCanvasInteractionCount,
} from '../../../../../session/browser/canvasInteractionMetrics'
import { resolveFetchUrl } from '../../../../../session/media/canvasMediaIo.ts'
import { isAnimatedImageBlob } from '../../../core/media.ts'

/**
 * Release a canvas back to the pool: zeroing dimensions drops the bitmap
 * without removing the element. Owned by the image-source cache (runtime
 * layer); core/media only answers animated-blob questions.
 */
function releaseCanvas(canvas: HTMLCanvasElement | null) {
  if (!canvas) return
  canvas.width = 0
  canvas.height = 0
}

const DEFAULT_IMAGE_SOURCE_CACHE_LIMIT = 12
const DEFAULT_FULL_SIZE_SOURCE_MAX_PIXELS = 24_000_000

type ImageSourceCacheErrorCode =
  | 'invalid-url'
  | 'fetch-failed'
  | 'not-image'
  | 'animated-image'
  | 'decode-failed'
  | 'too-large'
  | 'unsupported-canvas'
  | 'disposed'

type ImageSourceResolveOptions = Parameters<typeof resolveFetchUrl>[1]

export type ImageSourceCacheEntry = {
  readonly key: string
  readonly sourceUrl: string
  readonly fetchUrl: string
  readonly width: number
  readonly height: number
  readonly mimeType: string
  readonly canvas: HTMLCanvasElement
  readonly context: CanvasRenderingContext2D
}

export type ImageSourceCacheOptions = {
  readonly maxEntries?: number
}

export type ImageSourceFullSizeOptions = {
  readonly maxPixels?: number | undefined
  readonly maxSide?: number | undefined
  /** Downscale sources that exceed maxPixels instead of rejecting them. */
  readonly fitWithinMaxPixels?: boolean | undefined
  readonly resolveOptions?: ImageSourceResolveOptions | undefined
}

type LoadImageSourceOptions = {
  readonly maxPixels?: number | undefined
  readonly maxSide?: number | undefined
  readonly fitWithinMaxPixels?: boolean | undefined
  readonly resolveOptions?: ImageSourceResolveOptions | undefined
}

class ImageSourceCacheError extends Error {
  readonly code: ImageSourceCacheErrorCode

  constructor(code: ImageSourceCacheErrorCode, message: string) {
    super(message)
    this.name = 'ImageSourceCacheError'
    this.code = code
  }
}

export class ImageSourceCache {
  private readonly maxEntries: number
  private readonly entries = new Map<string, ImageSourceCacheEntry>()
  private readonly inFlight = new Map<string, Promise<ImageSourceCacheEntry | null>>()
  private readonly controller = new AbortController()

  constructor(options: ImageSourceCacheOptions = {}) {
    this.maxEntries = Math.max(
      0,
      Math.floor(options.maxEntries ?? DEFAULT_IMAGE_SOURCE_CACHE_LIMIT),
    )
  }

  get size() {
    return this.entries.size
  }

  getFullSizeSource(
    sourceUrl: string,
    options: ImageSourceFullSizeOptions = {},
  ): Promise<ImageSourceCacheEntry | null> {
    const maxPixels = Math.max(
      1,
      Math.floor(options.maxPixels ?? DEFAULT_FULL_SIZE_SOURCE_MAX_PIXELS),
    )
    return this.getOrLoad(sourceUrl, {
      maxPixels,
      maxSide: options.maxSide,
      fitWithinMaxPixels: options.fitWithinMaxPixels,
      resolveOptions: options.resolveOptions,
    })
  }

  releaseSource(source: ImageSourceCacheEntry | string | null | undefined) {
    if (!source) return
    const key = typeof source === 'string' ? source : source.key
    const cached = this.entries.get(key)
    if (!cached) return
    this.entries.delete(key)
    releaseImageSourceCanvas(cached)
  }

  dispose() {
    this.controller.abort()
    for (const entry of this.entries.values()) {
      releaseImageSourceCanvas(entry)
    }
    this.entries.clear()
    this.inFlight.clear()
  }

  private async getOrLoad(
    sourceUrl: string,
    options: LoadImageSourceOptions,
  ): Promise<ImageSourceCacheEntry | null> {
    if (this.controller.signal.aborted) throw createCacheError('disposed')

    const cacheKey = createImageSourceCacheKey(sourceUrl, options)
    const cached = this.entries.get(cacheKey)
    if (cached) {
      recordCanvasInteractionCount('retouchSourceCacheHit')
      this.entries.delete(cacheKey)
      this.entries.set(cacheKey, cached)
      return cached
    }

    const inFlight = this.inFlight.get(cacheKey)
    if (inFlight) {
      recordCanvasInteractionCount('retouchSourceCacheJoin')
      return inFlight
    }

    recordCanvasInteractionCount('retouchSourceCacheMiss')
    const finishTiming = beginCanvasInteractionTiming('retouchSourceLoad')
    const task = loadImageSourceCanvas(sourceUrl, { ...options, cacheKey }, this.controller.signal)
      .then((entry) => {
        if (!entry) return null
        if (this.controller.signal.aborted || this.maxEntries === 0) {
          releaseImageSourceCanvas(entry)
          if (this.controller.signal.aborted) throw createCacheError('disposed')
          return null
        }
        this.entries.set(cacheKey, entry)
        this.trim()
        return entry
      })
      .finally(() => {
        this.inFlight.delete(cacheKey)
        finishTiming?.()
      })

    this.inFlight.set(cacheKey, task)
    return task
  }

  private trim() {
    while (this.entries.size > this.maxEntries) {
      const oldestKey = this.entries.keys().next().value as string | undefined
      if (!oldestKey) break
      const oldest = this.entries.get(oldestKey)
      this.entries.delete(oldestKey)
      recordCanvasInteractionCount('retouchSourceCacheEviction')
      releaseImageSourceCanvas(oldest ?? null)
    }
  }
}

export function createImageSourceCache(options: ImageSourceCacheOptions = {}) {
  return new ImageSourceCache(options)
}

export function readImageSourceImageData(source: ImageSourceCacheEntry): ImageData {
  return source.context.getImageData(0, 0, source.width, source.height)
}

export function releaseImageSourceCanvas(source: ImageSourceCacheEntry | null | undefined) {
  releaseCanvas(source?.canvas ?? null)
}

async function loadImageSourceCanvas(
  sourceUrl: string,
  options: LoadImageSourceOptions & { readonly cacheKey: string },
  signal: AbortSignal,
): Promise<ImageSourceCacheEntry | null> {
  try {
    const fetchUrl = resolveFetchUrl(sourceUrl, options.resolveOptions)
    if (!fetchUrl) throw createCacheError('invalid-url')

    const response = await fetch(fetchUrl, { credentials: 'include', signal })
    signal.throwIfAborted()
    if (!response.ok || response.status === 202) throw createCacheError('fetch-failed')

    const blob = await response.blob()
    signal.throwIfAborted()
    if (!blob.type.startsWith('image/')) throw createCacheError('not-image')
    if (await isAnimatedImageBlob(blob)) throw createCacheError('animated-image')
    signal.throwIfAborted()

    const bitmap = await createImageBitmap(blob)
    try {
      signal.throwIfAborted()
      const { width, height } = resolveTargetSize(bitmap, options)
      const canvas = document.createElement('canvas')
      try {
        canvas.width = width
        canvas.height = height
        const context = canvas.getContext('2d', { willReadFrequently: true })
        if (!context) throw createCacheError('unsupported-canvas')
        context.drawImage(bitmap, 0, 0, width, height)
        return {
          key: options.cacheKey,
          sourceUrl,
          fetchUrl,
          width,
          height,
          mimeType: blob.type,
          canvas,
          context,
        }
      } catch (error) {
        releaseCanvas(canvas)
        throw error
      }
    } finally {
      bitmap.close()
    }
  } catch (error) {
    if (signal.aborted) throw createCacheError('disposed')
    const normalizedError =
      error instanceof ImageSourceCacheError ? error : createCacheError('decode-failed')
    throw normalizedError
  }
}

function resolveTargetSize(bitmap: ImageBitmap, options: LoadImageSourceOptions) {
  if (bitmap.width <= 0 || bitmap.height <= 0) throw createCacheError('decode-failed')

  const maxSide = Math.max(1, Math.floor(options.maxSide ?? Number.POSITIVE_INFINITY))
  const scale = Math.min(1, maxSide / Math.max(bitmap.width, bitmap.height))
  const targetSize = {
    width: Math.max(1, Math.round(bitmap.width * scale)),
    height: Math.max(1, Math.round(bitmap.height * scale)),
  }
  const maxPixels = options.maxPixels ?? DEFAULT_FULL_SIZE_SOURCE_MAX_PIXELS
  const pixelCount = targetSize.width * targetSize.height
  if (pixelCount <= maxPixels) return targetSize
  if (!options.fitWithinMaxPixels) throw createCacheError('too-large')

  const fitScale = Math.sqrt(maxPixels / pixelCount)
  return {
    width: Math.max(1, Math.floor(targetSize.width * fitScale)),
    height: Math.max(1, Math.floor(targetSize.height * fitScale)),
  }
}

function createImageSourceCacheKey(sourceUrl: string, options: LoadImageSourceOptions) {
  return [
    options.maxPixels ?? 'unbounded',
    options.maxSide ?? 'full-side',
    options.fitWithinMaxPixels ?? false,
    options.resolveOptions?.shouldAllowSameOriginUploads ?? 'default-uploads',
    options.resolveOptions?.shouldBlockExternal ?? 'default-external',
    sourceUrl,
  ].join('::')
}

function createCacheError(code: ImageSourceCacheErrorCode) {
  // Code is the fact; localized copy resolves at the commands/Chrome layer
  // via the existing canvas.retouch.errors.* keys. Message carries the code
  // so thrown errors stay locale-free.
  return new ImageSourceCacheError(code, code)
}
