import {
  extractUserVisionSearch,
  recordSearchImageFailure,
  searchUserInspiration,
} from '../../shared/api/generated-query/user/user'
import type {
  SearchResults,
  SearchResult,
  SearchCatalogProvider,
} from '../../shared/api/generated-query/model'
import { resolveApiData, resolveApiOk } from '../../shared/api/apiResponse'

export type SearchPage = SearchResults
export const MAX_SEARCH_QUERY_RUNES = 200

export function normalizeSearchQuery(raw: string): string {
  return raw.trim().replace(/\s+/g, ' ')
}

export function isSearchQueryWithinLimit(query: string): boolean {
  return Array.from(query).length <= MAX_SEARCH_QUERY_RUNES
}

export function generateRandomInspirationCursor(): string {
  const page = Math.floor(Math.random() * 8) + 1
  const payload = JSON.stringify({ unsplash: page, pexels: page })
  if (typeof btoa !== 'undefined') {
    return btoa(payload).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
  }
  return ''
}

const inFlightRequests = new Map<string, { promise: Promise<SearchPage>; signal?: AbortSignal }>()

export async function searchInspiration(input: {
  query: string
  cursor?: string
  limit?: number
  signal?: AbortSignal
}): Promise<SearchPage> {
  if (input.signal?.aborted) {
    throw new DOMException('The request was aborted.', 'AbortError')
  }
  const key = JSON.stringify({
    query: input.query,
    cursor: input.cursor ?? '',
    limit: input.limit ?? 0,
  })
  const inFlight = inFlightRequests.get(key)
  if (inFlight?.signal?.aborted) {
    inFlightRequests.delete(key)
  } else if (inFlight) {
    return inFlight.promise
  }
  const request = resolveApiData(
    searchUserInspiration(
      {
        q: input.query || undefined,
        cursor: input.cursor,
        limit: input.limit,
      },
      { signal: input.signal },
    ),
    '搜索灵感失败',
  ).finally(() => {
    if (inFlightRequests.get(key)?.promise === request) inFlightRequests.delete(key)
  })
  inFlightRequests.set(key, { promise: request, signal: input.signal })
  return request
}

export type VisionReference = { file?: File; libraryAssetID?: number; previewUrl: string; key: string }

export async function extractVisionSearch(input: {
  references: VisionReference[]
  userText: string
  signal?: AbortSignal
}) {
  return resolveApiData(
    extractUserVisionSearch(
      {
        images: input.references.flatMap((reference) => (reference.file ? [reference.file] : [])),
        library_asset_ids: input.references.flatMap((reference) =>
          reference.libraryAssetID ? [reference.libraryAssetID] : [],
        ),
        user_text: input.userText || undefined,
      },
      { signal: input.signal },
    ),
    '视觉搜索失败',
  )
}

export async function reportSearchImageFailure(provider: SearchCatalogProvider): Promise<void> {
  try {
    await resolveApiOk(recordSearchImageFailure({ provider }), '上报图片加载失败指标失败')
  } catch (error) {
    if (import.meta.env.DEV) console.debug('search image failure metric unavailable', error)
  }
}

export type { SearchResult }
