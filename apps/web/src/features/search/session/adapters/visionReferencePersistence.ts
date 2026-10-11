import type { CachedVisionReference } from '../../ports/persistencePort'
import type { VisionReference } from '../../core/models'
import { searchScopedStorageKey } from './browserPersistence'
import {
  readSessionStorageItem,
  removeSessionStorageItem,
  writeSessionStorageItem,
} from '../../../../core/utils/storage'

const VISION_REF_CACHE_KEY = 'ideaboard:search:vision_refs'

/** Library-backed and External references are reloadable; local File/blob state remains in memory. */
export function getCachedVisionReferences(scope = 'anonymous'): CachedVisionReference[] {
  try {
    const raw = readSessionStorageItem(searchScopedStorageKey(VISION_REF_CACHE_KEY, scope))
    if (!raw) return []
    return (JSON.parse(raw) as CachedVisionReference[]).filter((reference) =>
      Boolean(reference.libraryAssetID || reference.externalReference),
    )
  } catch {
    return []
  }
}

export function setCachedVisionReferences(
  references: VisionReference[],
  scope = 'anonymous',
): void {
  try {
    const serializable = references
      .filter(
        (reference) =>
          !reference.file &&
          !reference.previewUrl.startsWith('blob:') &&
          (reference.libraryAssetID || reference.externalReference),
      )
      .map((reference) => ({
        key: reference.key,
        previewUrl: reference.previewUrl,
        libraryAssetID: reference.libraryAssetID,
        externalReference: reference.externalReference,
      }))
    const storageKey = searchScopedStorageKey(VISION_REF_CACHE_KEY, scope)
    if (serializable.length > 0) writeSessionStorageItem(storageKey, JSON.stringify(serializable))
    else removeSessionStorageItem(storageKey)
  } catch {
    // Persistence is optional and must not affect the active query.
  }
}

export function clearCachedVisionReferences(scope = 'anonymous'): void {
  try {
    removeSessionStorageItem(searchScopedStorageKey(VISION_REF_CACHE_KEY, scope))
  } catch {
    // Persistence is optional.
  }
}
