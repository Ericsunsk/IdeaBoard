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

/** Canonical delivery identity: string `asset_...`. No numeric legacy. */
export type MediaAssetId = string

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

const metadataCache = new Map<string, CachedMetadata>()
const metadataRequests = new Map<string, Promise<MediaAssetDeliveryMetadata>>()
const metadataBatchRequests = new Map<
  string,
  Promise<ReadonlyMap<string, MediaAssetDeliveryMetadata>>
>()
const ensuredTiers = new Set<string>()
const invalidationListeners = new Map<string, Set<() => void>>()
let stopMediaEventSubscription: (() => void) | null = null

export function normalizeMediaAssetId(value: MediaAssetId | null | undefined): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : null
}

function normalizeMediaAssetKey(value: MediaAssetId): string {
  return value.trim()
}

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

function notifyAssetInvalidated(assetId: MediaAssetId) {
  const key = normalizeMediaAssetKey(assetId)
  metadataCache.delete(key)
  for (const listener of invalidationListeners.get(key) ?? []) listener()
}

function ensureMediaEventSubscription() {
  if (stopMediaEventSubscription) return
  stopMediaEventSubscription = subscribeUserEvent('media.derivative.updated', (event) => {
    try {
      const payload = JSON.parse(event.data) as { media_asset_id?: unknown }
      const raw = payload.media_asset_id
      const assetId = typeof raw === 'string' ? raw.trim() : ''
      if (assetId.length > 0) notifyAssetInvalidated(assetId)
    } catch {
      return
    }
  })
}

function subscribeAssetInvalidation(assetId: MediaAssetId, listener: () => void): () => void {
  ensureMediaEventSubscription()
  const key = normalizeMediaAssetKey(assetId)
  let listeners = invalidationListeners.get(key)
  if (!listeners) {
    listeners = new Set()
    invalidationListeners.set(key, listeners)
  }
  listeners.add(listener)
  return () => {
    listeners?.delete(listener)
    if (listeners?.size === 0) invalidationListeners.delete(key)
    if (invalidationListeners.size === 0 && stopMediaEventSubscription) {
      stopMediaEventSubscription()
      stopMediaEventSubscription = null
    }
  }
}

function getCachedMetadata(assetId: MediaAssetId): MediaAssetDeliveryMetadata | null {
  const key = normalizeMediaAssetKey(assetId)
  const cached = metadataCache.get(key)
  if (!cached) return null
  if (cached.expiresAt <= Date.now()) {
    metadataCache.delete(key)
    return null
  }
  return cached.value
}

export function invalidateMediaAssetMetadata(assetId?: MediaAssetId) {
  if (assetId === undefined) {
    metadataCache.clear()
    ensuredTiers.clear()
    return
  }
  notifyAssetInvalidated(assetId)
}

export async function getMediaAssetMetadata(
  assetId: MediaAssetId,
  options: {
    signal?: AbortSignal | undefined
    force?: boolean | undefined
    ensureTier?: MediaAssetDeliveryTier | undefined
  } = {},
): Promise<MediaAssetDeliveryMetadata> {
  const normalized = normalizeMediaAssetId(assetId)
  if (!normalized) throw new Error(i18n.global.t('common.errors.mediaInvalid'))
  const ensureKey = options.ensureTier ? `${normalized}:${options.ensureTier}` : ''
  const requestKey = ensureKey || normalized
  const needsEnsure = Boolean(ensureKey && !ensuredTiers.has(ensureKey))
  if (!options.force && !needsEnsure) {
    const cached = getCachedMetadata(assetId)
    if (cached) return cached
  }
  const existing = metadataRequests.get(requestKey)
  if (existing) return existing

  const request = api
    .request<ApiResponseMediaAssetMetadata>(
      `${getGetUserMediaAssetMetadataUrl(normalized)}${
        options.ensureTier ? `?ensure_tier=${encodeURIComponent(options.ensureTier)}` : ''
      }`,
      { method: 'GET', signal: options.signal },
    )
    .then((response) =>
      requireApiData(response, i18n.global.t('common.errors.mediaInfoLoadFailed')),
    )
    .then((metadata) => {
      metadataCache.set(normalized, { value: metadata, expiresAt: resolveMetadataExpiry(metadata) })
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
 * cache used by useMediaAssetDelivery. Results are keyed by string asset ID;
 * assets the user cannot access are omitted by the API and therefore absent.
 */
export async function getMediaAssetMetadataBatch(
  assetIds: readonly MediaAssetId[],
  options: {
    signal?: AbortSignal | undefined
    force?: boolean | undefined
    ensureTier?:
      | Extract<MediaAssetDeliveryTier, 'large-preview' | 'processing' | 'processing-input'>
      | undefined
  } = {},
): Promise<ReadonlyMap<string, MediaAssetDeliveryMetadata>> {
  const ids = Array.from(
    new Set(
      assetIds
        .map((value) => normalizeMediaAssetId(value))
        .filter((value): value is string => value !== null),
    ),
  )
  if (ids.length === 0) return new Map()
  if (ids.length > 100) throw new Error('Too many media assets requested')
  const sortedIDs = [...ids].sort()
  const requestKey = `${sortedIDs.join(',')}:${options.ensureTier ?? ''}`
  if (!options.force) {
    const cached = new Map<string, MediaAssetDeliveryMetadata>()
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
        asset_ids: sortedIDs,
        ...(options.ensureTier ? { ensure_tier: options.ensureTier } : {}),
      }),
    })
    .then((response) =>
      requireApiData(response, i18n.global.t('common.errors.mediaInfoLoadFailed')),
    )
    .then((data) => {
      const result = new Map<string, MediaAssetDeliveryMetadata>()
      for (const metadata of data.items ?? []) {
        const id = normalizeMediaAssetId(metadata.asset_id)
        if (!id) continue
        metadataCache.set(id, { value: metadata, expiresAt: resolveMetadataExpiry(metadata) })
        if (options.ensureTier) ensuredTiers.add(`${id}:${options.ensureTier}`)
        result.set(id, metadata)
      }
      return result as ReadonlyMap<string, MediaAssetDeliveryMetadata>
    })
    .finally(() => {
      if (metadataBatchRequests.get(requestKey) === request)
        metadataBatchRequests.delete(requestKey)
    })
  metadataBatchRequests.set(requestKey, request)
  return request
}

export function resolveMediaAssetOriginalURL(assetId: MediaAssetId): string {
  return resolveApiUrl(getGetUserMediaAssetOriginalContentUrl(assetId.trim()))
}

/**
 * Format-agnostic derivative delivery URL. The backend negotiates the ready
 * physical encoding server-side; callers never specify a format.
 */
export function resolveMediaAssetDerivativeURL(
  assetId: MediaAssetId,
  tier: 'thumb' | 'preview' | 'large-preview' | 'processing' | 'processing-input' = 'large-preview',
): string {
  const normalized = assetId.trim()
  return resolveApiUrl(
    `/user/media-assets/${encodeURIComponent(normalized)}/derivatives/${encodeURIComponent(tier)}/content`,
  )
}

export async function resolveMediaAssetProcessingURL(assetId: MediaAssetId): Promise<string> {
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
    currentAssetId: string,
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

  function scheduleDeliveryRetry(
    currentAssetId: string,
    currentTier: MediaAssetDeliveryTier,
  ): void {
    window.clearTimeout(retryTimer)
    retryTimer = window.setTimeout(() => {
      retryTimer = 0
      retryDelayMs = Math.min(2_000, retryDelayMs * 2)
      void load(currentAssetId, currentTier, true)
    }, retryDelayMs)
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
      retryDelayMs = 800
      scheduleRefresh(url.value, currentAssetId, currentTier)
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
        ensureTier:
          currentTier === 'large-preview' ||
          currentTier === 'processing' ||
          currentTier === 'processing-input'
            ? currentTier
            : undefined,
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
