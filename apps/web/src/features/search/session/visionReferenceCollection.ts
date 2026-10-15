import { MAX_REFERENCE_COUNT, partitionVisionReferences } from '../core/visionReferencePolicy'
import { computed, ref } from 'vue'
import { resolveApiUrl } from '../../../core/config/runtimeUrls'
import type { SearchResult, VisionReference } from '../core/models'
import type { CachedVisionReference } from '../ports/persistencePort'
import { searchReferenceKey, searchResultToVisionReference } from './adapters/libraryBridge'

function revokeLocalPreview(reference: VisionReference): void {
  if (reference.previewUrl.startsWith('blob:')) {
    URL.revokeObjectURL(reference.previewUrl)
  }
}

function normalizePresentation(reference: VisionReference): VisionReference {
  if (
    reference.file ||
    (reference.previewUrl.startsWith('blob:') &&
      !reference.libraryAssetID &&
      !reference.externalReference)
  )
    return reference
  const isCurrent =
    !reference.externalReference ||
    (reference.previewExpiresAt != null && Date.parse(reference.previewExpiresAt) > Date.now())
  return {
    ...reference,
    previewUrl:
      isCurrent && reference.previewUrl.startsWith(resolveApiUrl('/')) ? reference.previewUrl : '',
  }
}

export function createVisionReferenceCollection(initial: VisionReference[] = []) {
  const { accepted, discarded } = partitionVisionReferences(initial)
  for (const reference of discarded) revokeLocalPreview(reference)
  const references = ref<VisionReference[]>(accepted.map(normalizePresentation))

  const persistableReferences = computed<CachedVisionReference[]>(() =>
    references.value
      .filter(
        (reference) =>
          !reference.file &&
          !reference.previewUrl.startsWith('blob:') &&
          (reference.libraryAssetID || reference.externalReference),
      )
      .map((reference) => {
        const { key, previewUrl, previewExpiresAt, libraryAssetID, externalReference } =
          normalizePresentation(reference)
        return { key, previewUrl, previewExpiresAt, libraryAssetID, externalReference }
      }),
  )

  const slotCount = computed(() => references.value.length)
  const canAdd = computed(() => slotCount.value < MAX_REFERENCE_COUNT)

  function has(key: string): boolean {
    return references.value.some((reference) => reference.key === key)
  }

  function currentBytes(): number {
    return references.value.reduce((total, reference) => total + (reference.file?.size ?? 0), 0)
  }

  function add(reference: VisionReference): boolean {
    if (has(reference.key) || !canAdd.value) return false
    references.value = [...references.value, normalizePresentation(reference)]
    return true
  }

  function refreshPresentation(results: SearchResult[]): void {
    const byKey = new Map(
      results.map((result) => [searchReferenceKey(result.provider, result.id), result]),
    )
    for (const reference of references.value) {
      if (!reference.externalReference || reference.file) continue
      const fresh = byKey.get(reference.key)
      const presentation = normalizePresentation(
        fresh ? searchResultToVisionReference(fresh) : reference,
      )
      reference.previewUrl = presentation.previewUrl
      reference.previewExpiresAt = presentation.previewExpiresAt
    }
  }

  function remove(key: string): boolean {
    const reference = references.value.find((item) => item.key === key)
    if (!reference) return false
    revokeLocalPreview(reference)
    references.value = references.value.filter((item) => item.key !== key)
    return true
  }

  function clear(): void {
    for (const reference of references.value) revokeLocalPreview(reference)
    references.value = []
  }

  function take(): VisionReference[] {
    const current = references.value
    references.value = []
    return current
  }

  return {
    references,
    persistableReferences,
    refreshPresentation,
    slotCount,
    canAdd,
    has,
    currentBytes,
    add,
    remove,
    clear,
    take,
  }
}

export { MAX_REFERENCE_COUNT } from '../core/visionReferencePolicy'
