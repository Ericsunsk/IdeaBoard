import { libraryAssetKey, type LibraryAssetView } from '../library/libraryResource'
import type { VisionReference } from './searchResource'

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
