import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue'
import { resolveAssetUrl } from '../config/runtimeUrls'
import { createPollDelay } from '../utils/polling'
import {
  ensureWorkspaceMediaInvalidation,
  getMediaAssetMetadata,
  normalizeMediaAssetId,
  subscribeAssetInvalidation,
  type MediaAssetDeliveryMetadata,
  type MediaAssetDeliveryStatus,
} from './mediaAssetMetadata'
import {
  requireMediaWorkspaceId,
  MediaWorkspaceUnavailableError,
  type MediaAssetDeliveryTier,
  type MediaAssetId,
} from './mediaAssetUrls'

// Vue lifecycle binding for workspace media delivery.
//
// This module owns ALL component lifecycle in the delivery boundary
// (watchers, scope disposal, abort controllers, retry timers). The URL
// seam (`./mediaAssetUrls`) and the HTTP/cache/invalidation transport
// (`./mediaAssetMetadata`) stay free of Vue imports so domain and
// transport code never binds component lifecycles.

function normalizeTier(value: unknown): MediaAssetDeliveryTier {
  return value === 'original' ||
    value === 'thumb' ||
    value === 'preview' ||
    value === 'large-preview' ||
    value === 'processing-input'
    ? value
    : 'preview'
}

function resolveVariant(metadata: MediaAssetDeliveryMetadata, tier: MediaAssetDeliveryTier) {
  return tier === 'original' ? metadata.original : metadata.derivatives[tier]
}

export function useMediaAssetDelivery(
  assetId: MaybeRefOrGetter<MediaAssetId | null | undefined>,
  tier: MaybeRefOrGetter<MediaAssetDeliveryTier>,
  options: { enabled?: MaybeRefOrGetter<boolean> } = {},
) {
  const resolvedAssetId = computed(() => normalizeMediaAssetId(toValue(assetId)))
  const resolvedTier = computed(() => normalizeTier(toValue(tier)))
  const isEnabled = computed(() => toValue(options.enabled) !== false)
  const metadata = ref<MediaAssetDeliveryMetadata | null>(null)
  const url = ref('')
  const status = ref<MediaAssetDeliveryStatus>('idle')
  const error = ref<unknown>(null)
  let requestToken = 0
  let abortController: AbortController | null = null
  let unsubscribeInvalidation: (() => void) | null = null
  let releaseWorkspaceSubscription: (() => void) | null = null
  let retryTimer = 0
  const retryDelay = createPollDelay({ initialDelayMs: 800, maxDelayMs: 2_000, backoffFactor: 2 })

  function reset() {
    window.clearTimeout(retryTimer)
    retryTimer = 0
    retryDelay.reset()
    metadata.value = null
    url.value = ''
    status.value = 'idle'
    error.value = null
  }

  watch(
    [resolvedAssetId, resolvedTier, isEnabled],
    ([currentAssetId, currentTier, enabled]) => {
      requestToken += 1
      const token = requestToken
      abortController?.abort()
      abortController = null
      unsubscribeInvalidation?.()
      unsubscribeInvalidation = null
      releaseWorkspaceSubscription?.()
      releaseWorkspaceSubscription = null
      reset()
      if (currentAssetId === null || !enabled) return

      try {
        releaseWorkspaceSubscription = ensureWorkspaceMediaInvalidation(requireMediaWorkspaceId())
      } catch {
        status.value = 'error'
        error.value = new MediaWorkspaceUnavailableError()
        return
      }
      unsubscribeInvalidation = subscribeAssetInvalidation(currentAssetId, () => {
        if (token !== requestToken) return
        void load(currentAssetId, currentTier, true)
      })
      void load(currentAssetId, currentTier)
    },
    { immediate: true },
  )

  function scheduleDeliveryRetry(
    currentAssetId: string,
    currentTier: MediaAssetDeliveryTier,
  ): void {
    window.clearTimeout(retryTimer)
    retryTimer = window.setTimeout(() => {
      retryTimer = 0
      void load(currentAssetId, currentTier, true)
    }, retryDelay.next())
  }

  function settleDeliveryUrl(
    variant: ReturnType<typeof resolveVariant> | undefined,
    nextStatus: MediaAssetDeliveryStatus,
    currentAssetId: string,
    currentTier: MediaAssetDeliveryTier,
  ): void {
    status.value = nextStatus
    url.value = variant?.status === 'ready' && variant.url ? resolveAssetUrl(variant.url) : ''
    if (url.value) {
      retryDelay.reset()
    } else if (nextStatus === 'pending' || nextStatus === 'processing') {
      scheduleDeliveryRetry(currentAssetId, currentTier)
    }
  }

  async function load(currentAssetId: string, currentTier: MediaAssetDeliveryTier, force = false) {
    const token = requestToken
    abortController?.abort()
    const controller = new AbortController()
    abortController = controller
    status.value = 'loading'
    error.value = null
    try {
      const nextMetadata = await getMediaAssetMetadata(currentAssetId, {
        signal: controller.signal,
        force,
      })
      if (token !== requestToken || controller.signal.aborted) return
      metadata.value = nextMetadata
      const variant = resolveVariant(nextMetadata, currentTier)
      const nextStatus = (variant?.status || nextMetadata.status) as MediaAssetDeliveryStatus
      settleDeliveryUrl(variant, nextStatus, currentAssetId, currentTier)
    } catch (loadError) {
      if (controller.signal.aborted || token !== requestToken) return
      status.value = 'error'
      error.value = loadError
      url.value = ''
    }
  }

  if (getCurrentScope()) {
    onScopeDispose(() => {
      requestToken += 1
      abortController?.abort()
      window.clearTimeout(retryTimer)
      unsubscribeInvalidation?.()
      releaseWorkspaceSubscription?.()
    })
  }

  return {
    metadata,
    url,
    status,
    error,
    isLoading: computed(
      () =>
        status.value === 'loading' || status.value === 'pending' || status.value === 'processing',
    ),
  }
}
