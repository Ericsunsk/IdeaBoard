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
import {
  detectCanvasTransparency,
  resolvePreviewMimeType,
  WEBP_PREVIEW_MIME_TYPE,
  WEBP_PREVIEW_QUALITY,
} from '../utils/displayImagePreviewEncoding'
import { canonicalizeRuntimeUploadRef, resolveAssetUrl } from '../../../shared/config/runtimeUrls'
import {
  useMediaAssetDelivery,
  type MediaAssetDeliveryTier,
} from '../../../shared/api/mediaAssetDelivery'

interface PreviewOptions {
  maxSide?: MaybeRefOrGetter<number | null | undefined>
  mediaAssetId?: MaybeRefOrGetter<number | null | undefined>
  isVisible?: MaybeRefOrGetter<boolean>
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

async function createPreviewObjectUrl(sourceUrl: string, maxSide: number): Promise<string | null> {
  const fetchUrl = resolveFetchUrl(sourceUrl)
  if (!fetchUrl) return null
  const response = await fetch(fetchUrl, { credentials: 'include' })
  if (!response) return null
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
  const isVisible = computed(() => toValue(options.isVisible) !== false)
  const resolvedMediaTier = computed<MediaAssetDeliveryTier>(() => {
    if (resolvedMaxSide.value === null) return 'original'
    if (resolvedMaxSide.value <= 256) return 'thumb'
    if (resolvedMaxSide.value <= 640) return 'preview'
    return 'large-preview'
  })
  const mediaDelivery = useMediaAssetDelivery(resolvedMediaAssetId, resolvedMediaTier, {
    enabled: isVisible,
  })
  const previewUrl = ref<string | null>(null)
  const isResolved = ref(false)
  let requestToken = 0
  let activeSource = ''

  watch(
    [
      () => resolvedSourceUrl.value,
      () => resolvedMaxSide.value,
      () => resolvedMediaAssetId.value,
      isVisible,
    ],
    ([currentSource, maxSide, mediaAssetId, visible]) => {
      requestToken += 1
      const token = requestToken
      if (currentSource !== activeSource) previewUrl.value = null
      activeSource = currentSource
      isResolved.value = false

      if (!currentSource) return
      if (mediaAssetId !== null && !visible) return
      if (maxSide === null) {
        previewUrl.value = null
        return
      }

      if (mediaAssetId !== null) return

      const cacheKey = buildPreviewCacheKey(currentSource, maxSide)
      const cached = getCachedPreviewUrl(cacheKey)
      if (cached) {
        previewUrl.value = cached
        isResolved.value = true
        return
      }

      void getOrCreatePreviewUrl(currentSource, maxSide).then((generatedPreviewUrl) => {
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
    if (resolvedMediaAssetId.value !== null) {
      if (!isVisible.value) return ''
      if (mediaDelivery.url.value) return mediaDelivery.url.value
      return ''
    }
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
