import type { VisionReference } from './searchModels'

export const MAX_REFERENCE_COUNT = 4
export const MAX_REFERENCE_BYTES = 10 * 1024 * 1024

export function partitionVisionReferences(initial: VisionReference[]) {
  const accepted: VisionReference[] = []
  const discarded: VisionReference[] = []
  const keys = new Set<string>()
  for (const reference of initial) {
    if (!keys.has(reference.key) && accepted.length < MAX_REFERENCE_COUNT) {
      keys.add(reference.key)
      accepted.push(reference)
    } else if (!accepted.some((item) => item.previewUrl === reference.previewUrl)) {
      discarded.push(reference)
    }
  }
  return { accepted, discarded }
}

export function createRequestFingerprint(query: string, references: VisionReference[]): string {
  return JSON.stringify({
    userText: query,
    references: references.map((reference) => ({
      key: reference.key,
      libraryAssetID: reference.libraryAssetID ?? null,
      externalReference: reference.externalReference ?? null,
      file: reference.file
        ? {
            name: reference.file.name,
            size: reference.file.size,
            type: reference.file.type,
            lastModified: reference.file.lastModified,
          }
        : null,
    })),
  })
}
