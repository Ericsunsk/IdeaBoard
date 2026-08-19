import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue'
import { canvasToBlob, resolveFetchUrl } from '../services/flowMediaIO'
import { IMAGE_PREVIEW_ENDPOINT, MEDIA_ASSET_ENDPOINT } from '../../../shared/constants/media'
import {
  detectCanvasTransparency,
  resolvePreviewMimeType,
  WEBP_PREVIEW_MIME_TYPE,
  WEBP_PREVIEW_QUALITY,
} from '../utils/displayImagePreviewEncoding'
import { canonicalizeRuntimeUploadRef, resolveAssetUrl } from '../../../shared/config/runtimeUrls'

interface PreviewOptions {
  maxSide?: MaybeRefOrGetter<number | null | undefined>
  mediaAssetId?: MaybeRefOrGetter<number | null | undefined>
}

const DEFAULT_MAX_SIDE = 1280
const PREVIEW_CACHE_LIMIT = 80
const PREVIEW_CONCURRENCY = 2
const MEDIA_DERIVATIVE_RETRY_LIMIT = 20
const MEDIA_DERIVATIVE_RETRY_DELAY_MS = 750

const previewCache = new Map<string, string>()
const inFlightPreviewTasks = new Map<string, Promise<string | null>>()
const pendingPreviewTasks: Array<() => void> = []
let activePreviewTasks = 0

function buildPreviewCacheKey(sourceUrl: string, maxSide: number, mediaAssetId: number | null) {
  return `${maxSide}::${mediaAssetId ?? ''}::${sourceUrl}`
}

function resolvePreviewMaxSide(value: unknown): number | null {
  // null is the explicit full-quality tier: keep the original URL and skip canvas work.
  if (value === null) return null
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? Math.floor(value)
    : DEFAULT_MAX_SIDE
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

function resolveDerivativeTier(maxSide: number): string {
  if (maxSide <= 256) return 'thumb'
  if (maxSide <= 640) return 'preview'
  return 'large-preview'
}

async function fetchMediaDerivative(fetchUrl: string): Promise<Response | null> {
  for (let attempt = 0; attempt <= MEDIA_DERIVATIVE_RETRY_LIMIT; attempt += 1) {
    const response = await fetch(fetchUrl, { credentials: 'include' })
    if (response.ok) return response
    if (response.status !== 202 || attempt === MEDIA_DERIVATIVE_RETRY_LIMIT) return null
    await new Promise((resolve) => window.setTimeout(resolve, MEDIA_DERIVATIVE_RETRY_DELAY_MS))
  }
  return null
}

async function createPreviewObjectUrl(
  sourceUrl: string,
  maxSide: number,
  mediaAssetId: number | null,
): Promise<string | null> {
  const canonicalSourceUrl = canonicalizeRuntimeUploadRef(sourceUrl.trim()).trim()
  const serverPreviewUrl = mediaAssetId
    ? `${MEDIA_ASSET_ENDPOINT}/${encodeURIComponent(String(mediaAssetId))}/derivatives/${resolveDerivativeTier(maxSide)}/content`
    : canonicalSourceUrl.startsWith('/uploads/')
      ? `${IMAGE_PREVIEW_ENDPOINT}?url=${encodeURIComponent(canonicalSourceUrl)}&max_side=${maxSide}`
      : null
  const fetchUrl = serverPreviewUrl ?? resolveFetchUrl(sourceUrl)
  if (!fetchUrl) return null
  const response = mediaAssetId
    ? await fetchMediaDerivative(fetchUrl)
    : await fetch(fetchUrl, { credentials: 'include' })
  if (!response) return null
  if (!response.ok) return null
  const sourceBlob = await response.blob()
  if (!sourceBlob.type.startsWith('image/')) return null

  if (serverPreviewUrl) return URL.createObjectURL(sourceBlob)

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

  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) {
    bitmap.close()
    return null
  }

  ctx.drawImage(bitmap, 0, 0, width, height)
  bitmap.close()

  const hasTransparentPixels = detectCanvasTransparency(ctx, width, height)
  const previewMime = resolvePreviewMimeType(sourceBlob.type, hasTransparentPixels)
  const previewBlob = await canvasToBlob(
    canvas,
    previewMime,
    previewMime === WEBP_PREVIEW_MIME_TYPE ? WEBP_PREVIEW_QUALITY : undefined,
  )
  return URL.createObjectURL(previewBlob)
}

async function getOrCreatePreviewUrl(
  sourceUrl: string,
  maxSide: number,
  mediaAssetId: number | null,
): Promise<string | null> {
  const cacheKey = buildPreviewCacheKey(sourceUrl, maxSide, mediaAssetId)
  const cached = getCachedPreviewUrl(cacheKey)
  if (cached) return cached

  const inFlight = inFlightPreviewTasks.get(cacheKey)
  if (inFlight) return inFlight

  const task = enqueuePreviewTask(() => createPreviewObjectUrl(sourceUrl, maxSide, mediaAssetId))
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
  const canonicalSourceUrl = computed(() => {
    const raw = toValue(sourceUrl)
    return raw ? canonicalizeRuntimeUploadRef(raw.trim()).trim() : ''
  })
  const isManagedUpload = computed(() => canonicalSourceUrl.value.startsWith('/uploads/'))
  const resolvedSourceUrl = computed(() => {
    return canonicalSourceUrl.value ? resolveAssetUrl(canonicalSourceUrl.value) : ''
  })
  const resolvedMaxSide = computed(() => resolvePreviewMaxSide(toValue(options.maxSide)))
  const resolvedMediaAssetId = computed(() => {
    const raw = toValue(options.mediaAssetId)
    return typeof raw === 'number' && Number.isInteger(raw) && raw > 0 ? raw : null
  })
  const previewUrl = ref<string | null>(null)
  const isResolved = ref(false)
  let requestToken = 0
  let activeSource = ''

  watch(
    [() => resolvedSourceUrl.value, () => resolvedMaxSide.value, () => resolvedMediaAssetId.value],
    ([currentSource, maxSide, mediaAssetId]) => {
      requestToken += 1
      const token = requestToken
      if (currentSource !== activeSource) previewUrl.value = null
      activeSource = currentSource
      isResolved.value = false

      if (!currentSource) return
      if (maxSide === null) {
        previewUrl.value = null
        return
      }

      const cacheKey = buildPreviewCacheKey(currentSource, maxSide, mediaAssetId)
      const cached = getCachedPreviewUrl(cacheKey)
      if (cached) {
        previewUrl.value = cached
        isResolved.value = true
        return
      }

      void getOrCreatePreviewUrl(currentSource, maxSide, mediaAssetId).then((generatedPreviewUrl) => {
        if (token !== requestToken) return
        previewUrl.value = generatedPreviewUrl
        isResolved.value = true
      })
    },
    { immediate: true },
  )

  if (getCurrentScope()) {
    onScopeDispose(() => {
      requestToken += 1
    })
  }

  const displayUrl = computed(() => {
    if (previewUrl.value) return previewUrl.value
    if (
      isManagedUpload.value &&
      resolvedMaxSide.value !== null &&
      (resolvedMediaAssetId.value !== null || !isResolved.value)
    ) {
      return ''
    }
    return resolvedSourceUrl.value
  })

  return {
    displayUrl,
  }
}
