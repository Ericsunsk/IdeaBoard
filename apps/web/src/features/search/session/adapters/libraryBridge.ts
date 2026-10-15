import { resolveMediaDisplayUrl } from '../../../../core/media/mediaDisplay'
import { libraryAssetKey, type WorkspaceLibraryAsset } from '../../../library'
import type { SearchResult, VisionReference } from '../../core/models'

export function searchReferenceKey(provider: string, sourceID: string): string {
  return libraryAssetKey(provider, sourceID)
}

export function toVisionReference(asset: WorkspaceLibraryAsset): VisionReference {
  return {
    libraryAssetID: asset.id,
    previewUrl: resolveMediaDisplayUrl(asset.display, 256),
    key: searchReferenceKey(asset.provider, asset.sourceId),
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
