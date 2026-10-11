/** Browser-only persistence primitives for identity-scoped Search preferences. */
export function searchScopedStorageKey(key: string, scope = 'anonymous'): string {
  return `${key}:${scope || 'anonymous'}`
}
