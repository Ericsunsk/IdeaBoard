import { resolveMediaDisplayUrl } from '../../../../core/media/mediaDisplay'
import { libraryAssetKeyFor, type WorkspaceLibraryAsset } from '../../../library'
import type { SearchResult, VisionReference } from '../../core/models'

export function searchReferenceKey(provider: string, sourceID: string): string {
  // Feed/vision keys are external-only; the provider allowlist is enforced at
  // the search result boundary, so this formats without re-validating.
  return libraryAssetKeyFor({
    source: 'external',
    provider,
    sourceId: sourceID,
  })
}

export function toVisionReference(asset: WorkspaceLibraryAsset): VisionReference {
  return {
    libraryAssetID: asset.id,
    previewUrl: resolveMediaDisplayUrl(asset.display, 256),
    key: libraryAssetKeyFor(asset),
  }
}

export function searchResultToVisionReference(result: SearchResult): VisionReference {
  return {
    externalReference: {
      provider: result.provider,
      id: result.id,
      preview_url: result.preview_url,
    },
    previewUrl: resolveMediaDisplayUrl(result.display, 256),
    previewExpiresAt: result.display?.expires_at,
    key: searchReferenceKey(result.provider, result.id),
  }
}
