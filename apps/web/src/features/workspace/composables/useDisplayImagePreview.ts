import type { MaybeRefOrGetter } from 'vue'
import { resolveFetchUrl } from '../services/flowMediaIO'

interface PreviewOptions {
  maxSide?: number
}

const DEFAULT_MAX_SIDE = 1280
const PREVIEW_CACHE_LIMIT = 80
const PREVIEW_CONCURRENCY = 2

const previewCache = new Map<string, string>()
const inFlightPreviewTasks = new Map<string, Promise<string | null>>()
const pendingPreviewTasks: Array<() => void> = []
let activePreviewTasks = 0

function buildPreviewCacheKey(sourceUrl: string, maxSide: number) {
  return `${maxSide}::${sourceUrl}`
}

function touchCache(cacheKey: string, previewObjectUrl: string) {
  if (previewCache.has(cacheKey)) previewCache.delete(cacheKey)
  previewCache.set(cacheKey, previewObjectUrl)
  trimPreviewCache()
}

function trimPreviewCache() {
  while (previewCache.size > PREVIEW_CACHE_LIMIT) {
    const oldestEntry = previewCache.entries().next().value as [string, string] | undefined
    if (!oldestEntry) break
    previewCache.delete(oldestEntry[0])
    URL.revokeObjectURL(oldestEntry[1])
  }
}

function getCachedPreviewUrl(cacheKey: string): string | null {
  const cached = previewCache.get(cacheKey)
  if (!cached) return null
  touchCache(cacheKey, cached)
  return cached
}

function runNextPreviewTask() {
  while (activePreviewTasks < PREVIEW_CONCURRENCY && pendingPreviewTasks.length > 0) {
    const task = pendingPreviewTasks.shift()
    if (!task) break
    task()
  }
}

function enqueuePreviewTask<T>(taskFactory: () => Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const run = () => {
      activePreviewTasks += 1
      void taskFactory()
        .then(resolve)
        .catch(reject)
        .finally(() => {
          activePreviewTasks = Math.max(0, activePreviewTasks - 1)
          runNextPreviewTask()
        })
    }
    pendingPreviewTasks.push(run)
    runNextPreviewTask()
  })
}

async function createPreviewObjectUrl(sourceUrl: string, maxSide: number): Promise<string | null> {
  const fetchUrl = resolveFetchUrl(sourceUrl)
  if (!fetchUrl) return null
  const response = await fetch(fetchUrl, { credentials: 'include' })
  if (!response.ok) return null
  const sourceBlob = await response.blob()
  if (!sourceBlob.type.startsWith('image/')) return null

  const bitmap = await createImageBitmap(sourceBlob)
  const longestSide = Math.max(bitmap.width, bitmap.height)
  if (longestSide <= 0 || longestSide <= maxSide) {
    bitmap.close()
    return null
  }

  const scale = maxSide / longestSide
  const width = Math.max(1, Math.round(bitmap.width * scale))
  const height = Math.max(1, Math.round(bitmap.height * scale))
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height

  const ctx = canvas.getContext('2d')
  if (!ctx) {
    bitmap.close()
    return null
  }

  ctx.drawImage(bitmap, 0, 0, width, height)
  bitmap.close()

  const previewBlob = await new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, 'image/webp', 0.82)
  })

  if (!previewBlob) return null
  return URL.createObjectURL(previewBlob)
}

async function getOrCreatePreviewUrl(sourceUrl: string, maxSide: number): Promise<string | null> {
  const cacheKey = buildPreviewCacheKey(sourceUrl, maxSide)
  const cached = getCachedPreviewUrl(cacheKey)
  if (cached) return cached

  const inFlight = inFlightPreviewTasks.get(cacheKey)
  if (inFlight) return inFlight

  const task = enqueuePreviewTask(() => createPreviewObjectUrl(sourceUrl, maxSide))
    .then((previewUrl) => {
      if (previewUrl) touchCache(cacheKey, previewUrl)
      return previewUrl
    })
    .catch(() => null)
    .finally(() => {
      inFlightPreviewTasks.delete(cacheKey)
    })

  inFlightPreviewTasks.set(cacheKey, task)
  return task
}

export function useDisplayImagePreview(
  sourceUrl: MaybeRefOrGetter<string | null | undefined>,
  options: PreviewOptions = {},
) {
  const maxSide = options.maxSide ?? DEFAULT_MAX_SIDE
  const previewUrl = ref<string | null>(null)
  let requestToken = 0

  watch(
    () => toValue(sourceUrl),
    (currentSource) => {
      requestToken += 1
      const token = requestToken
      previewUrl.value = null

      if (!currentSource) return

      const cacheKey = buildPreviewCacheKey(currentSource, maxSide)
      const cached = getCachedPreviewUrl(cacheKey)
      if (cached) {
        previewUrl.value = cached
        return
      }

      void getOrCreatePreviewUrl(currentSource, maxSide).then((generatedPreviewUrl) => {
        if (token !== requestToken) return
        previewUrl.value = generatedPreviewUrl
      })
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    requestToken += 1
  })

  const displayUrl = computed(() => previewUrl.value || toValue(sourceUrl) || '')

  return {
    displayUrl,
  }
}
