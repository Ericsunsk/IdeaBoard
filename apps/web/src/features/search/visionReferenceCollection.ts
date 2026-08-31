import { computed, ref } from 'vue'
import type { VisionReference } from './searchResource'

export const MAX_REFERENCE_COUNT = 4
export const MAX_REFERENCE_BYTES = 10 * 1024 * 1024

function revokeLocalPreview(reference: VisionReference): void {
  if (reference.previewUrl.startsWith('blob:')) {
    URL.revokeObjectURL(reference.previewUrl)
  }
}

export function createVisionReferenceCollection(initial: VisionReference[] = []) {
  const references = ref<VisionReference[]>(normalizeInitialReferences(initial))

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
    references.value = [...references.value, reference]
    return true
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

function normalizeInitialReferences(initial: VisionReference[]): VisionReference[] {
  const accepted: VisionReference[] = []
  const keys = new Set<string>()
  for (const reference of initial) {
    const acceptedByKey = !keys.has(reference.key) && accepted.length < MAX_REFERENCE_COUNT
    if (acceptedByKey) {
      keys.add(reference.key)
      accepted.push(reference)
      continue
    }
    const previewStillOwned = accepted.some((item) => item.previewUrl === reference.previewUrl)
    if (!previewStillOwned) revokeLocalPreview(reference)
  }
  return accepted
}

export type VisionReferenceCollection = ReturnType<typeof createVisionReferenceCollection>
