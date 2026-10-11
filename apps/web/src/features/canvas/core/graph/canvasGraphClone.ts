/**
 * Pure structural clone for graph records and snapshots.
 *
 * The domain must not import Vue, yet snapshots handed to the store may be
 * Vue-reactive proxies (the application owns that boundary and passes its
 * refs' values straight through). `structuredClone` rejects such proxies,
 * so this helper clones by transparent property reads instead: proxy traps
 * forward `Object.entries`/`Array` access to the underlying facts, and no
 * framework import is needed. Domain facts are JSON-shaped, so only plain
 * data containers are cloned (class instances are returned by reference and
 * must never appear in graph state).
 */
export function cloneJsonValue<T>(value: T): T {
  if (value === null || value === undefined) return value
  if (typeof value !== 'object') return value
  if (value instanceof Date) return new Date(value.getTime()) as T
  if (value instanceof Map) {
    const next = new Map()
    for (const [key, entry] of value) next.set(cloneJsonValue(key), cloneJsonValue(entry))
    return next as T
  }
  if (value instanceof Set) {
    const next = new Set()
    for (const entry of value) next.add(cloneJsonValue(entry))
    return next as T
  }
  if (Array.isArray(value)) return value.map(cloneJsonValue) as T
  const prototype = Object.getPrototypeOf(value)
  if (prototype !== Object.prototype && prototype !== null) return value
  const next: Record<string, unknown> = {}
  for (const [key, entry] of Object.entries(value)) next[key] = cloneJsonValue(entry)
  return next as T
}
