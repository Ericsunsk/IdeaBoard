import { toMediaDisplay } from '../../../../core/media/mediaDisplay'
import type { SearchPage } from '../../core/models'
import { searchInspiration as searchInspirationTransport } from '../../../../api/generated-query/public/workspace-search/workspace-search'
import { withSignal } from '../../../../core/utils/request'
import { resolveContractData } from '@/api/http/contractKernel'
import { stripUndefined } from '@/api/http/stripUndefined'
import { i18n } from '../../../../core/i18n'

// Search boundary (issue #809).
//
// `searchInspiration` (GET `/api/v1/search`, public inspiration) calls the
// generated `public/workspace-search` transport (`/api/v1/...` via `publicMutator`).
// That public route is anonymous-capable by design — the member-only workspace
// search (`searchWorkspace`, workspace-scoped, `ws_` required) is not a substitute.
// Domain shapes stay owned by `../core/models`.

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
  const params = stripUndefined({
    q: input.query || undefined,
    cursor: input.cursor,
    limit: input.limit,
  })
  const request = resolveContractData<SearchPage>(
    searchInspirationTransport(params, withSignal(input.signal)),
    i18n.global.t('search.loadFailed'),
  )
    .then((page) => ({
      ...page,
      results: page.results.map((result) => ({
        ...result,
        display: toMediaDisplay(result.display),
      })),
    }))
    .finally(() => {
      if (inFlightRequests.get(key)?.promise === request) inFlightRequests.delete(key)
    })
  inFlightRequests.set(key, { promise: request, signal: input.signal })
  return request
}
