const MAX_SEARCH_QUERY_RUNES = 200

export function normalizeSearchQuery(raw: string): string {
  return raw.trim().replace(/\s+/g, ' ')
}

export function isSearchQueryWithinLimit(query: string): boolean {
  return Array.from(query).length <= MAX_SEARCH_QUERY_RUNES
}
