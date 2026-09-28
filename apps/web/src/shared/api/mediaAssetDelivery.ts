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
import { MEDIA_METADATA_TTL_MS } from '../config/timing'
import {
  batchGetWorkspaceMediaAssets as batchGetWorkspaceMediaAssetsTransport,
  getWorkspaceMediaAsset,
} from './generated-query/public/workspace-media/workspace-media'
import type { MediaAsset } from './generated-query/public/model/mediaAsset'
import {
  workspaceMediaAssetContentSuffix,
  workspaceMediaAssetDerivativeSuffix,
  type WorkspaceMediaDerivativeVariant,
} from './upload'
import { subscribeWorkspaceMediaInvalidation } from './workspaceEventStream'
import { createPollDelay } from '../utils/polling'
import { i18n } from '../i18n'
import { requireWorkspaceId as requireSharedWorkspaceId } from '../utils/workspaceId'
import {
  readMediaWorkspaceId,
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from './mediaWorkspaceScope'

export { resetMediaWorkspaceIdResolver, setMediaWorkspaceIdResolver }

// Workspace media-asset delivery boundary (Public Data Plane, `/api/v1`).
//
// Metadata reads go through the generated `public/workspace-media`
// `getWorkspaceMediaAsset` (members only); browser URL builders mount the
// canonical `/api/v1/workspaces/{workspace_id}/media-assets/...` suffixes
// owned by `./upload` (single source of truth, validated opaque ids).
// Callers pass bare `asset_...` ids; the workspace resolves per call from
// the `features/workspace` selection store and fail-closed (throw with
// guidance) when unset — never an inferred workspace.
//
// Notes on v1 semantics:
// - v1 statuses are `pending | ready` only (no `processing`/`failed`
//   vocabulary); binary reads on non-ready assets stay 409 upstream.
// - Bounded batch metadata reads ride the generated
//   `batchGetWorkspaceMediaAssets` hook (`POST
//   /api/v1/workspaces/{id}/media-assets/batch`, up to 50 ids, request
//   order, same asset DTOs as the single read) through
//   `getWorkspaceMediaAssetMetadataBatch` below; results hydrate the same
//   cache the delivery composable feeds.
// - TODO(2026-09-26, processing-input tier gap): v1 has no
//   `processing-input` derivative tier in the generated `MediaAsset` DTO,
//   so that metadata lookup stays strict (throws
//   `MediaAssetDerivativeNotReadyError`) so generation inputs never
//   silently change.
// - Invalidation rides the versioned workspace stream
//   (`media.asset_ready` / `media.asset_archived`); the deleted
//   `/api/user/events` legacy stream is gone (backend #809 hard cut).

export class MediaWorkspaceUnavailableError extends Error {
  constructor() {
    super('Media workspace is unavailable. Select a workspace before loading media.')
    this.name = 'MediaWorkspaceUnavailableError'
  }
}

/** Fail-closed workspace resolution: throws before any network call when unset. */
export function requireMediaWorkspaceId(explicit?: string | null): string {
  return requireSharedWorkspaceId(
    explicit?.trim() ? explicit : readMediaWorkspaceId(),
    () => new MediaWorkspaceUnavailableError(),
  )
}

export type MediaAssetDeliveryVariantStatus = 'pending' | 'processing' | 'ready' | 'failed'

export type MediaAssetDeliveryVariant = {
  status: MediaAssetDeliveryVariantStatus
  format?: string
  url?: string
  width?: number
  height?: number
  size_bytes?: number
  error_code?: string
}

export type MediaAssetDeliveryMetadata = {
  asset_id: string
  status: string
  mime_type: string
  width: number
  height: number
  size_bytes: number
  original: MediaAssetDeliveryVariant
  derivatives: { [key: string]: MediaAssetDeliveryVariant }
}

export type ApiResponseMediaAssetMetadata = { data: MediaAssetDeliveryMetadata }

export type MediaAssetDeliveryTier =
  'thumb' | 'preview' | 'large-preview' | 'processing-input' | 'original'
export type MediaAssetDeliveryStatus =
  'idle' | 'loading' | 'pending' | 'processing' | 'ready' | 'failed' | 'error'

/** Canonical delivery identity: string `asset_...`. No numeric legacy. */
export type MediaAssetId = string

export class MediaAssetDerivativeNotReadyError extends Error {
  readonly tier: MediaAssetDeliveryTier
  readonly status: string

  constructor(tier: MediaAssetDeliveryTier, status: string) {
    super(`Media asset ${tier} source is ${status}`)
    this.name = 'MediaAssetDerivativeNotReadyError'
    this.tier = tier
    this.status = status
  }
}

type CachedMetadata = {
  value: MediaAssetDeliveryMetadata
  expiresAt: number
}

const METADATA_CACHE_TTL_MS = MEDIA_METADATA_TTL_MS

const metadataCache = new Map<string, CachedMetadata>()
type InFlightMetadataRequest = {
  promise: Promise<MediaAssetDeliveryMetadata>
  controller: AbortController
  force: boolean
}
const metadataRequests = new Map<string, InFlightMetadataRequest>()
const invalidationListeners = new Map<string, Set<() => void>>()

export function normalizeMediaAssetId(value: MediaAssetId | null | undefined): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : null
}

function metadataCacheKey(workspaceId: string, assetId: string): string {
  return `${workspaceId}:${assetId}`
}

// Delivery URLs are same-origin API routes (not expiring signed storage
// URLs), so metadata freshness is a plain TTL; per-asset workspace-stream
// invalidation handles updates inside the window.
function resolveMetadataExpiry(_metadata: MediaAssetDeliveryMetadata): number {
  return Date.now() + METADATA_CACHE_TTL_MS
}

function clearCachedMetadata(workspaceId: string | null, assetId: string): void {
  const key = normalizeMediaAssetId(assetId) ?? ''
  if (!key) return
  if (workspaceId) {
    metadataCache.delete(metadataCacheKey(workspaceId, key))
    return
  }
  for (const cacheKey of [...metadataCache.keys()]) {
    if (cacheKey === key || cacheKey.endsWith(`:${key}`)) metadataCache.delete(cacheKey)
  }
}

function notifyAssetInvalidated(workspaceId: string | null, assetId: MediaAssetId) {
  const key = normalizeMediaAssetId(assetId) ?? ''
  clearCachedMetadata(workspaceId, key)
  for (const listener of invalidationListeners.get(key) ?? []) listener()
}

// Workspace media invalidation (issue #807). Workspace-scoped callers
// subscribe per workspace id; the versioned stream (`media.asset_ready` /
// `media.asset_archived`) invalidates the same metadata cache the delivery
// composable feeds. Entries are refcounted so overlapping composables
// share one transport.
type WorkspaceMediaSubscription = {
  stop: () => void
  count: number
}

const workspaceMediaSubscriptions = new Map<string, WorkspaceMediaSubscription>()

export function ensureWorkspaceMediaInvalidation(workspaceId: string): () => void {
  const key = workspaceId.trim()
  if (key.length === 0) throw new Error('Invalid workspace id: expected ws_...')
  const existing = workspaceMediaSubscriptions.get(key)
  if (existing) {
    existing.count += 1
    return () => releaseWorkspaceMediaInvalidation(key)
  }
  const stop = subscribeWorkspaceMediaInvalidation(key, (assetId) => {
    notifyAssetInvalidated(key, assetId)
  })
  workspaceMediaSubscriptions.set(key, { stop, count: 1 })
  return () => releaseWorkspaceMediaInvalidation(key)
}

function releaseWorkspaceMediaInvalidation(workspaceId: string): void {
  const entry = workspaceMediaSubscriptions.get(workspaceId)
  if (!entry) return
  entry.count -= 1
  if (entry.count <= 0) {
    workspaceMediaSubscriptions.delete(workspaceId)
    entry.stop()
  }
}

function subscribeAssetInvalidation(assetId: MediaAssetId, listener: () => void): () => void {
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
  }
}

function getCachedMetadata(
  workspaceId: string,
  assetId: MediaAssetId,
): MediaAssetDeliveryMetadata | null {
  const key = normalizeMediaAssetId(assetId) ?? ''
  const cached = metadataCache.get(metadataCacheKey(workspaceId, key))
  if (!cached) return null
  if (cached.expiresAt <= Date.now()) {
    metadataCache.delete(metadataCacheKey(workspaceId, key))
    return null
  }
  return cached.value
}

export function invalidateMediaAssetMetadata(assetId?: MediaAssetId) {
  if (assetId === undefined) {
    metadataCache.clear()
    return
  }
  notifyAssetInvalidated(null, assetId)
}

function mapDeliveryVariantStatus(value: unknown, field: string): MediaAssetDeliveryVariantStatus {
  if (value === 'pending' || value === 'processing' || value === 'ready' || value === 'failed') {
    return value
  }
  throw new Error(`Invalid media asset payload: ${field}`)
}

function toDeliveryVariant(
  tier: string,
  variant: unknown,
): MediaAssetDeliveryVariant | null {
  if (!variant || typeof variant !== 'object') return null
  const record = variant as { status?: unknown; url?: unknown }
  const url =
    typeof record.url === 'string' && record.url.trim() ? record.url.trim() : undefined
  return {
    status: mapDeliveryVariantStatus(record.status, `derivatives.${tier}`),
    ...(url == null ? {} : { url }),
  }
}

function toDeliveryDerivatives(
  derivatives: MediaAsset['derivatives'],
): { [key: string]: MediaAssetDeliveryVariant } {
  const mapped: { [key: string]: MediaAssetDeliveryVariant } = {}
  for (const [tier, variant] of Object.entries(derivatives ?? {})) {
    const delivery = toDeliveryVariant(tier, variant)
    if (delivery) mapped[tier] = delivery
  }
  return mapped
}

function assertDeliveryContentUrl(contentUrl: string, assetId: string): string {
  const trimmed = contentUrl?.trim()
  if (!trimmed) throw new Error('Invalid media asset payload: content_url')
  // Content URLs carry asset_... only; storage keys never leave the server.
  if (!trimmed.includes(assetId) || /storage[_-]?key/i.test(trimmed)) {
    throw new Error('Invalid media asset payload: content_url')
  }
  return trimmed
}

function toDeliveryMetadata(asset: MediaAsset): MediaAssetDeliveryMetadata {
  const id = normalizeMediaAssetId(asset.id)
  if (!id) throw new Error('Invalid media asset payload: id')
  if (!Number.isFinite(asset.width) || !Number.isFinite(asset.height)) {
    throw new Error('Invalid media asset payload: dimensions')
  }
  if (!Number.isFinite(asset.size_bytes)) {
    throw new Error('Invalid media asset payload: size_bytes')
  }
  const contentUrl = assertDeliveryContentUrl(asset.content_url, id)
  return {
    asset_id: id,
    status: mapDeliveryVariantStatus(asset.status, 'status'),
    mime_type: asset.mime_type,
    width: asset.width,
    height: asset.height,
    size_bytes: asset.size_bytes,
    original: { status: 'ready', url: contentUrl },
    derivatives: toDeliveryDerivatives(asset.derivatives),
  }
}

function createMetadataAbortError(): DOMException {
  return new DOMException('Media asset metadata read aborted.', 'AbortError')
}

// Deduped reads share one transport, but each caller's AbortSignal stays
// local: aborting one consumer rejects only its own read while the shared
// fetch continues for joiners. Previously the first caller's signal rode
// the shared fetch, so one component unmount aborted every joined load.
function joinMetadataRequest(
  shared: Promise<MediaAssetDeliveryMetadata>,
  signal?: AbortSignal,
): Promise<MediaAssetDeliveryMetadata> {
  if (!signal) return shared
  if (signal.aborted) return Promise.reject(createMetadataAbortError())
  return new Promise<MediaAssetDeliveryMetadata>((resolve, reject) => {
    const onAbort = () => reject(createMetadataAbortError())
    signal.addEventListener('abort', onAbort, { once: true })
    shared.then(
      (metadata) => {
        signal.removeEventListener('abort', onAbort)
        resolve(metadata)
      },
      (error) => {
        signal.removeEventListener('abort', onAbort)
        reject(error)
      },
    )
  })
}

/**
 * Reads one workspace media asset and projects it onto the delivery
 * metadata shape shared delivery consumers (cache, composable, the
 * poll-free Library Canvas resolve) read. Results hydrate the same cache the
 * workspace-stream invalidation feeds.
 */
export async function getWorkspaceMediaAssetMetadata(
  workspaceId: string | null | undefined,
  assetId: MediaAssetId,
  options: {
    signal?: AbortSignal | undefined
    force?: boolean | undefined
  } = {},
): Promise<MediaAssetDeliveryMetadata> {
  const workspace = requireMediaWorkspaceId(workspaceId)
  const normalized = normalizeMediaAssetId(assetId)
  if (!normalized) throw new Error(i18n.global.t('common.errors.mediaInvalid'))
  const requestKey = metadataCacheKey(workspace, normalized)
  // An already-aborted caller never touches shared state: no cache read,
  // no dedup join, no stale cancellation.
  if (options.signal?.aborted) return Promise.reject(createMetadataAbortError())
  if (!options.force) {
    const cached = getCachedMetadata(workspace, assetId)
    if (cached) return cached
    const existing = metadataRequests.get(requestKey)
    if (existing) return joinMetadataRequest(existing.promise, options.signal)
  } else {
    // `force` bypasses the cache. Replace a non-force in-flight read, while
    // concurrent force callers share the same fresh transport.
    const stale = metadataRequests.get(requestKey)
    if (stale?.force) return joinMetadataRequest(stale.promise, options.signal)
    if (stale) {
      stale.controller.abort()
      metadataRequests.delete(requestKey)
    }
  }

  const controller = new AbortController()
  const request = getWorkspaceMediaAsset(workspace, normalized, { signal: controller.signal })
    .then((envelope) => toDeliveryMetadata(envelope.data))
    .then((metadata) => {
      // A force-superseded read must not poison the cache when its
      // transport ignores the abort and still resolves.
      if (metadataRequests.get(requestKey)?.promise === request) {
        metadataCache.set(requestKey, {
          value: metadata,
          expiresAt: resolveMetadataExpiry(metadata),
        })
      }
      return metadata
    })
    .finally(() => {
      if (metadataRequests.get(requestKey)?.promise === request) {
        metadataRequests.delete(requestKey)
      }
    })
  metadataRequests.set(requestKey, { promise: request, controller, force: options.force === true })
  return joinMetadataRequest(request, options.signal)
}

/**
 * Workspace-implicit metadata read for call sites holding bare `asset_`
 * ids (canvas surfaces). The workspace resolves from the selection store
 * and fail-closed when unset; prefer `getWorkspaceMediaAssetMetadata`
 * where the workspace is already known.
 */
export async function getMediaAssetMetadata(
  assetId: MediaAssetId,
  options: {
    signal?: AbortSignal | undefined
    force?: boolean | undefined
  } = {},
): Promise<MediaAssetDeliveryMetadata> {
  return getWorkspaceMediaAssetMetadata(null, assetId, options)
}

/** Backend batch bound (`asset_ids` min 1, max 50 per call). */
export const WORKSPACE_MEDIA_BATCH_MAX_ITEMS = 50

/**
 * Bounded batch metadata read over the generated
 * `batchGetWorkspaceMediaAssets` hook (same asset DTOs as the single
 * read, request order). Cache-first unless `force`: cached entries are
 * served without transport and misses ride one transport per 50-id chunk,
 * hydrating the same cache the workspace-stream invalidation feeds. A
 * malformed, unknown, or cross-workspace id converges the whole batch to
 * 404 `media_not_found` (backend contract); the error propagates untouched
 * so 404-vs-403 stays caller-visible.
 */
export async function getWorkspaceMediaAssetMetadataBatch(
  workspaceId: string | null | undefined,
  assetIds: readonly MediaAssetId[],
  options: {
    signal?: AbortSignal | undefined
    force?: boolean | undefined
  } = {},
): Promise<MediaAssetDeliveryMetadata[]> {
  const workspace = requireMediaWorkspaceId(workspaceId)
  const normalized = assetIds.map((id) => normalizeMediaAssetId(id))
  if (normalized.length === 0 || normalized.some((id) => id === null)) {
    throw new Error(i18n.global.t('common.errors.mediaInvalid'))
  }
  const ids = normalized.filter((id): id is string => id !== null)
  const results = new Array<MediaAssetDeliveryMetadata | undefined>(ids.length)
  const missingIndexes: number[] = []
  ids.forEach((id, index) => {
    if (!options.force) {
      const cached = getCachedMetadata(workspace, id)
      if (cached) {
        results[index] = cached
        return
      }
    }
    missingIndexes.push(index)
  })
  for (let at = 0; at < missingIndexes.length; at += WORKSPACE_MEDIA_BATCH_MAX_ITEMS) {
    const chunk = missingIndexes.slice(at, at + WORKSPACE_MEDIA_BATCH_MAX_ITEMS)
    const chunkIds = chunk.map((index) => ids[index] as string)
    const envelope = await batchGetWorkspaceMediaAssetsTransport(
      workspace,
      { data: { asset_ids: chunkIds } },
      { ...(options.signal == null ? {} : { signal: options.signal }) },
    )
    const items = envelope?.data?.items
    if (!Array.isArray(items) || items.length !== chunk.length) {
      throw new Error(i18n.global.t('common.errors.mediaInvalid'))
    }
    chunk.forEach((resultIndex, position) => {
      const item = items[position] as MediaAsset | undefined
      if (!item) throw new Error(i18n.global.t('common.errors.mediaInvalid'))
      const metadata = toDeliveryMetadata(item)
      const cacheId = ids[resultIndex] as string
      metadataCache.set(metadataCacheKey(workspace, cacheId), {
        value: metadata,
        expiresAt: resolveMetadataExpiry(metadata),
      })
      results[resultIndex] = metadata
    })
  }
  return results.filter((item): item is MediaAssetDeliveryMetadata => item !== undefined)
}

export function resolveMediaAssetOriginalURL(assetId: MediaAssetId): string {
  const normalized = assetId.trim()
  return resolveApiUrl(workspaceMediaAssetContentSuffix(requireMediaWorkspaceId(), normalized))
}

/**
 * Format-agnostic derivative delivery URL. The backend negotiates the ready
 * physical encoding server-side; callers never specify a format. The path
 * comes from the `./upload` suffix builders (never hand-concatenated) so
 * delivery URLs cannot drift from the OpenAPI contract.
 */
export function resolveMediaAssetDerivativeURL(
  assetId: MediaAssetId,
  tier: 'thumb' | 'preview' | 'large-preview' | 'processing-input' = 'large-preview',
): string {
  const normalized = assetId.trim()
  return resolveApiUrl(
    workspaceMediaAssetDerivativeSuffix(
      requireMediaWorkspaceId(),
      normalized,
      tier as WorkspaceMediaDerivativeVariant,
    ),
  )
}

/**
 * Resolve the full-resolution export source for generation. v1 carries no
 * `processing-input` derivative tier; the source is the `original` content
 * derived from the asset payload, so this throws
 * `MediaAssetDerivativeNotReadyError` while the asset itself is not ready —
 * generation inputs must never silently change.
 */
export async function resolveMediaAssetProcessingURL(assetId: MediaAssetId): Promise<string> {
  const metadata = await getMediaAssetMetadata(assetId)
  if (metadata.status !== 'ready' || !metadata.original?.url) {
    throw new MediaAssetDerivativeNotReadyError('original', metadata.status)
  }
  return resolveAssetUrl(metadata.original.url)
}

// ---------------------------------------------------------------------------
// Workspace media delivery URLs (issue #803, Public Data Plane).
//
// Canonical browser URLs for workspace-bound assets live under
// `/api/v1/workspaces/{workspace_id}/media-assets/...` and carry
// `asset_...` only (storage keys never leave the server). Path suffixes
// are owned by `./upload` (single source of truth, validated opaque
// ids); this module only mounts them onto the runtime base URL.

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
        releaseWorkspaceSubscription = ensureWorkspaceMediaInvalidation(
          requireMediaWorkspaceId(),
        )
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
