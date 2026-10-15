import { isRecord } from '../../../../core/utils/guards'
import type { CachedVisionReference } from '../../ports/persistencePort'
import { searchScopedStorageKey } from './browserPersistence'
import {
  readSessionStorageItem,
  removeSessionStorageItem,
  writeSessionStorageItem,
} from '../../../../core/utils/storage'

const VISION_REF_CACHE_KEY = 'ideaboard:search:vision_refs'

function isCachedVisionReference(value: unknown): value is CachedVisionReference {
  if (!isRecord(value) || typeof value.key !== 'string' || typeof value.previewUrl !== 'string') {
    return false
  }
  if (value.previewExpiresAt != null && typeof value.previewExpiresAt !== 'string') return false
  if (value.libraryAssetID != null && typeof value.libraryAssetID !== 'string') return false
  if (value.externalReference != null) {
    const external = value.externalReference
    if (
      !isRecord(external) ||
      typeof external.provider !== 'string' ||
      typeof external.id !== 'string' ||
      typeof external.preview_url !== 'string'
    )
      return false
  }
  return Boolean(value.libraryAssetID || value.externalReference)
}

/** Library-backed and External references are reloadable; local File/blob state remains in memory. */
export function getCachedVisionReferences(scope = 'anonymous'): CachedVisionReference[] {
  try {
    const raw = readSessionStorageItem(searchScopedStorageKey(VISION_REF_CACHE_KEY, scope))
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(isCachedVisionReference).map((reference) => ({
      key: reference.key,
      previewUrl: reference.previewUrl,
      previewExpiresAt: reference.previewExpiresAt,
      libraryAssetID: reference.libraryAssetID,
      externalReference: reference.externalReference,
    }))
  } catch {
    return []
  }
}

export function setCachedVisionReferences(
  references: CachedVisionReference[],
  scope = 'anonymous',
): void {
  try {
    const storageKey = searchScopedStorageKey(VISION_REF_CACHE_KEY, scope)
    if (references.length > 0) writeSessionStorageItem(storageKey, JSON.stringify(references))
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
