/** Unique ID helpers — `crypto.randomUUID` first, timestamp+random fallback. */

function randomFallback(dashed: boolean): string {
  const stamp = Date.now().toString(36)
  const random = Math.random().toString(36).slice(2, 12)
  return dashed ? `${stamp}-${random}` : `${stamp}${random}`
}

export function createUniqueSuffix(): string {
  const randomUUID = globalThis.crypto?.randomUUID
  if (typeof randomUUID === 'function') {
    return randomUUID.call(globalThis.crypto).replace(/-/g, '').slice(0, 12)
  }
  return randomFallback(false)
}

/** Full UUID (no truncation) for idempotency keys and other high-entropy needs. */
export function createUniqueId(): string {
  const randomUUID = globalThis.crypto?.randomUUID
  if (typeof randomUUID === 'function') {
    return randomUUID.call(globalThis.crypto)
  }
  return randomFallback(true)
}

/** `prefix + separator + unique suffix`, e.g. `board_9f3a…`. Keeps domain prefix for readability. */
export function createPrefixedId(prefix: string, separator = '_'): string {
  return `${prefix}${separator}${createUniqueSuffix()}`
}
