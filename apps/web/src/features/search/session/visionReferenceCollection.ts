import { MAX_REFERENCE_COUNT, partitionVisionReferences } from '../core/visionReferencePolicy'
import { computed, ref } from 'vue'
import type { VisionReference } from '../core/models'

function revokeLocalPreview(reference: VisionReference): void {
  if (reference.previewUrl.startsWith('blob:')) {
    URL.revokeObjectURL(reference.previewUrl)
  }
}

export function createVisionReferenceCollection(initial: VisionReference[] = []) {
  const { accepted, discarded } = partitionVisionReferences(initial)
  for (const reference of discarded) revokeLocalPreview(reference)
  const references = ref<VisionReference[]>(accepted)

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

export { MAX_REFERENCE_COUNT } from '../core/visionReferencePolicy'
