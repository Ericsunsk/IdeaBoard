import type {
  SearchPage,
  VisionReference,
  VisionExtraction,
} from '../domain/models'
import { api } from '../../../shared/api/client'
import { resolveApiData, type ApiResponseWithData } from '../../../shared/api/apiResponse'
import { stripUndefined } from '../../../shared/api/stripUndefined'
import { i18n } from '../../../shared/i18n'

// Search boundary (issue #809). The generated `search`/`user` clients are
// gone but the backend still serves `GET /search` (public inspiration) and
// `POST /api/user/search/vision-extract` with the shared envelope, so this
// boundary transports through the central `api.request`. Domain shapes stay
// owned by `../domain/models`.

export function generateRandomInspirationCursor(): string {
  const page = Math.floor(Math.random() * 8) + 1
  const payload = JSON.stringify({ unsplash: page, pexels: page })
  if (typeof btoa !== 'undefined') {
    return btoa(payload).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
  }
  return ''
}
const inFlightRequests = new Map<
  string,
  { promise: Promise<SearchPage>; signal?: AbortSignal | undefined }
>()

function searchInspirationUrl(query: {
  q?: string | undefined
  cursor?: string | undefined
  limit?: number | undefined
}): string {
  const search = new URLSearchParams()
  const cleaned = stripUndefined({
    q: query.q || undefined,
    cursor: query.cursor,
    limit: query.limit == null ? undefined : String(query.limit),
  })
  for (const [key, value] of Object.entries(cleaned)) search.append(key, value)
  const queryString = search.toString()
  return queryString ? `/search?${queryString}` : '/search'
}

export async function searchInspiration(input: {
  query: string
  cursor?: string | undefined
  limit?: number | undefined
  signal?: AbortSignal | undefined
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
    api.request<ApiResponseWithData<SearchPage>>(
      searchInspirationUrl({ q: input.query, cursor: input.cursor, limit: input.limit }),
      {
        method: 'GET',
        ...(input.signal == null ? {} : { signal: input.signal }),
      },
    ),
    i18n.global.t('search.loadFailed'),
  ).finally(() => {
    if (inFlightRequests.get(key)?.promise === request) inFlightRequests.delete(key)
  })
  inFlightRequests.set(key, { promise: request, signal: input.signal })
  return request
}

export async function extractVisionSearch(input: {
  references: VisionReference[]
  userText: string
  idempotencyKey: string
  signal?: AbortSignal
}): Promise<VisionExtraction> {
  const formData = new FormData()
  for (const file of input.references.flatMap((reference) =>
    reference.file ? [reference.file] : [],
  )) {
    formData.append('images', file)
  }
  for (const assetID of input.references.flatMap((reference) =>
    reference.libraryAssetID ? [reference.libraryAssetID] : [],
  )) {
    formData.append('library_asset_ids', String(assetID))
  }
  for (const external of input.references.flatMap((reference) =>
    reference.externalReference ? [JSON.stringify(reference.externalReference)] : [],
  )) {
    formData.append('external_references', external)
  }
  if (input.userText) formData.append('user_text', input.userText)
  return resolveApiData(
    api.request<ApiResponseWithData<VisionExtraction>>('/user/search/vision-extract', {
      method: 'POST',
      body: formData,
      timeout: 40_000,
      headers: { 'Idempotency-Key': input.idempotencyKey },
      ...(input.signal == null ? {} : { signal: input.signal }),
    }),
    i18n.global.t('search.visionSearchFailed'),
  )
}
