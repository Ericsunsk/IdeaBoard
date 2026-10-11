import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue'
import {
  canvasToBlob,
  fetchImageBlobFromUrl,
  resolveFetchUrl,
} from '../../media/canvasMediaIo.ts'
import {
  detectCanvasTransparency,
  resolvePreviewMimeType,
  WEBP_PREVIEW_MIME_TYPE,
  WEBP_PREVIEW_QUALITY,
} from '../../media/displayImagePreviewEncoding.ts'
import { canonicalizeRuntimeUploadRef, resolveAssetUrl } from '../../../../../core/config/runtimeUrls'
import {
  normalizeMediaAssetId,
  useMediaAssetDelivery,
  type MediaAssetDeliveryTier,
  type MediaAssetId,
} from '../../../../../api/mediaAssetDelivery'

interface PreviewOptions {
  maxSide?: MaybeRefOrGetter<number | null | undefined>
  assetId?: MaybeRefOrGetter<MediaAssetId | null | undefined>
  isVisible?: MaybeRefOrGetter<boolean>
  keepPreviousOnSourceChange?: boolean
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

// Object URLs currently committed to a live `<img>` (or awaiting decode in
// the display gate) must survive LRU eviction: revoking a URL out from under
// a mid-load image breaks the render with no re-trigger. Pinned entries are
// skipped as eviction victims; unpinned on replace/dispose.
const pinnedPreviewUrls = new Set<string>()

function pinPreviewUrl(previewObjectUrl: string) {
  if (previewObjectUrl) pinnedPreviewUrls.add(previewObjectUrl)
}

function unpinPreviewUrl(previewObjectUrl: string) {
  if (previewObjectUrl) pinnedPreviewUrls.delete(previewObjectUrl)
}

function trimPreviewCache() {
  while (previewCache.size > PREVIEW_CACHE_LIMIT) {
    let victim: [string, string] | undefined
    for (const entry of previewCache.entries()) {
      if (!pinnedPreviewUrls.has(entry[1])) {
        victim = entry
        break
      }
    }
    if (!victim) break
    previewCache.delete(victim[0])
    URL.revokeObjectURL(victim[1])
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

function enqueuePreviewTask<T>(taskFactory: () => Promise<T>, signal?: AbortSignal): Promise<T> {
  if (signal?.aborted) return Promise.reject(new DOMException('Aborted', 'AbortError'))
  return new Promise<T>((resolve, reject) => {
    const cleanup = () => signal?.removeEventListener('abort', handleAbort)
    const wrappedResolve = (value: T) => {
      cleanup()
      resolve(value)
    }
    const wrappedReject = (reason?: unknown) => {
      cleanup()
      reject(reason)
    }
    const run = () => {
      if (signal?.aborted) {
        wrappedReject(new DOMException('Aborted', 'AbortError'))
        return
      }
      activePreviewTasks += 1
      void taskFactory()
        .then(wrappedResolve)
        .catch(wrappedReject)
        .finally(() => {
          activePreviewTasks = Math.max(0, activePreviewTasks - 1)
          runNextPreviewTask()
        })
    }
    const handleAbort = () => {
      const pendingIndex = pendingPreviewTasks.indexOf(run)
      if (pendingIndex >= 0) {
        pendingPreviewTasks.splice(pendingIndex, 1)
        wrappedReject(new DOMException('Aborted', 'AbortError'))
      }
    }
    if (signal) {
      signal.addEventListener('abort', handleAbort, { once: true })
    }
    pendingPreviewTasks.push(run)
    runNextPreviewTask()
  })
}

async function fetchPreviewSourceBlob(
  sourceUrl: string,
  signal?: AbortSignal,
): Promise<Blob | null> {
  if (signal?.aborted) return null
  const fetchUrl = resolveFetchUrl(sourceUrl)
  if (!fetchUrl) return null
  try {
    const blob = await fetchImageBlobFromUrl(fetchUrl, signal ? { signal } : undefined)
    if (signal?.aborted) return null
    return blob
  } catch {
    return null
  }
}

function resolvePreviewRenderSize(
  bitmap: ImageBitmap,
  maxSide: number,
): { width: number; height: number } | null {
  const longestSide = Math.max(bitmap.width, bitmap.height)
  if (longestSide <= 0 || longestSide <= maxSide) return null
  const scale = maxSide / longestSide
  return {
    width: Math.max(1, Math.round(bitmap.width * scale)),
    height: Math.max(1, Math.round(bitmap.height * scale)),
  }
}

async function renderPreviewObjectUrl(
  sourceBlob: Blob,
  maxSide: number,
  signal?: AbortSignal,
): Promise<string | null> {
  let bitmap: ImageBitmap | null = null
  try {
    try {
      bitmap = await createImageBitmap(sourceBlob)
    } catch {
      return null
    }
    if (signal?.aborted) return null
    const size = resolvePreviewRenderSize(bitmap, maxSide)
    if (!size) return null
    const canvas = document.createElement('canvas')
    canvas.width = size.width
    canvas.height = size.height
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) return null
    ctx.drawImage(bitmap, 0, 0, size.width, size.height)
    bitmap.close()
    bitmap = null
    if (signal?.aborted) return null
    const hasTransparentPixels = detectCanvasTransparency(ctx, size.width, size.height)
    const previewMime = resolvePreviewMimeType(sourceBlob.type, hasTransparentPixels)
    const previewBlob = await canvasToBlob(
      canvas,
      previewMime,
      previewMime === WEBP_PREVIEW_MIME_TYPE ? WEBP_PREVIEW_QUALITY : undefined,
    )
    if (signal?.aborted) return null
    const objectUrl = URL.createObjectURL(previewBlob)
    if (signal?.aborted) {
      try {
        URL.revokeObjectURL(objectUrl)
      } catch {
        // ignore cleanup failures
      }
      return null
    }
    return objectUrl
  } finally {
    bitmap?.close()
  }
}

async function createPreviewObjectUrl(
  sourceUrl: string,
  maxSide: number,
  signal?: AbortSignal,
): Promise<string | null> {
  if (signal?.aborted) return null
  const sourceBlob = await fetchPreviewSourceBlob(sourceUrl, signal)
  if (!sourceBlob || signal?.aborted) return null
  if (!sourceBlob.type.startsWith('image/')) return null
  return renderPreviewObjectUrl(sourceBlob, maxSide, signal)
}

async function getOrCreatePreviewUrl(
  sourceUrl: string,
  maxSide: number,
  signal?: AbortSignal,
): Promise<string | null> {
  if (signal?.aborted) return null
  const cacheKey = buildPreviewCacheKey(sourceUrl, maxSide)
  const cached = getCachedPreviewUrl(cacheKey)
  if (cached) return cached

  const inFlight = inFlightPreviewTasks.get(cacheKey)
  if (inFlight) return inFlight

  const task = enqueuePreviewTask(() => createPreviewObjectUrl(sourceUrl, maxSide, signal), signal)
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

function supportsImageDecode() {
  return typeof Image !== 'undefined' && typeof Image.prototype.decode === 'function'
}

async function canDisplayImage(url: string, signal?: AbortSignal): Promise<boolean> {
  if (signal?.aborted) return false
  if (!supportsImageDecode()) return true
  const image = new Image()
  image.src = url
  try {
    await image.decode()
    if (signal?.aborted) return false
    return true
  } catch {
    return false
  } finally {
    try {
      image.src = ''
    } catch {
      // ignore cleanup failures
    }
    try {
      ;(image as unknown as { removeAttribute?: (name: string) => void }).removeAttribute?.('src')
    } catch {
      // ignore cleanup failures
    }
  }
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
  const resolvedAssetId = computed(() => {
    return normalizeMediaAssetId(toValue(options.assetId))
  })
  const isVisible = computed(() => toValue(options.isVisible) !== false)
  const resolvedMediaTier = computed<MediaAssetDeliveryTier>(() => {
    if (resolvedMaxSide.value === null) return 'original'
    if (resolvedMaxSide.value <= 256) return 'thumb'
    if (resolvedMaxSide.value <= 640) return 'preview'
    // Canvas nodes stay backed by the largest display tier. It is the
    // durable edit-quality source; preview is reserved for editor controls.
    return 'large-preview'
  })
  const mediaDelivery = useMediaAssetDelivery(resolvedAssetId, resolvedMediaTier, {
    enabled: isVisible,
  })
  const previewUrl = ref<string | null>(null)
  const isResolved = ref(false)
  const displayedUrl = ref('')
  const displayedSource = ref('')
  let requestToken = 0
  let activeSource = ''
  let displayRequestToken = 0
  let previewController: AbortController | null = null
  let displayController: AbortController | null = null
  // The candidate most recently observed by the display gate. Pinned while
  // observed so LRU eviction cannot revoke it mid-decode or mid-load.
  let pinnedDisplayUrl = ''

  function setPinnedDisplayUrl(next: string) {
    if (pinnedDisplayUrl === next) return
    unpinPreviewUrl(pinnedDisplayUrl)
    pinnedDisplayUrl = next
    pinPreviewUrl(pinnedDisplayUrl)
  }

  watch(
    [
      () => resolvedSourceUrl.value,
      () => resolvedMaxSide.value,
      () => resolvedAssetId.value,
      isVisible,
    ],
    ([currentSource, maxSide, assetId, visible]) => {
      requestToken += 1
      const token = requestToken
      previewController?.abort()
      previewController = new AbortController()
      const signal = previewController.signal
      if (currentSource !== activeSource) previewUrl.value = null
      activeSource = currentSource
      isResolved.value = false

      if (!currentSource || !visible) return
      if (maxSide === null) {
        previewUrl.value = null
        return
      }

      if (assetId !== null) return

      const cacheKey = buildPreviewCacheKey(currentSource, maxSide)
      const cached = getCachedPreviewUrl(cacheKey)
      if (cached) {
        previewUrl.value = cached
        isResolved.value = true
        return
      }

      void getOrCreatePreviewUrl(currentSource, maxSide, signal).then((generatedPreviewUrl) => {
        if (token !== requestToken || signal.aborted) return
        previewUrl.value = generatedPreviewUrl
        isResolved.value = true
      })
    },
    { immediate: true },
  )

  if (getCurrentScope()) {
    onScopeDispose(() => {
      previewController?.abort()
      displayController?.abort()
      previewController = null
      displayController = null
      setPinnedDisplayUrl('')
      requestToken += 1
      displayRequestToken += 1
    })
  }

  const candidateUrl = computed(() => {
    if (resolvedAssetId.value !== null) {
      if (mediaDelivery.url.value) return mediaDelivery.url.value
      // While the derivative is genuinely not ready (HTTP 202 pending /
      // processing), fall back to the persisted source URL so nodes never
      // hang on blank — unless the source itself is an authenticated API
      // content URL, which must never bind optimistically (CORS + the stage
      // guard relies on an empty candidate while delivery loads).
      // Transient loading/idle keeps the previous frame so zoom-tier
      // switches don't flash the source.
      if (
        (mediaDelivery.status.value === 'pending' || mediaDelivery.status.value === 'processing') &&
        !resolvedSourceUrl.value.startsWith('/api/')
      ) {
        return resolvedSourceUrl.value
      }
      return mediaDelivery.url.value
    }
    if (previewUrl.value) return previewUrl.value
    if (isManagedUpload.value && resolvedMaxSide.value !== null && !isResolved.value) {
      return ''
    }
    return resolvedSourceUrl.value
  })

  watch(
    [resolvedSourceUrl, candidateUrl],
    ([currentSource, nextUrl]) => {
      displayRequestToken += 1
      const token = displayRequestToken
      displayController?.abort()
      displayController = new AbortController()
      const signal = displayController.signal

      if (!currentSource) {
        displayedSource.value = ''
        displayedUrl.value = ''
        setPinnedDisplayUrl('')
        return
      }

      if (currentSource !== displayedSource.value) {
        displayedSource.value = currentSource
        if (!options.keepPreviousOnSourceChange) displayedUrl.value = ''
      }
      setPinnedDisplayUrl(nextUrl)
      if (!nextUrl || nextUrl === displayedUrl.value) return

      // A new external/blob source is already the optimistic preview. Commit it immediately;
      // derived previews and delivery URLs wait for decode so a zoom never flashes a placeholder.
      if (nextUrl === currentSource && !isManagedUpload.value && !displayedUrl.value) {
        displayedUrl.value = nextUrl
        return
      }

      void canDisplayImage(nextUrl, signal).then((isDisplayable) => {
        if (
          !isDisplayable ||
          token !== displayRequestToken ||
          signal.aborted ||
          currentSource !== displayedSource.value
        ) {
          return
        }
        displayedUrl.value = nextUrl
      })
    },
    { immediate: true, flush: 'sync' },
  )

  const displayUrl = computed(() => displayedUrl.value)

  return {
    displayUrl,
  }
}
