import { resolveApiUrl } from '../../core/config/runtimeUrls'
import { requireWorkspaceId as requireSharedWorkspaceId } from '../../core/utils/workspaceId'
import { readMediaWorkspaceId } from './mediaWorkspaceScope'
import {
  workspaceMediaAssetContentSuffix,
  workspaceMediaAssetDerivativeSuffix,
  type WorkspaceMediaDerivativeVariant,
} from './upload'

// Media delivery URL seam (Public Data Plane, `/api/v1`).
//
// Pure URL parsing/generation only: no HTTP, no metadata cache, no
// invalidation subscriptions, no Vue lifecycle. Browser URL builders mount
// the canonical `/api/v1/workspaces/{workspace_id}/media-assets/...`
// suffixes owned by `./upload` (single source of truth, validated opaque
// ids). Callers pass bare `asset_...` ids; the workspace resolves per call
// from the `features/workspace` selection store and fails closed (throws
// with guidance) when unset — never an inferred workspace.

export class MediaWorkspaceUnavailableError extends Error {
  constructor() {
    super('Media workspace is unavailable. Select a workspace before loading media.')
    this.name = 'MediaWorkspaceUnavailableError'
  }
}

/** Fail-closed workspace resolution: throws before any URL is built when unset. */
export function requireMediaWorkspaceId(explicit?: string | null): string {
  return requireSharedWorkspaceId(
    explicit?.trim() ? explicit : readMediaWorkspaceId(),
    () => new MediaWorkspaceUnavailableError(),
  )
}

/** Canonical delivery identity: string `asset_...`. No numeric legacy. */
export type MediaAssetId = string

export type MediaAssetDeliveryTier = WorkspaceMediaDerivativeVariant | 'original'

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
  tier: WorkspaceMediaDerivativeVariant = 'large-preview',
): string {
  const normalized = assetId.trim()
  return resolveApiUrl(
    workspaceMediaAssetDerivativeSuffix(requireMediaWorkspaceId(), normalized, tier),
  )
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
