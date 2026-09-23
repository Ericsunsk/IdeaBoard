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
import {
  workspaceMediaAssetContentSuffix,
  workspaceMediaAssetDerivativeSuffix,
  type WorkspaceMediaDerivativeVariant,
} from './upload'
import { subscribeUserEvent } from './userEventStream'
import { createPollDelay } from '../utils/polling'
import { i18n } from '../i18n'
import {
  getGetUserMediaAssetMetadataUrl,
  getGetUserMediaAssetMetadataBatchUrl,
  getGetUserMediaAssetDerivativeContentUrl,
  getGetUserMediaAssetOriginalContentUrl,
} from './generated-query/user-assets/user-assets'
import type {
  ApiResponseMediaAssetMetadata,
  ApiResponseMediaAssetMetadataBatch,
  MediaAssetDeliveryVariant as GeneratedMediaAssetDeliveryVariant,
  MediaAssetMetadata,
} from './generated-query/model'

export type MediaAssetDeliveryTier =
  'thumb' | 'preview' | 'large-preview' | 'processing-input' | 'original'
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

const metadataCache = new Map<string, CachedMetadata>()
const metadataRequests = new Map<string, Promise<MediaAssetDeliveryMetadata>>()
const metadataBatchRequests = new Map<
  string,
  Promise<ReadonlyMap<string, MediaAssetDeliveryMetadata>>
>()
const invalidationListeners = new Map<string, Set<() => void>>()
let stopMediaEventSubscription: (() => void) | null = null

export function normalizeMediaAssetId(value: MediaAssetId | null | undefined): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : null
}

// Delivery URLs are same-origin API routes (not expiring signed storage
// URLs), so metadata freshness is a plain TTL; per-asset SSE invalidation
// handles updates inside the window.
function resolveMetadataExpiry(_metadata: MediaAssetDeliveryMetadata): number {
  return Date.now() + METADATA_CACHE_TTL_MS
}

function notifyAssetInvalidated(assetId: MediaAssetId) {
  const key = normalizeMediaAssetId(assetId) ?? ''
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
  const key = normalizeMediaAssetId(assetId) ?? ''
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
  const key = normalizeMediaAssetId(assetId) ?? ''
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
    return
  }
  notifyAssetInvalidated(assetId)
}

export async function getMediaAssetMetadata(
  assetId: MediaAssetId,
  options: {
    signal?: AbortSignal | undefined
    force?: boolean | undefined
  } = {},
): Promise<MediaAssetDeliveryMetadata> {
  const normalized = normalizeMediaAssetId(assetId)
  if (!normalized) throw new Error(i18n.global.t('common.errors.mediaInvalid'))
  const requestKey = normalized
  if (!options.force) {
    const cached = getCachedMetadata(assetId)
    if (cached) return cached
  }
  const existing = metadataRequests.get(requestKey)
  if (existing) return existing

  const request = api
    .request<ApiResponseMediaAssetMetadata>(getGetUserMediaAssetMetadataUrl(normalized), {
      method: 'GET',
      signal: options.signal,
    })
    .then((response) =>
      requireApiData(response, i18n.global.t('common.errors.mediaInfoLoadFailed')),
    )
    .then((metadata) => {
      metadataCache.set(normalized, { value: metadata, expiresAt: resolveMetadataExpiry(metadata) })
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
  const requestKey = sortedIDs.join(',')
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
      body: JSON.stringify({ asset_ids: sortedIDs }),
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
 * physical encoding server-side; callers never specify a format. The path
 * comes from the generated client (never hand-concatenated) so legacy
 * delivery URLs cannot drift from the OpenAPI contract.
 */
export function resolveMediaAssetDerivativeURL(
  assetId: MediaAssetId,
  tier: 'thumb' | 'preview' | 'large-preview' | 'processing-input' = 'large-preview',
): string {
  const normalized = assetId.trim()
  return resolveApiUrl(getGetUserMediaAssetDerivativeContentUrl(normalized, tier))
}

/**
 * Resolve the provider-neutral processing input for generation. The tier is
 * materialized synchronously during ingestion, so this is a single metadata
 * read: there is no work to enqueue and nothing to poll for.
 */
export async function resolveMediaAssetProcessingURL(assetId: MediaAssetId): Promise<string> {
  const metadata = await getMediaAssetMetadata(assetId)
  const variant = metadata.derivatives['processing-input']
  const status = variant?.status ?? metadata.status
  if (status === 'ready' && variant?.url) return resolveAssetUrl(variant.url)
  throw new MediaAssetDerivativeNotReadyError('processing-input', status)
}

// ---------------------------------------------------------------------------
// Workspace media delivery URLs (issue #803, Public Data Plane).
//
// Canonical browser URLs for workspace-bound assets live under
// `/api/v1/workspaces/{workspace_id}/media-assets/...` and carry
// `asset_...` only (storage keys never leave the server). Path suffixes
// are owned by `./upload` (single source of truth, validated opaque
// ids); this module only mounts them onto the runtime base URL.
// Board/Task/Library adoption of these URLs is #805/#806.

export function resolveWorkspaceMediaAssetOriginalURL(
  workspaceId: string,
  assetId: MediaAssetId,
): string {
  return resolveApiUrl(workspaceMediaAssetContentSuffix(workspaceId, assetId))
}

export function resolveWorkspaceMediaAssetDerivativeURL(
  workspaceId: string,
  assetId: MediaAssetId,
  tier: WorkspaceMediaDerivativeVariant = 'large-preview',
): string {
  return resolveApiUrl(workspaceMediaAssetDerivativeSuffix(workspaceId, assetId, tier))
}

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
