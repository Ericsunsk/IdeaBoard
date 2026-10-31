import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  shallowRef,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue'
import {
  invalidateImageNaturalSize,
  readImageNaturalSize,
} from '../../../../core/media/imageDimensions'
import { resolveAssetUrl } from '../../../../core/config/runtimeUrls'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import { createPollDelay } from '../../../../core/utils/polling'
import {
  ensureWorkspaceMediaInvalidation,
  getWorkspaceMediaAssetMetadata,
  refreshWorkspaceMediaAssetMetadata,
  subscribeAssetInvalidation,
  type MediaAssetDeliveryMetadata,
} from '@/api/media/mediaAssetMetadata'
import { SCENE_THUMBNAIL_TIER } from '../../core/preview/renderer'

// Gallery thumbnail media.
//
// Reliability contract for Scene raster `<image>` mounts:
//
// - Scene generation never waits on images: the layout renders first and
//   rasters resolve independently; missing media never reads as an empty
//   board, and a missing raster is never committed as a success.
// - Reuses the authorized Workspace Media transport (metadata cache,
//   workspace-stream invalidation). The thumbnail tier is fixed (`preview`),
//   independent of canvas zoom. No independent polling loops and no second
//   cache: recovery after the bounded retries rides invalidation events.
// - Bounded retry: pending/retryable states retry at most
//   `THUMBNAIL_MEDIA_MAX_ATTEMPTS` with capped backoff; 403/404 and
//   permanent-delete codes are terminal and never retried.
// - Same-identity transient failures keep the last usable image; any
//   workspace/asset identity change, scope disposal, or destroy clears it.
// - Every async commit is gated on the request token, mount state, and page
//   visibility; late resolves never overwrite a newer identity.

export const THUMBNAIL_MEDIA_MAX_ATTEMPTS = 3
export const THUMBNAIL_MEDIA_READ_TIMEOUT_MS = 15_000

export type SceneThumbnailMediaStatus =
  'idle' | 'loading' | 'pending' | 'ready' | 'failed' | 'error'

export type SceneThumbnailMediaTransport = {
  readMetadata: (
    workspaceId: string,
    assetId: string,
    signal: AbortSignal,
  ) => Promise<MediaAssetDeliveryMetadata>
  refreshMetadata: (
    workspaceId: string,
    assetId: string,
    signal: AbortSignal,
    staleMetadata?: MediaAssetDeliveryMetadata,
  ) => Promise<MediaAssetDeliveryMetadata>
  ensureInvalidation: (workspaceId: string) => () => void
  subscribeInvalidation: (assetId: string, listener: () => void) => () => void
  loadImage?: (url: string, signal: AbortSignal) => Promise<boolean>
}

const defaultTransport: SceneThumbnailMediaTransport = {
  readMetadata: (workspaceId, assetId, signal) =>
    getWorkspaceMediaAssetMetadata(workspaceId, assetId, { signal }),
  refreshMetadata: (workspaceId, assetId, signal, staleMetadata) =>
    refreshWorkspaceMediaAssetMetadata(workspaceId, assetId, { signal, staleMetadata }),
  ensureInvalidation: ensureWorkspaceMediaInvalidation,
  subscribeInvalidation: subscribeAssetInvalidation,
}

const TERMINAL_MEDIA_CODES = new Set([
  'media_not_found',
  'media_forbidden',
  'media_archived',
  'asset_archived',
  'media_deleted',
])

function errorStatusInChain(error: unknown): number | null {
  const seen = new Set<unknown>()
  let current: unknown = error
  while (current && typeof current === 'object' && !seen.has(current)) {
    seen.add(current)
    const record = current as { status?: unknown; cause?: unknown }
    if (typeof record.status === 'number') return record.status
    current = record.cause
  }
  return null
}

/** 403/404 and permanent-delete codes are terminal: never retried. */
export function isTerminalThumbnailMediaError(error: unknown): boolean {
  const status = errorStatusInChain(error)
  if (status === 403 || status === 404) return true
  const code = userErrorCode(error)
  return code !== null && TERMINAL_MEDIA_CODES.has(code)
}

function resolvePreviewVariant(metadata: MediaAssetDeliveryMetadata) {
  return metadata.derivatives[SCENE_THUMBNAIL_TIER]
}

export type UseSceneThumbnailMediaOptions = {
  assetId: MaybeRefOrGetter<string | null | undefined>
  workspaceId: MaybeRefOrGetter<string | null | undefined>
  boardId: MaybeRefOrGetter<string | null | undefined>
  contentVersion: MaybeRefOrGetter<string | null | undefined>
  isEnabled?: MaybeRefOrGetter<boolean> | undefined
  transport?: SceneThumbnailMediaTransport | undefined
  isPageVisible?: (() => boolean) | undefined
}

type ThumbnailImage = {
  url: string
  version: number
  onLoad: () => void
  onError: () => void
}

export function useSceneThumbnailMedia(options: UseSceneThumbnailMediaOptions) {
  const transport = options.transport ?? defaultTransport
  const isPageVisible = options.isPageVisible ?? defaultPageVisibility
  const candidateImage = shallowRef<ThumbnailImage | null>(null)
  const displayedImage = shallowRef<ThumbnailImage | null>(null)
  const url = computed(() => candidateImage.value?.url ?? displayedImage.value?.url ?? '')
  const identity = computed(() => ({
    workspaceId: normalizeId(toValue(options.workspaceId)),
    assetId: normalizeId(toValue(options.assetId)),
    boardId: normalizeId(toValue(options.boardId)),
    contentVersion: normalizeId(toValue(options.contentVersion)),
  }))
  let imageVersion = 0
  let isDisposed = false
  const status = ref<SceneThumbnailMediaStatus>('idle')
  const error = ref<unknown>(null)
  let lastMetadata: MediaAssetDeliveryMetadata | undefined
  let requestToken = 0
  let loadEpoch = 0
  let attempts = 0
  let isResumeOnVisible = false
  let activeWorkspaceId: string | null = null
  let activeAssetId: string | null = null
  let abortController: AbortController | null = null
  let retryTimer = 0
  let readTimeoutTimer = 0
  let unsubscribeInvalidation: (() => void) | null = null
  let releaseWorkspaceSubscription: (() => void) | null = null
  let stopVisibility: (() => void) | null = null
  const retryDelay = createPollDelay({ initialDelayMs: 800, maxDelayMs: 2_000, backoffFactor: 2 })

  function defaultPageVisibility(): boolean {
    return typeof document === 'undefined' || document.visibilityState !== 'hidden'
  }

  function clearTimers() {
    window.clearTimeout(retryTimer)
    retryTimer = 0
    window.clearTimeout(readTimeoutTimer)
    readTimeoutTimer = 0
  }

  function abortLoad() {
    loadEpoch += 1
    abortController?.abort()
    abortController = null
    clearTimers()
    candidateImage.value = null
  }

  function teardownSubscriptions() {
    unsubscribeInvalidation?.()
    unsubscribeInvalidation = null
    releaseWorkspaceSubscription?.()
    releaseWorkspaceSubscription = null
    stopVisibility?.()
    stopVisibility = null
  }

  function clearStaleImage() {
    lastMetadata = undefined
    candidateImage.value = null
    displayedImage.value = null
    status.value = 'idle'
    error.value = null
    attempts = 0
    isResumeOnVisible = false
    retryDelay.reset()
  }

  function scheduleRetry(workspaceId: string, assetId: string) {
    // Visibility gates retries: hidden tabs schedule nothing and resume on
    // the next visible event instead of polling in the background.
    if (isDisposed || !isPageVisible()) return
    window.clearTimeout(retryTimer)
    retryTimer = window.setTimeout(() => {
      retryTimer = 0
      void load(workspaceId, assetId, true)
    }, retryDelay.next())
  }

  function commitReady(variantUrl: string) {
    const token = requestToken
    const epoch = loadEpoch
    const imageIdentity = identity.value
    const isCurrent = () =>
      !isDisposed &&
      token === requestToken &&
      epoch === loadEpoch &&
      imageIdentity === identity.value &&
      candidateImage.value === image &&
      toValue(options.isEnabled) !== false &&
      isPageVisible()
    const image: ThumbnailImage = {
      url: resolveAssetUrl(variantUrl),
      version: ++imageVersion,
      onLoad: () => {
        if (!isCurrent()) return
        displayedImage.value = image
        candidateImage.value = null
        attempts = 0
        retryDelay.reset()
      },
      onError: () => {
        if (!isCurrent() || !activeWorkspaceId || !activeAssetId) return
        candidateImage.value = null
        invalidateImageNaturalSize(image.url)
        settleRetryable(
          activeWorkspaceId,
          activeAssetId,
          new Error('Media preview image could not be displayed'),
        )
      },
    }
    candidateImage.value = image
    status.value = 'ready'
    error.value = null
    isResumeOnVisible = false
    retryDelay.reset()
  }

  async function load(workspaceId: string, assetId: string, shouldRefresh: boolean) {
    if (isDisposed || toValue(options.isEnabled) === false) return
    if (!isPageVisible()) {
      status.value = 'pending'
      isResumeOnVisible = true
      return
    }
    const token = requestToken
    abortLoad()
    const epoch = loadEpoch
    attempts += 1
    const controller = new AbortController()
    abortController = controller
    status.value = 'loading'
    error.value = null
    // Within one load, the timeout and the transport race: whichever settles
    // first wins, the loser is dropped. Across loads, the epoch drops stale
    // commits from superseded requests.
    let isSettled = false
    const settleOnce = (settle: () => void) => {
      if (isSettled || epoch !== loadEpoch || token !== requestToken) return
      isSettled = true
      settle()
    }
    let metadata: MediaAssetDeliveryMetadata
    try {
      readTimeoutTimer = window.setTimeout(() => {
        controller.abort()
        settleOnce(() => settleRetryable(workspaceId, assetId, timeoutError(assetId)))
      }, THUMBNAIL_MEDIA_READ_TIMEOUT_MS)
      metadata = shouldRefresh
        ? await transport.refreshMetadata(workspaceId, assetId, controller.signal, lastMetadata)
        : await transport.readMetadata(workspaceId, assetId, controller.signal)
      const variant = resolvePreviewVariant(metadata)
      if (variant?.status === 'ready' && variant.url) {
        const source = resolveAssetUrl(variant.url)
        const isLoaded = transport.loadImage
          ? await transport.loadImage(source, controller.signal)
          : await readImageNaturalSize(source, THUMBNAIL_MEDIA_READ_TIMEOUT_MS, controller.signal)
        if (!isLoaded) throw new Error('Media preview image could not be loaded')
      }
    } catch (loadError) {
      settleOnce(() => settleLoadError(workspaceId, assetId, loadError))
      return
    }
    settleOnce(() => {
      lastMetadata = metadata
      settleLoadedVariant(workspaceId, assetId, metadata)
    })
  }

  function timeoutError(assetId: string): Error {
    return new Error(`Media asset ${assetId} preview read timed out`)
  }

  function settleLoadError(workspaceId: string, assetId: string, loadError: unknown) {
    window.clearTimeout(readTimeoutTimer)
    readTimeoutTimer = 0
    if (isTerminalThumbnailMediaError(loadError)) {
      // Permission or existence is gone for this identity: stop the stale
      // image and never retry it.
      candidateImage.value = null
      displayedImage.value = null
      status.value = 'error'
      error.value = loadError
      isResumeOnVisible = false
      return
    }
    settleRetryable(workspaceId, assetId, loadError)
  }

  function settleLoadedVariant(
    workspaceId: string,
    assetId: string,
    metadata: MediaAssetDeliveryMetadata,
  ) {
    window.clearTimeout(readTimeoutTimer)
    readTimeoutTimer = 0
    const variant = resolvePreviewVariant(metadata)
    const variantStatus = variant?.status ?? metadata.status
    if (variantStatus === 'ready' && variant?.url) {
      commitReady(variant.url)
      return
    }
    if (variantStatus === 'pending' || variantStatus === 'processing') {
      status.value = 'pending'
      if (attempts < THUMBNAIL_MEDIA_MAX_ATTEMPTS) scheduleRetry(workspaceId, assetId)
      else {
        status.value = 'error'
        error.value = new Error(`Media asset ${assetId} preview timed out`)
        isResumeOnVisible = true
      }
      return
    }
    // `failed`, or ready without a URL: a missing raster is never encoded as
    // a success fact. The last usable image stays when the identity matches.
    status.value = 'failed'
    error.value = new Error(`Media asset ${assetId} preview unavailable`)
    isResumeOnVisible = false
  }

  function settleRetryable(workspaceId: string, assetId: string, loadError: unknown) {
    if (attempts < THUMBNAIL_MEDIA_MAX_ATTEMPTS) {
      status.value = 'loading'
      error.value = null
      scheduleRetry(workspaceId, assetId)
      return
    }
    // Attempts exhausted: keep the last usable image for this identity and
    // surface the error; recovery afterwards rides invalidation events (with
    // a fresh budget per event) or the next visible tab only.
    status.value = 'error'
    error.value = loadError
    isResumeOnVisible = true
  }

  function reloadForEvent(workspaceId: string, assetId: string) {
    // Event-driven recovery (invalidation, tab visible) gets a fresh attempt
    // budget: events are discrete server or user facts, never a client loop.
    attempts = 0
    isResumeOnVisible = false
    retryDelay.reset()
    void load(workspaceId, assetId, true)
  }

  function startFor(workspaceId: string, assetId: string) {
    attempts = 0
    isResumeOnVisible = false
    retryDelay.reset()
    try {
      releaseWorkspaceSubscription = transport.ensureInvalidation(workspaceId)
    } catch (subscriptionError) {
      status.value = 'error'
      error.value = subscriptionError
      return
    }
    unsubscribeInvalidation = transport.subscribeInvalidation(assetId, () => {
      // Every event revalidates, including while ready: an `asset_archived`
      // arriving after a commit must still stop the stale image. Events are
      // discrete server facts, so this stays event-driven, never a poll.
      reloadForEvent(workspaceId, assetId)
    })
    stopVisibility = listenVisible(() => {
      if (status.value === 'pending' || status.value === 'loading') {
        reloadForEvent(workspaceId, assetId)
      } else if (status.value === 'error' && isResumeOnVisible) {
        reloadForEvent(workspaceId, assetId)
      }
    })
    void load(workspaceId, assetId, false)
  }

  function listenVisible(onVisible: () => void): () => void {
    if (typeof document === 'undefined' || typeof document.addEventListener !== 'function') {
      return () => {}
    }
    const handler = () => {
      if (isPageVisible()) onVisible()
      else {
        if (candidateImage.value) status.value = 'pending'
        abortLoad()
      }
    }
    document.addEventListener('visibilitychange', handler)
    return () => document.removeEventListener('visibilitychange', handler)
  }

  watch(
    [identity, () => toValue(options.isEnabled) !== false],
    ([{ workspaceId, assetId }, isEnabled]) => {
      requestToken += 1
      abortLoad()
      teardownSubscriptions()
      const hasIdentityChanged = workspaceId !== activeWorkspaceId || assetId !== activeAssetId
      if (hasIdentityChanged) {
        activeWorkspaceId = workspaceId
        activeAssetId = assetId
        clearStaleImage()
      }
      if (!workspaceId || !assetId || !isEnabled) return
      startFor(workspaceId, assetId)
    },
    { immediate: true },
  )

  if (getCurrentScope()) {
    onScopeDispose(() => {
      isDisposed = true
      requestToken += 1
      abortLoad()
      teardownSubscriptions()
      clearStaleImage()
    })
  }

  return {
    url,
    images: computed(() => [
      ...(displayedImage.value ? [{ ...displayedImage.value, isVisible: true }] : []),
      ...(candidateImage.value ? [{ ...candidateImage.value, isVisible: false }] : []),
    ]),
    status,
    error,
    isLoading: computed(() => status.value === 'loading' || status.value === 'pending'),
  }
}

function normalizeId(value: string | null | undefined): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed ? trimmed : null
}
