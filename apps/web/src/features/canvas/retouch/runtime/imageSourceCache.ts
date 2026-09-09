import { i18n } from '../../../../shared/i18n'
import { resolveFetchUrl } from '../../services/flowMediaIO'
import { isAnimatedImageBlob, releaseCanvas } from '../utils/retouchMedia'

const DEFAULT_IMAGE_SOURCE_CACHE_LIMIT = 12
const DEFAULT_FULL_SIZE_SOURCE_MAX_PIXELS = 24_000_000

export type ImageSourceCacheErrorCode =
  | 'invalid-url'
  | 'fetch-failed'
  | 'not-image'
  | 'animated-image'
  | 'decode-failed'
  | 'too-large'
  | 'unsupported-canvas'
  | 'disposed'

export type ImageSourceResolveOptions = Parameters<typeof resolveFetchUrl>[1]

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
  private isDisposed = false

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
    this.isDisposed = true
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
    if (this.isDisposed) throw createCacheError('disposed')

    const cacheKey = createImageSourceCacheKey(sourceUrl, options)
    const cached = this.entries.get(cacheKey)
    if (cached) {
      this.entries.delete(cacheKey)
      this.entries.set(cacheKey, cached)
      return cached
    }

    const inFlight = this.inFlight.get(cacheKey)
    if (inFlight) return inFlight

    const task = loadImageSourceCanvas(sourceUrl, { ...options, cacheKey })
      .then((entry) => {
        if (!entry) return null
        if (this.isDisposed || this.maxEntries === 0) {
          releaseImageSourceCanvas(entry)
          if (this.isDisposed) throw createCacheError('disposed')
          return null
        }
        this.entries.set(cacheKey, entry)
        this.trim()
        return entry
      })
      .finally(() => {
        this.inFlight.delete(cacheKey)
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
): Promise<ImageSourceCacheEntry | null> {
  try {
    const fetchUrl = resolveFetchUrl(sourceUrl, options.resolveOptions)
    if (!fetchUrl) throw createCacheError('invalid-url')

    const response = await fetch(fetchUrl, { credentials: 'include' })
    if (!response.ok || response.status === 202) throw createCacheError('fetch-failed')

    const blob = await response.blob()
    if (!blob.type.startsWith('image/')) throw createCacheError('not-image')
    if (await isAnimatedImageBlob(blob)) throw createCacheError('animated-image')

    const bitmap = await createImageBitmap(blob)
    try {
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
    options.resolveOptions?.allowSameOriginUploads ?? 'default-uploads',
    options.resolveOptions?.blockExternal ?? 'default-external',
    sourceUrl,
  ].join('::')
}

function createCacheError(code: ImageSourceCacheErrorCode) {
  return new ImageSourceCacheError(code, resolveCacheErrorMessage(code))
}

function resolveCacheErrorMessage(code: ImageSourceCacheErrorCode) {
  switch (code) {
    case 'invalid-url':
      return i18n.global.t('canvas.retouch.errors.invalidImageUrl')
    case 'fetch-failed':
      return i18n.global.t('canvas.retouch.errors.readImageFailed')
    case 'not-image':
      return i18n.global.t('canvas.retouch.errors.notAnImage')
    case 'animated-image':
      return i18n.global.t('canvas.retouch.errors.animatedUnsupported')
    case 'too-large':
      return i18n.global.t('canvas.retouch.errors.tooLargeToEdit')
    case 'unsupported-canvas':
      return i18n.global.t('canvas.retouch.errors.unsupportedCanvas')
    case 'disposed':
      return i18n.global.t('canvas.retouch.errors.cacheDisposed')
    case 'decode-failed':
      return i18n.global.t('canvas.retouch.errors.decodeFailed')
  }
}
