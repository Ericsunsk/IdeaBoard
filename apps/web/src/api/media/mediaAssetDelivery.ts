/**
 * Workspace media-asset delivery boundary (Public Data Plane, `/api/v1`).
 *
 * Stable import seam for existing consumers. The implementation is split
 * by responsibility (issue #861):
 *
 * - `./mediaAssetUrls` — pure URL parsing/generation (no HTTP, no cache,
 *   no invalidation, no Vue).
 * - `./mediaAssetMetadata` — HTTP, metadata cache, and workspace-stream
 *   invalidation transport (no URL building, no Vue lifecycle).
 * - `./useMediaAssetDelivery` — Vue lifecycle binding only (watchers,
 *   scope disposal, retry timers).
 *
 * New code should import from the focused module it needs; this barrel
 * stays so the current consumer set keeps one stable path.
 */

// Pure delivery-URL builders and workspace resolution.
export {
  MediaWorkspaceUnavailableError,
  requireMediaWorkspaceId,
  resolveMediaAssetDerivativeURL,
  resolveMediaAssetOriginalURL,
  resolveWorkspaceMediaAssetDerivativeURL,
  resolveWorkspaceMediaAssetOriginalURL,
  type MediaAssetDeliveryTier,
  type MediaAssetId,
} from './mediaAssetUrls'

// HTTP/cache/invalidation transport.
export {
  MediaAssetDerivativeNotReadyError,
  WORKSPACE_MEDIA_BATCH_MAX_ITEMS,
  getMediaAssetMetadata,
  getWorkspaceMediaAssetMetadata,
  getWorkspaceMediaAssetMetadataBatch,
  invalidateMediaAssetMetadata,
  normalizeMediaAssetId,
  resolveMediaAssetProcessingURL,
  ensureWorkspaceMediaInvalidation,
  type ApiResponseMediaAssetMetadata,
  type MediaAssetDeliveryMetadata,
  type MediaAssetDeliveryStatus,
  type MediaAssetDeliveryVariant,
  type MediaAssetDeliveryVariantStatus,
} from './mediaAssetMetadata'

export { resetMediaWorkspaceIdResolver, setMediaWorkspaceIdResolver } from './mediaWorkspaceScope'
