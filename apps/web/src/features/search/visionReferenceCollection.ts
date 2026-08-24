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
  const pendingKeys = ref(new Set<string>())
  const pendingBytes = new Map<string, number>()

  const hasPending = computed(() => pendingKeys.value.size > 0)
  const slotCount = computed(() => {
    const keys = new Set(references.value.map((reference) => reference.key))
    for (const key of pendingKeys.value) keys.add(key)
    return keys.size
  })
  const canAdd = computed(() => slotCount.value < MAX_REFERENCE_COUNT)

  function has(key: string): boolean {
    return references.value.some((reference) => reference.key === key)
  }

  function isPending(key: string): boolean {
    return pendingKeys.value.has(key)
  }

  function currentBytes(): number {
    const storedBytes = references.value.reduce(
      (total, reference) => total + (reference.file?.size ?? 0),
      0,
    )
    const reservedBytes = [...pendingBytes.values()].reduce((total, size) => total + size, 0)
    return storedBytes + reservedBytes
  }

  function add(reference: VisionReference): boolean {
    if (has(reference.key) || isPending(reference.key) || !canAdd.value) return false
    references.value = [...references.value, reference]
    return true
  }

  function begin(key: string): boolean {
    if (has(key) || isPending(key) || !canAdd.value) return false
    pendingKeys.value = new Set([...pendingKeys.value, key])
    return true
  }

  function appendPreview(key: string, previewUrl: string): boolean {
    if (!isPending(key) || has(key)) return false
    references.value = [...references.value, { key, previewUrl }]
    return true
  }

  function reserveBytes(key: string, size: number): boolean {
    if (!isPending(key) || size <= 0 || currentBytes() + size > MAX_REFERENCE_BYTES) return false
    pendingBytes.set(key, size)
    return true
  }

  function materialize(key: string, file: File): boolean {
    if (!isPending(key) || !has(key)) return false
    references.value = references.value.map((reference) =>
      reference.key === key ? { ...reference, file } : reference,
    )
    pendingBytes.delete(key)
    return true
  }

  function finish(key: string): void {
    pendingBytes.delete(key)
    const next = new Set(pendingKeys.value)
    next.delete(key)
    pendingKeys.value = next
  }

  function fail(key: string): boolean {
    const wasSelected = isPending(key) || has(key)
    const removed = references.value.find((reference) => reference.key === key)
    if (removed) revokeLocalPreview(removed)
    references.value = references.value.filter((reference) => reference.key !== key)
    finish(key)
    return wasSelected
  }

  function remove(key: string): boolean {
    const reference = references.value.find((item) => item.key === key)
    if (!reference && !isPending(key)) return false
    if (reference) revokeLocalPreview(reference)
    references.value = references.value.filter((item) => item.key !== key)
    finish(key)
    return true
  }

  function clear(): void {
    for (const reference of references.value) revokeLocalPreview(reference)
    references.value = []
    pendingKeys.value = new Set()
    pendingBytes.clear()
  }

  function take(): VisionReference[] {
    const current = references.value
    references.value = []
    pendingKeys.value = new Set()
    pendingBytes.clear()
    return current
  }

  return {
    references,
    hasPending,
    slotCount,
    canAdd,
    has,
    isPending,
    currentBytes,
    add,
    begin,
    appendPreview,
    reserveBytes,
    materialize,
    finish,
    fail,
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
