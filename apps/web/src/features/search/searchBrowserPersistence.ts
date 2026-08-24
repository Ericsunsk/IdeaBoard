/** Browser-only persistence primitives for identity-scoped Search preferences. */
export function searchSessionStorage(): Storage | null {
  if (typeof window === 'undefined' || typeof window.sessionStorage === 'undefined') return null
  return window.sessionStorage
}

export function searchScopedStorageKey(key: string, scope = 'anonymous'): string {
  return `${key}:${scope || 'anonymous'}`
}
