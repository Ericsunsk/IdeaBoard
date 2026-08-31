import type { VisionExternalReference, VisionReference } from './searchResource'
import { searchScopedStorageKey, searchSessionStorage } from './searchBrowserPersistence'

const VISION_REF_CACHE_KEY = 'ideaboard:search:vision_refs'

export type CachedVisionReference = {
  key: string
  previewUrl: string
  libraryAssetID?: number
  externalReference?: VisionExternalReference
}

/** Library-backed and External references are reloadable; local File/blob state remains in memory. */
export function getCachedVisionReferences(scope = 'anonymous'): CachedVisionReference[] {
  try {
    const raw = searchSessionStorage()?.getItem(searchScopedStorageKey(VISION_REF_CACHE_KEY, scope))
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
    const session = searchSessionStorage()
    if (!session) return
    const storageKey = searchScopedStorageKey(VISION_REF_CACHE_KEY, scope)
    if (serializable.length > 0) session.setItem(storageKey, JSON.stringify(serializable))
    else session.removeItem(storageKey)
  } catch {
    // Persistence is optional and must not affect the active query.
  }
}

export function clearCachedVisionReferences(scope = 'anonymous'): void {
  try {
    searchSessionStorage()?.removeItem(searchScopedStorageKey(VISION_REF_CACHE_KEY, scope))
  } catch {
    // Persistence is optional.
  }
}
