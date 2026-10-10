import type { SearchPage } from '../domain/models'
import { api } from '../../../api/client'
import { resolveContractData, type ContractSuccessEnvelope } from '../../../api/contractKernel'
import { stripUndefined } from '../../../api/stripUndefined'
import { i18n } from '../../../core/i18n'

// Search boundary (issue #809).
//
// `searchInspiration` (GET `/search`, public inspiration) stays on the
// shared envelope through the central `api.request`: that public route is
// alive (see `registerPublicRoutes`) and anonymous-capable by design —
// the member-only workspace search (`searchWorkspace`, workspace-scoped,
// `ws_` required) is not a substitute. Domain shapes stay owned by
// `../domain/models`.
//
// TODO(2026-09-26, public inspiration gap): GET `/search` has no operation
// in `apps/api/openapi.public.yaml`, so no generated hook exists and this
// hand-rolled GET stays. Adopt the hook once the spec gains the operation;
// do not run `api:generate` from this task (out of scope).

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
  const request = resolveContractData<SearchPage>(
    api.request<ContractSuccessEnvelope<SearchPage>>(
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
