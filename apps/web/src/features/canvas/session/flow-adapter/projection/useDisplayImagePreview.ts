import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue'
import { beginCanvasInteractionTiming } from '../../browser/canvasInteractionMetrics'
import { parseImageUrlPathname } from '../../media/canvasMediaIo.ts'
import { createDisplayPreviewResources } from './displayImagePreviewResources'
import {
  canonicalizeRuntimeUploadRef,
  resolveAssetUrl,
} from '../../../../../core/config/runtimeUrls'
import { resolveMediaDisplayTier } from '../../../../../core/media/displayTier'
import { isMediaAssetDeliveryPath } from '../../../../../core/media/mediaAssetPath'
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
  shouldKeepPreviousOnSourceChange?: boolean
}

const DEFAULT_MAX_SIDE = 1280

function resolvePreviewMaxSide(value: unknown): number | null {
  // null is the explicit full-quality tier: keep the original URL and skip canvas work.
  if (value === null) return null
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? Math.floor(value)
    : DEFAULT_MAX_SIDE
}

function supportsImageDecode() {
  return typeof Image !== 'undefined' && typeof Image.prototype.decode === 'function'
}

function canDisplayImage(url: string, signal: AbortSignal, metricName: string): Promise<boolean> {
  if (signal.aborted) return Promise.resolve(false)
  if (!supportsImageDecode()) return Promise.resolve(true)
  return new Promise((resolve) => {
    const finishTiming = beginCanvasInteractionTiming(metricName)
    const image = new Image()
    let isSettled = false
    const finish = (isDisplayable: boolean) => {
      if (isSettled) return
      isSettled = true
      signal.removeEventListener('abort', handleAbort)
      try {
        image.src = ''
        image.removeAttribute?.('src')
      } catch {
        // Some image implementations do not support clearing their source.
      }
      finishTiming?.({
        count: `${metricName}${signal.aborted ? 'Cancelled' : isDisplayable ? 'Ready' : 'Failed'}`,
        shouldRecordTiming: isDisplayable,
      })
      resolve(isDisplayable)
    }
    const handleAbort = () => finish(false)
    signal.addEventListener('abort', handleAbort, { once: true })
    try {
      image.src = url
      void image.decode().then(
        () => finish(!signal.aborted),
        () => finish(false),
      )
    } catch {
      finish(false)
    }
  })
}

export function useDisplayImagePreview(
  sourceUrl: MaybeRefOrGetter<string | null | undefined>,
  options: PreviewOptions = {},
) {
  const canonicalSourceUrl = computed(() => {
    const raw = toValue(sourceUrl)
    return raw ? canonicalizeRuntimeUploadRef(raw.trim()).trim() : ''
  })
  const isManagedUpload = computed(() => {
    const source = canonicalSourceUrl.value
    if (!source) return false
    const pathname = parseImageUrlPathname(source)
    return (
      pathname !== null && (pathname.startsWith('/uploads/') || isMediaAssetDeliveryPath(pathname))
    )
  })
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
    return resolveMediaDisplayTier(resolvedMaxSide.value)
  })
  const mediaDelivery = useMediaAssetDelivery(resolvedAssetId, resolvedMediaTier, {
    isEnabled: isVisible,
  })
  const previewUrl = ref<string | null>(null)
  const isResolved = ref(false)
  const displayedUrl = ref('')
  const displayedSource = ref('')
  let requestToken = 0
  let activeSource = ''
  let displayRequestToken = 0
  const previewResources = createDisplayPreviewResources()
  let displayController: AbortController | null = null

  watch(
    [
      () => resolvedSourceUrl.value,
      () => resolvedMaxSide.value,
      () => resolvedAssetId.value,
      isVisible,
    ],
    ([currentSource, maxSide, assetId, isVisible]) => {
      requestToken += 1
      const token = requestToken
      const previewTask = previewResources.load(
        currentSource && isVisible && assetId === null ? currentSource : '',
        maxSide,
      )
      if (currentSource !== activeSource) previewUrl.value = null
      activeSource = currentSource
      isResolved.value = false

      if (!currentSource || !isVisible) return
      if (maxSide === null) {
        previewUrl.value = null
        return
      }

      if (assetId !== null) return

      void previewTask.then((generatedPreviewUrl) => {
        if (token !== requestToken) return
        previewUrl.value = generatedPreviewUrl
        isResolved.value = true
      })
    },
    { immediate: true },
  )

  if (getCurrentScope()) {
    onScopeDispose(() => {
      displayController?.abort()
      displayController = null
      previewResources.dispose()
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
        previewResources.display()
        return
      }

      if (currentSource !== displayedSource.value) {
        displayedSource.value = currentSource
        if (!options.shouldKeepPreviousOnSourceChange) displayedUrl.value = ''
      }
      previewResources.display(nextUrl, displayedUrl.value)
      if (!nextUrl || nextUrl === displayedUrl.value) return

      // A new external/blob source is already the optimistic preview. Commit it immediately;
      // derived previews and delivery URLs wait for decode so a zoom never flashes a placeholder.
      if (nextUrl === currentSource && !isManagedUpload.value && !displayedUrl.value) {
        displayedUrl.value = nextUrl
        previewResources.display(nextUrl)
        return
      }

      const decodeMetric =
        resolvedAssetId.value === null ? 'displayImageFallbackDecode' : 'displayImageManagedDecode'
      void canDisplayImage(nextUrl, signal, decodeMetric).then(async (isDisplayable) => {
        let decodedUrl = isDisplayable ? nextUrl : ''
        const fallbackUrl = contentFallbackUrl.value
        if (!decodedUrl && fallbackUrl && fallbackUrl !== nextUrl && !signal.aborted) {
          if (await canDisplayImage(fallbackUrl, signal, decodeMetric)) decodedUrl = fallbackUrl
        }
        if (
          !decodedUrl ||
          token !== displayRequestToken ||
          signal.aborted ||
          currentSource !== displayedSource.value
        ) {
          return
        }
        displayedUrl.value = decodedUrl
        previewResources.display(decodedUrl)
      })
    },
    { immediate: true, flush: 'sync' },
  )

  const displayUrl = computed(() => displayedUrl.value)

  // Offer the original only after metadata confirms it is ready.
  const contentFallbackUrl = computed(() => {
    if (resolvedAssetId.value === null) return ''
    const original = mediaDelivery.metadata.value?.original
    const originalUrl = original?.url?.trim()
    if (original?.status === 'ready' && originalUrl) return resolveAssetUrl(originalUrl)
    return ''
  })

  return {
    displayUrl,
    contentFallbackUrl,
    retainDisplayUrl: previewResources.retain,
  }
}
