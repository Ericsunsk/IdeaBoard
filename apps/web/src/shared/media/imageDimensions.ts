export type ImageNaturalSize = {
  width: number
  height: number
}

const IMAGE_DIMENSION_TIMEOUT_MS = 1800
const IMAGE_DIMENSION_CACHE_LIMIT = 120

const imageDimensionCache = new Map<string, ImageNaturalSize>()
const imageDimensionTasks = new Map<string, Promise<ImageNaturalSize | null>>()

export function normalizeNaturalSize(width: number, height: number): ImageNaturalSize | null {
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null
  const roundedWidth = Math.round(width)
  const roundedHeight = Math.round(height)
  if (roundedWidth <= 0 || roundedHeight <= 0) return null
  return { width: roundedWidth, height: roundedHeight }
}

function rememberImageDimension(url: string, size: ImageNaturalSize) {
  if (imageDimensionCache.has(url)) imageDimensionCache.delete(url)
  imageDimensionCache.set(url, size)
  while (imageDimensionCache.size > IMAGE_DIMENSION_CACHE_LIMIT) {
    const oldestKey = imageDimensionCache.keys().next().value
    if (!oldestKey) break
    imageDimensionCache.delete(oldestKey)
  }
}

/** LRU-cached natural-size probe for remote URLs. Resolves `null` on timeout/error. */
export function readImageNaturalSize(
  url: string,
  timeoutMs = IMAGE_DIMENSION_TIMEOUT_MS,
): Promise<ImageNaturalSize | null> {
  const cached = imageDimensionCache.get(url)
  if (cached) return Promise.resolve(cached)
  const inFlight = imageDimensionTasks.get(url)
  if (inFlight) return inFlight

  const task = new Promise<ImageNaturalSize | null>((resolve) => {
    const image = new Image()
    let settled = false
    let timeoutId = 0
    function finish(size: ImageNaturalSize | null) {
      if (settled) return
      settled = true
      window.clearTimeout(timeoutId)
      image.onload = null
      image.onerror = null
      if (size) rememberImageDimension(url, size)
      resolve(size)
    }
    timeoutId = window.setTimeout(() => finish(null), timeoutMs)
    image.onload = () => finish(normalizeNaturalSize(image.naturalWidth, image.naturalHeight))
    image.onerror = () => finish(null)
    image.src = url
    if (image.complete) {
      finish(normalizeNaturalSize(image.naturalWidth, image.naturalHeight))
    }
  }).finally(() => {
    imageDimensionTasks.delete(url)
  })

  imageDimensionTasks.set(url, task)
  return task
}

/** Test seam: clears the module-level LRU + in-flight maps. */
export function clearImageDimensionCacheForTesting() {
  imageDimensionCache.clear()
  imageDimensionTasks.clear()
}
