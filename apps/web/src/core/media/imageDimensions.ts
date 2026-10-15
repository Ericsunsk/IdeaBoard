import { scheduleMediaRead } from './mediaReadScheduler'

export type ImageNaturalSize = {
  width: number
  height: number
}

const IMAGE_DIMENSION_TIMEOUT_MS = 1800
const IMAGE_DIMENSION_CACHE_LIMIT = 120

const imageDimensionCache = new Map<string, ImageNaturalSize>()
type ImageDimensionTask = {
  promise: Promise<ImageNaturalSize | null>
  controller: AbortController
  consumers: number
}
const imageDimensionTasks = new Map<string, ImageDimensionTask>()

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
  signal?: AbortSignal,
): Promise<ImageNaturalSize | null> {
  if (signal?.aborted) return Promise.resolve(null)
  const cached = imageDimensionCache.get(url)
  if (cached) return Promise.resolve(cached)
  const inFlight = imageDimensionTasks.get(url)
  if (inFlight && !inFlight.controller.signal.aborted)
    return joinImageDimensionTask(inFlight, signal)

  const controller = new AbortController()
  const task = scheduleMediaRead(
    controller.signal,
    () =>
      new Promise<ImageNaturalSize | null>((resolve) => {
        const image = new Image()
        let settled = false
        let timeoutId = 0
        function finish(size: ImageNaturalSize | null) {
          if (settled) return
          settled = true
          window.clearTimeout(timeoutId)
          image.onload = null
          image.onerror = null
          controller.signal.removeEventListener('abort', onAbort)
          if (!size) image.src = ''
          if (size) rememberImageDimension(url, size)
          resolve(size)
        }
        const onAbort = () => finish(null)
        controller.signal.addEventListener('abort', onAbort, { once: true })
        timeoutId = window.setTimeout(() => finish(null), timeoutMs)
        image.onload = () => finish(normalizeNaturalSize(image.naturalWidth, image.naturalHeight))
        image.onerror = () => finish(null)
        image.src = url
        if (image.complete) {
          finish(normalizeNaturalSize(image.naturalWidth, image.naturalHeight))
        }
      }),
  )
    .catch(() => null)
    .finally(() => {
      if (imageDimensionTasks.get(url)?.promise === task) imageDimensionTasks.delete(url)
    })

  const entry = { promise: task, controller, consumers: 0 }
  imageDimensionTasks.set(url, entry)
  return joinImageDimensionTask(entry, signal)
}

function joinImageDimensionTask(
  task: ImageDimensionTask,
  signal?: AbortSignal,
): Promise<ImageNaturalSize | null> {
  task.consumers += 1
  return new Promise((resolve) => {
    let isSettled = false
    const finish = (value: ImageNaturalSize | null) => {
      if (isSettled) return
      isSettled = true
      signal?.removeEventListener('abort', onAbort)
      task.consumers -= 1
      if (task.consumers === 0) task.controller.abort()
      resolve(value)
    }
    const onAbort = () => finish(null)
    signal?.addEventListener('abort', onAbort, { once: true })
    task.promise.then(finish)
  })
}

export function invalidateImageNaturalSize(url: string): void {
  imageDimensionCache.delete(url)
}

/** Test seam: clears the module-level LRU + in-flight maps. */
export function clearImageDimensionCacheForTesting() {
  imageDimensionCache.clear()
  for (const task of imageDimensionTasks.values()) task.controller.abort()
  imageDimensionTasks.clear()
}
