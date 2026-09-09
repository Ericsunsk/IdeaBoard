import { libraryAssetKey, type LibraryAssetView } from '../../library'
import type { SearchResult, VisionReference } from '../domain/models'

export function searchReferenceKey(provider: string, sourceID: string): string {
  return libraryAssetKey(provider, sourceID)
}

export function toVisionReference(asset: LibraryAssetView): VisionReference {
  return {
    libraryAssetID: asset.id,
    previewUrl: asset.preview_url,
    key: searchReferenceKey(asset.provider, asset.source_id),
  }
}

export function searchResultToVisionReference(result: SearchResult): VisionReference {
  return {
    externalReference: {
      provider: result.provider,
      id: result.id,
      preview_url: result.preview_url,
    },
    previewUrl: result.thumbnail_url || result.preview_url,
    key: searchReferenceKey(result.provider, result.id),
  }
}
