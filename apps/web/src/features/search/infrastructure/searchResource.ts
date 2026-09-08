import type {
  SearchCatalogProvider,
  SearchPage,
  VisionReference,
  VisionExtraction,
} from '../domain/searchModels'
import { searchInspiration as searchPublicInspiration } from '../../../shared/api/generated-query/search/search'
import {
  extractUserVisionSearch,
  recordSearchImageFailure,
} from '../../../shared/api/generated-query/user/user'
import { resolveApiData, resolveApiOk } from '../../../shared/api/apiResponse'
import { stripUndefined } from '../../../shared/api/stripUndefined'
import { i18n } from '../../../shared/i18n'

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
    searchPublicInspiration(
      stripUndefined({
        q: input.query || undefined,
        cursor: input.cursor,
        limit: input.limit,
      }),
      { signal: input.signal },
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
  return resolveApiData(
    extractUserVisionSearch(
      stripUndefined({
        images: input.references.flatMap((reference) => (reference.file ? [reference.file] : [])),
        library_asset_ids: input.references.flatMap((reference) =>
          reference.libraryAssetID ? [reference.libraryAssetID] : [],
        ),
        external_references: input.references.flatMap((reference) =>
          reference.externalReference ? [JSON.stringify(reference.externalReference)] : [],
        ),
        user_text: input.userText || undefined,
      }),
      {
        signal: input.signal,
        timeout: 40_000,
        headers: { 'Idempotency-Key': input.idempotencyKey },
      },
    ),
    i18n.global.t('search.visionSearchFailed'),
  )
}

export async function reportSearchImageFailure(provider: SearchCatalogProvider): Promise<void> {
  try {
    await resolveApiOk(
      recordSearchImageFailure({ provider }),
      i18n.global.t('search.reportImageFailureFailed'),
    )
  } catch (error) {
    if (import.meta.env.DEV) console.debug('search image failure metric unavailable', error)
  }
}
