import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue'
import { resolveApiUrl, resolveAssetUrl } from '../config/runtimeUrls'
import { requireApiData } from './apiResponse'
import { api } from './client'
import { subscribeUserEvent } from './userEventStream'
import { i18n } from '../i18n'
import {
  getGetUserMediaAssetMetadataUrl,
  getGetUserMediaAssetMetadataBatchUrl,
  getGetUserMediaAssetOriginalContentUrl,
  getGetUserMediaAssetDerivativeContentUrl,
} from './generated-query/user-assets/user-assets'
import type {
  ApiResponseMediaAssetMetadata,
  ApiResponseMediaAssetMetadataBatch,
  MediaAssetDeliveryVariant as GeneratedMediaAssetDeliveryVariant,
  MediaAssetMetadata,
} from './generated-query/model'

export type MediaAssetDeliveryTier =
  'thumb' | 'preview' | 'large-preview' | 'processing' | 'processing-input' | 'original'
export type MediaAssetDeliveryStatus =
  'idle' | 'loading' | 'pending' | 'processing' | 'ready' | 'failed' | 'error'

export type MediaAssetDeliveryMetadata = MediaAssetMetadata
export type MediaAssetDeliveryVariant = GeneratedMediaAssetDeliveryVariant

export class MediaAssetDerivativeNotReadyError extends Error {
  readonly tier: Exclude<MediaAssetDeliveryTier, 'original'>
  readonly status: string

  constructor(tier: Exclude<MediaAssetDeliveryTier, 'original'>, status: string) {
    super(`Media asset ${tier} derivative is ${status}`)
    this.name = 'MediaAssetDerivativeNotReadyError'
    this.tier = tier
    this.status = status
  }
}

type CachedMetadata = {
  value: MediaAssetDeliveryMetadata
  expiresAt: number
}

const METADATA_CACHE_TTL_MS = 5 * 60 * 1000
const SIGNED_URL_REFRESH_SKEW_MS = 2 * 60 * 1000

const metadataCache = new Map<number, CachedMetadata>()
const metadataRequests = new Map<string, Promise<MediaAssetDeliveryMetadata>>()
const metadataBatchRequests = new Map<string, Promise<ReadonlyMap<number, MediaAssetDeliveryMetadata>>>()
const ensuredTiers = new Set<string>()
const invalidationListeners = new Map<number, Set<() => void>>()
let stopMediaEventSubscription: (() => void) | null = null

function parseSignedURLExpiry(url: string): number | null {
  try {
    const parsed = new URL(
      url,
      typeof window === 'undefined' ? 'http://localhost' : window.location.origin,
    )
    const expires = Number(
      parsed.searchParams.get('X-Amz-Expires') ?? parsed.searchParams.get('Expires'),
    )
    if (!Number.isFinite(expires) || expires <= 0) return null
    const signedAt = parsed.searchParams.get('X-Amz-Date')
    if (signedAt) {
      const normalized = signedAt.replace(
        /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/,
        '$1-$2-$3T$4:$5:$6Z',
      )
      const timestamp = Date.parse(normalized)
      if (Number.isFinite(timestamp)) return timestamp + expires * 1000
    }
    return Date.now() + expires * 1000
  } catch {
    return null
  }
}

function resolveMetadataExpiry(metadata: MediaAssetDeliveryMetadata): number {
  const signedURLExpiries = [
    metadata.original.url,
    ...Object.values(metadata.derivatives).map((item) => item.url),
  ]
    .filter((url): url is string => typeof url === 'string' && url.length > 0)
    .map(parseSignedURLExpiry)
    .filter((timestamp): timestamp is number => timestamp !== null)
  const signedURLExpiry = signedURLExpiries.length > 0 ? Math.min(...signedURLExpiries) : Infinity
  return Math.min(Date.now() + METADATA_CACHE_TTL_MS, signedURLExpiry - SIGNED_URL_REFRESH_SKEW_MS)
}

function notifyAssetInvalidated(assetId: number) {
  metadataCache.delete(assetId)
  for (const listener of invalidationListeners.get(assetId) ?? []) listener()
}

function ensureMediaEventSubscription() {
  if (stopMediaEventSubscription) return
  stopMediaEventSubscription = subscribeUserEvent('media.derivative.updated', (event) => {
    try {
      const payload = JSON.parse(event.data) as { media_asset_id?: unknown }
      const assetId = Number(payload.media_asset_id)
      if (Number.isInteger(assetId) && assetId > 0) notifyAssetInvalidated(assetId)
    } catch {
      return
    }
  })
}

function subscribeAssetInvalidation(assetId: number, listener: () => void): () => void {
  ensureMediaEventSubscription()
  let listeners = invalidationListeners.get(assetId)
  if (!listeners) {
    listeners = new Set()
    invalidationListeners.set(assetId, listeners)
  }
  listeners.add(listener)
  return () => {
    listeners?.delete(listener)
    if (listeners?.size === 0) invalidationListeners.delete(assetId)
    if (invalidationListeners.size === 0 && stopMediaEventSubscription) {
      stopMediaEventSubscription()
      stopMediaEventSubscription = null
    }
  }
}

function getCachedMetadata(assetId: number): MediaAssetDeliveryMetadata | null {
  const cached = metadataCache.get(assetId)
  if (!cached) return null
  if (cached.expiresAt <= Date.now()) {
    metadataCache.delete(assetId)
    return null
  }
  return cached.value
}

export function invalidateMediaAssetMetadata(assetId?: number) {
  if (assetId === undefined) {
    metadataCache.clear()
    ensuredTiers.clear()
    return
  }
  notifyAssetInvalidated(assetId)
}

export async function getMediaAssetMetadata(
  assetId: number,
  options: { signal?: AbortSignal; force?: boolean; ensureTier?: MediaAssetDeliveryTier } = {},
): Promise<MediaAssetDeliveryMetadata> {
  if (!Number.isInteger(assetId) || assetId <= 0)
    throw new Error(i18n.global.t('common.errors.mediaInvalid'))
  const ensureKey = options.ensureTier ? `${assetId}:${options.ensureTier}` : ''
  const requestKey = ensureKey || String(assetId)
  const needsEnsure = Boolean(ensureKey && !ensuredTiers.has(ensureKey))
  if (!options.force && !needsEnsure) {
    const cached = getCachedMetadata(assetId)
    if (cached) return cached
  }
  const existing = metadataRequests.get(requestKey)
  if (existing) return existing

  const request = api
    .request<ApiResponseMediaAssetMetadata>(
      `${getGetUserMediaAssetMetadataUrl(String(assetId))}${
        options.ensureTier ? `?ensure_tier=${encodeURIComponent(options.ensureTier)}` : ''
      }`,
      { method: 'GET', signal: options.signal },
    )
    .then((response) =>
      requireApiData(response, i18n.global.t('common.errors.mediaInfoLoadFailed')),
    )
    .then((metadata) => {
      metadataCache.set(assetId, { value: metadata, expiresAt: resolveMetadataExpiry(metadata) })
      if (ensureKey) ensuredTiers.add(ensureKey)
      return metadata
    })
    .finally(() => {
      if (metadataRequests.get(requestKey) === request) metadataRequests.delete(requestKey)
    })
  metadataRequests.set(requestKey, request)
  return request
}

/**
 * Fetch metadata for multiple assets in one round trip and hydrate the same
 * cache used by useMediaAssetDelivery. Results are keyed by numeric media ID;
 * assets the user cannot access are omitted by the API and therefore absent.
 */
export async function getMediaAssetMetadataBatch(
  assetIds: readonly number[],
  options: {
    signal?: AbortSignal
    force?: boolean
    ensureTier?: Extract<MediaAssetDeliveryTier, 'large-preview' | 'processing' | 'processing-input'>
  } = {},
): Promise<ReadonlyMap<number, MediaAssetDeliveryMetadata>> {
  const ids = Array.from(
    new Set(
      assetIds.filter((value) => Number.isInteger(value) && value > 0),
    ),
  )
  if (ids.length === 0) return new Map()
  if (ids.length > 100) throw new Error('Too many media assets requested')
  const sortedIDs = [...ids].sort((a, b) => a - b)
  const requestKey = `${sortedIDs.join(',')}:${options.ensureTier ?? ''}`
  if (!options.force) {
    const cached = new Map<number, MediaAssetDeliveryMetadata>()
    for (const id of ids) {
      const metadata = getCachedMetadata(id)
      if (metadata) cached.set(id, metadata)
    }
    if (cached.size === ids.length) return cached
  }
  const existing = metadataBatchRequests.get(requestKey)
  if (existing) return existing
  const request = api
    .request<ApiResponseMediaAssetMetadataBatch>(getGetUserMediaAssetMetadataBatchUrl(), {
      method: 'POST',
      signal: options.signal,
      body: JSON.stringify({
        asset_ids: ids.map(String),
        ...(options.ensureTier ? { ensure_tier: options.ensureTier } : {}),
      }),
    })
    .then((response) => requireApiData(response, i18n.global.t('common.errors.mediaInfoLoadFailed')))
    .then((data) => {
      const result = new Map<number, MediaAssetDeliveryMetadata>()
      for (const metadata of data.items ?? []) {
        const id = Number(metadata.asset_id)
        if (!Number.isInteger(id) || id <= 0) continue
        metadataCache.set(id, { value: metadata, expiresAt: resolveMetadataExpiry(metadata) })
        if (options.ensureTier) ensuredTiers.add(`${id}:${options.ensureTier}`)
        result.set(id, metadata)
      }
      return result as ReadonlyMap<number, MediaAssetDeliveryMetadata>
    })
    .finally(() => {
      if (metadataBatchRequests.get(requestKey) === request) metadataBatchRequests.delete(requestKey)
    })
  metadataBatchRequests.set(requestKey, request)
  return request
}

export function resolveMediaAssetOriginalURL(assetId: number): string {
  return resolveApiUrl(getGetUserMediaAssetOriginalContentUrl(String(assetId)))
}

export function resolveMediaAssetDerivativeURL(
  assetId: number,
  tier: 'thumb' | 'preview' | 'large-preview' | 'processing' | 'processing-input' = 'large-preview',
  format: 'webp' | 'jpeg' | 'png' = 'webp',
): string {
  return resolveApiUrl(getGetUserMediaAssetDerivativeContentUrl(String(assetId), tier, format))
}

export async function resolveMediaAssetProcessingURL(assetId: number): Promise<string> {
  const maxWaitMs = 15_000
  const initialPollIntervalMs = 400
  const maxPollIntervalMs = 2_000
  const startedAt = Date.now()
  let force = false
  let pollIntervalMs = initialPollIntervalMs
  while (true) {
    const metadata = await getMediaAssetMetadata(assetId, {
      ensureTier: force ? undefined : 'processing-input',
      force,
    })
    const variant = metadata.derivatives['processing-input']
    const status = variant?.status ?? 'pending'
    if (status === 'ready' && variant?.url) {
      // The metadata URL is a signed storage URL. Returning the API content
      // endpoint here would require the browser session Cookie, which the
      // backend Worker does not have when it prepares generation media.
      return variant.url
    }
    if (status === 'failed' || Date.now() - startedAt >= maxWaitMs) {
      throw new MediaAssetDerivativeNotReadyError('processing-input', status)
    }
    await new Promise((resolve) => globalThis.setTimeout(resolve, pollIntervalMs))
    force = true
    pollIntervalMs = Math.min(maxPollIntervalMs, pollIntervalMs * 2)
  }
}

function normalizeTier(value: unknown): MediaAssetDeliveryTier {
  return value === 'original' ||
    value === 'thumb' ||
    value === 'preview' ||
    value === 'large-preview' ||
    value === 'processing' ||
    value === 'processing-input'
    ? value
    : 'preview'
}

function resolveVariant(metadata: MediaAssetDeliveryMetadata, tier: MediaAssetDeliveryTier) {
  return tier === 'original' ? metadata.original : metadata.derivatives[tier]
}

export function useMediaAssetDelivery(
  assetId: MaybeRefOrGetter<number | null | undefined>,
  tier: MaybeRefOrGetter<MediaAssetDeliveryTier>,
  options: { enabled?: MaybeRefOrGetter<boolean> } = {},
) {
  const resolvedAssetId = computed(() => {
    const value = Number(toValue(assetId))
    return Number.isInteger(value) && value > 0 ? value : null
  })
  const resolvedTier = computed(() => normalizeTier(toValue(tier)))
  const isEnabled = computed(() => toValue(options.enabled) !== false)
  const metadata = ref<MediaAssetDeliveryMetadata | null>(null)
  const url = ref('')
  const status = ref<MediaAssetDeliveryStatus>('idle')
  const error = ref<unknown>(null)
  let requestToken = 0
  let abortController: AbortController | null = null
  let unsubscribeInvalidation: (() => void) | null = null
  let refreshTimer = 0
  let retryTimer = 0
  let retryDelayMs = 800

  function reset() {
    window.clearTimeout(refreshTimer)
    window.clearTimeout(retryTimer)
    retryTimer = 0
    retryDelayMs = 800
    metadata.value = null
    url.value = ''
    status.value = 'idle'
    error.value = null
  }

  function scheduleRefresh(
    nextUrl: string,
    currentAssetId: number,
    currentTier: MediaAssetDeliveryTier,
  ) {
    window.clearTimeout(refreshTimer)
    const expiresAt = parseSignedURLExpiry(nextUrl)
    if (!expiresAt) return
    refreshTimer = window.setTimeout(
      () => void load(currentAssetId, currentTier, true),
      Math.max(30_000, expiresAt - Date.now() - SIGNED_URL_REFRESH_SKEW_MS),
    )
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
      reset()
      if (currentAssetId === null || !enabled) return

      unsubscribeInvalidation = subscribeAssetInvalidation(currentAssetId, () => {
        if (token !== requestToken) return
        void load(currentAssetId, currentTier, true)
      })
      void load(currentAssetId, currentTier)
    },
    { immediate: true },
  )

  async function load(currentAssetId: number, currentTier: MediaAssetDeliveryTier, force = false) {
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
        ensureTier:
          currentTier === 'large-preview' || currentTier === 'processing' || currentTier === 'processing-input'
            ? currentTier
            : undefined,
      })
      if (token !== requestToken || controller.signal.aborted) return
      metadata.value = nextMetadata
      const variant = resolveVariant(nextMetadata, currentTier)
      const nextStatus = (variant?.status || nextMetadata.status) as MediaAssetDeliveryStatus
      status.value = nextStatus
      url.value = variant?.status === 'ready' && variant.url ? resolveAssetUrl(variant.url) : ''
      if (url.value) {
        retryDelayMs = 800
        scheduleRefresh(url.value, currentAssetId, currentTier)
      } else if (nextStatus === 'pending' || nextStatus === 'processing') {
        window.clearTimeout(retryTimer)
        retryTimer = window.setTimeout(() => {
          retryTimer = 0
          retryDelayMs = Math.min(2_000, retryDelayMs * 2)
          void load(currentAssetId, currentTier, true)
        }, retryDelayMs)
      }
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
      window.clearTimeout(refreshTimer)
      window.clearTimeout(retryTimer)
      unsubscribeInvalidation?.()
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
