import { resolveAssetUrl } from '../core/config/runtimeUrls'
import { MEDIA_METADATA_TTL_MS } from '../core/config/timing'
import { subscribeAuthStateReset } from '../core/utils/auth'
import {
  batchGetWorkspaceMediaAssets as batchGetWorkspaceMediaAssetsTransport,
  getWorkspaceMediaAsset,
} from './generated-query/public/workspace-media/workspace-media'
import type { MediaAsset } from './generated-query/public/model/mediaAsset'
import { subscribeWorkspaceMediaInvalidation } from './workspaceEventStream'
import { i18n } from '../core/i18n'
import {
  requireMediaWorkspaceId,
  type MediaAssetDeliveryTier,
  type MediaAssetId,
} from './mediaAssetUrls'

// Workspace media-asset metadata transport (Public Data Plane, `/api/v1`).
//
// HTTP, metadata cache, and invalidation only: no URL building (see
// `./mediaAssetUrls`) and no Vue lifecycle (see `./useMediaAssetDelivery`).
//
// Metadata reads go through the generated `public/workspace-media`
// `getWorkspaceMediaAsset` (members only). Delivery URLs are same-origin
// API routes (not expiring signed storage URLs), so metadata freshness is
// a plain TTL; per-asset workspace-stream invalidation handles updates
// inside the window.
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
// - Invalidation rides the versioned workspace stream
//   (`media.asset_ready` / `media.asset_archived`); the deleted
//   `/api/user/events` legacy stream is gone (backend #809 hard cut).

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

export type MediaAssetDeliveryStatus =
  'idle' | 'loading' | 'pending' | 'processing' | 'ready' | 'failed' | 'error'

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
const metadataBatchControllers = new Set<AbortController>()
let metadataIdentityEpoch = 0
const invalidationListeners = new Map<string, Set<() => void>>()

export function normalizeMediaAssetId(value: MediaAssetId | null | undefined): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : null
}

function metadataCacheKey(workspaceId: string, assetId: string): string {
  return `${workspaceId}:${assetId}`
}

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
    return () => releaseWorkspaceMediaInvalidation(key, existing)
  }
  const stop = subscribeWorkspaceMediaInvalidation(key, (assetId) => {
    notifyAssetInvalidated(key, assetId)
  })
  const entry = { stop, count: 1 }
  workspaceMediaSubscriptions.set(key, entry)
  return () => releaseWorkspaceMediaInvalidation(key, entry)
}

function releaseWorkspaceMediaInvalidation(
  workspaceId: string,
  expected: WorkspaceMediaSubscription,
): void {
  const entry = workspaceMediaSubscriptions.get(workspaceId)
  if (entry !== expected) return
  entry.count -= 1
  if (entry.count <= 0) {
    workspaceMediaSubscriptions.delete(workspaceId)
    entry.stop()
  }
}

/**
 * Delivery-internal per-asset invalidation hook. The Vue binding
 * (`./useMediaAssetDelivery`) subscribes here so transport stays free of
 * component lifecycle; the workspace stream notifies through
 * `notifyAssetInvalidated` above.
 */
export function subscribeAssetInvalidation(
  assetId: MediaAssetId,
  listener: () => void,
): () => void {
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

/**
 * Production identity-switch reset: drop every cached metadata entry, abort
 * in-flight reads (a late resolve must not repopulate the cache under the
 * new identity), and release workspace-stream invalidation subscriptions so
 * the next identity opens fresh transports. Component-level per-asset
 * listeners (`subscribeAssetInvalidation`) are untouched: they re-resolve on
 * their next load.
 */
export function resetWorkspaceMediaDeliveryOnIdentitySwitch(): void {
  metadataIdentityEpoch += 1
  for (const request of metadataRequests.values()) {
    try {
      request.controller.abort()
    } catch {
      // Abort must never break the identity switch.
    }
  }
  metadataRequests.clear()
  for (const controller of metadataBatchControllers) {
    try {
      controller.abort()
    } catch {
      // Abort must never break the identity switch.
    }
  }
  metadataBatchControllers.clear()
  metadataCache.clear()
  for (const subscription of workspaceMediaSubscriptions.values()) {
    try {
      subscription.stop()
    } catch {
      // Unsubscribe must never break the identity switch.
    }
  }
  workspaceMediaSubscriptions.clear()
}

// Silent 401 paths (`clearStoredAuthState`) bypass the session transition,
// so delivery subscribes to the auth-state reset directly instead of the
// feature layer importing it (`api` never imports concrete features).
subscribeAuthStateReset(resetWorkspaceMediaDeliveryOnIdentitySwitch)

function mapDeliveryVariantStatus(value: unknown, field: string): MediaAssetDeliveryVariantStatus {
  if (value === 'pending' || value === 'processing' || value === 'ready' || value === 'failed') {
    return value
  }
  throw new Error(`Invalid media asset payload: ${field}`)
}

function toDeliveryVariant(tier: string, variant: unknown): MediaAssetDeliveryVariant | null {
  if (!variant || typeof variant !== 'object') return null
  const record = variant as { status?: unknown; url?: unknown }
  const url = typeof record.url === 'string' && record.url.trim() ? record.url.trim() : undefined
  return {
    status: mapDeliveryVariantStatus(record.status, `derivatives.${tier}`),
    ...(url == null ? {} : { url }),
  }
}

function toDeliveryDerivatives(derivatives: MediaAsset['derivatives']): {
  [key: string]: MediaAssetDeliveryVariant
} {
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

function assertMetadataBatchActive(epoch: number, signal: AbortSignal): void {
  if (epoch !== metadataIdentityEpoch || signal.aborted) throw createMetadataAbortError()
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
  const requestEpoch = metadataIdentityEpoch
  const controller = new AbortController()
  const onCallerAbort = () => controller.abort()
  if (options.signal?.aborted) return Promise.reject(createMetadataAbortError())
  options.signal?.addEventListener('abort', onCallerAbort)
  metadataBatchControllers.add(controller)
  try {
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
      assertMetadataBatchActive(requestEpoch, controller.signal)
      const chunk = missingIndexes.slice(at, at + WORKSPACE_MEDIA_BATCH_MAX_ITEMS)
      const chunkIds = chunk.map((index) => ids[index] as string)
      const envelope = await batchGetWorkspaceMediaAssetsTransport(
        workspace,
        { data: { asset_ids: chunkIds } },
        { signal: controller.signal },
      )
      assertMetadataBatchActive(requestEpoch, controller.signal)
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
  } finally {
    metadataBatchControllers.delete(controller)
    options.signal?.removeEventListener('abort', onCallerAbort)
  }
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
