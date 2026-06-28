import { resolveFetchUrl } from '../../services/flowMediaIO'
import { isAnimatedImageBlob, releaseCanvas } from '../utils/imageAdjustmentMedia'

export const DEFAULT_IMAGE_SOURCE_CACHE_LIMIT = 12
export const DEFAULT_PREVIEW_SOURCE_MAX_SIDE = 960
export const DEFAULT_FULL_SIZE_SOURCE_MAX_PIXELS = 24_000_000

export type ImageSourcePurpose = 'preview' | 'full-size'
export type ImageSourceFailureMode = 'throw' | 'return-null'
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
  readonly purpose: ImageSourcePurpose
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

export type ImageSourcePreviewOptions = {
  readonly maxSide?: number
  readonly failureMode?: ImageSourceFailureMode
  readonly resolveOptions?: ImageSourceResolveOptions
}

export type ImageSourceFullSizeOptions = {
  readonly maxPixels?: number
  readonly failureMode?: ImageSourceFailureMode
  readonly resolveOptions?: ImageSourceResolveOptions
}

type LoadImageSourceOptions = {
  readonly purpose: ImageSourcePurpose
  readonly maxSide?: number
  readonly maxPixels?: number
  readonly failureMode: ImageSourceFailureMode
  readonly resolveOptions?: ImageSourceResolveOptions
}

export class ImageSourceCacheError extends Error {
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
    this.maxEntries = Math.max(0, Math.floor(options.maxEntries ?? DEFAULT_IMAGE_SOURCE_CACHE_LIMIT))
  }

  get size() {
    return this.entries.size
  }

  getPreviewSource(
    sourceUrl: string,
    options: ImageSourcePreviewOptions = {},
  ): Promise<ImageSourceCacheEntry | null> {
    const maxSide = Math.max(1, Math.floor(options.maxSide ?? DEFAULT_PREVIEW_SOURCE_MAX_SIDE))
    return this.getOrLoad(sourceUrl, {
      purpose: 'preview',
      maxSide,
      failureMode: options.failureMode ?? 'return-null',
      resolveOptions: options.resolveOptions,
    })
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
      purpose: 'full-size',
      maxPixels,
      failureMode: options.failureMode ?? 'throw',
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
    if (this.isDisposed) return handleImageSourceFailure(createCacheError('disposed'), options)

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
          return this.isDisposed ? handleImageSourceFailure(createCacheError('disposed'), options) : null
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
    if (!response.ok) throw createCacheError('fetch-failed')

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
        const context = canvas.getContext('2d')
        if (!context) throw createCacheError('unsupported-canvas')
        context.drawImage(bitmap, 0, 0, width, height)
        return {
          key: options.cacheKey,
          purpose: options.purpose,
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
    return handleImageSourceFailure(normalizedError, options)
  }
}

function resolveTargetSize(bitmap: ImageBitmap, options: LoadImageSourceOptions) {
  if (bitmap.width <= 0 || bitmap.height <= 0) throw createCacheError('decode-failed')

  if (options.purpose === 'full-size') {
    const pixelCount = bitmap.width * bitmap.height
    if (pixelCount > (options.maxPixels ?? DEFAULT_FULL_SIZE_SOURCE_MAX_PIXELS)) {
      throw createCacheError('too-large')
    }
    return { width: bitmap.width, height: bitmap.height }
  }

  const longestSide = Math.max(bitmap.width, bitmap.height)
  const scale = Math.min(1, (options.maxSide ?? DEFAULT_PREVIEW_SOURCE_MAX_SIDE) / longestSide)
  return {
    width: Math.max(1, Math.round(bitmap.width * scale)),
    height: Math.max(1, Math.round(bitmap.height * scale)),
  }
}

function handleImageSourceFailure(
  error: ImageSourceCacheError,
  options: Pick<LoadImageSourceOptions, 'failureMode'>,
) {
  if (options.failureMode === 'return-null') return null
  throw error
}

function createImageSourceCacheKey(sourceUrl: string, options: LoadImageSourceOptions) {
  return [
    options.purpose,
    options.maxSide ?? 'full',
    options.maxPixels ?? 'unbounded',
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
      return '图片地址无效'
    case 'fetch-failed':
      return '读取图片失败'
    case 'not-image':
      return '读取到的文件不是图片'
    case 'animated-image':
      return '暂不支持编辑动图'
    case 'too-large':
      return '图片尺寸过大，暂不支持在浏览器内编辑'
    case 'unsupported-canvas':
      return '当前浏览器不支持图像处理'
    case 'disposed':
      return '图像源缓存已释放'
    case 'decode-failed':
      return '图片解码失败'
  }
}
