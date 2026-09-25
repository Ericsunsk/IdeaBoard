import { api } from '../../../shared/api/client'
import {
  buildContractRequestHeaders,
  parseContractEnvelope,
  requireContractData,
} from '../../../shared/api/contractKernel'
import { i18n } from '../../../shared/i18n'
import { normalizeSearchQuery } from '../domain/queryPolicy'

// Workspace-scoped search resource (issue #806).
//
// Talks to the v1 endpoints under `/api/v1/workspaces/{id}/search`
// through the central `api.request` client (envelope `{ data, meta }` /
// `{ error: { code, message, details, request_id } }`). Search UI must
// consume this module, never generated code directly.
//
// Migration note: `feed.ts` / `useSearchSession` still read the legacy
// `search` + `user` generated clients. They move onto this module once
// `generated-query/public` regenerates from `openapi.public.yaml`
// (regeneration TODO, owned by the #802-804 merger follow-up); legacy
// routes are removed in #809. Query length validation stays
// server-owned: over-limit queries surface as 422 validation_failed
// (never a silent empty page).

const WORKSPACE_ID_RE = /^ws_[A-Za-z0-9_-]+$/
const LIBRARY_ASSET_ID_RE = /^lasset_[A-Za-z0-9_-]+$/

function assertWorkspaceId(value: string): string {
  const trimmed = value.trim()
  if (!WORKSPACE_ID_RE.test(trimmed)) throw new Error('Invalid workspace id: expected ws_...')
  return trimmed
}

export function assertWorkspaceLibraryAssetId(value: string): string {
  const trimmed = value.trim()
  if (!LIBRARY_ASSET_ID_RE.test(trimmed)) {
    throw new Error('Invalid library asset id: expected lasset_...')
  }
  return trimmed
}

function encodeId(value: string): string {
  return encodeURIComponent(value)
}

export function workspaceSearchSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/search`
}

export function workspaceVisionQueriesSuffix(workspaceId: string): string {
  return `${workspaceSearchSuffix(workspaceId)}/vision-queries`
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requiredString(raw: unknown, field: string): string {
  if (typeof raw !== 'string' || !raw.trim()) throw new Error(`Invalid search payload: ${field}`)
  return raw
}

function optionalString(raw: unknown): string | undefined {
  return typeof raw === 'string' && raw ? raw : undefined
}

function requiredNumber(raw: unknown, field: string): number {
  if (typeof raw !== 'number' || !Number.isFinite(raw)) {
    throw new Error(`Invalid search payload: ${field}`)
  }
  return raw
}

export type WorkspaceSearchProvider = 'unsplash' | 'pexels'

export function mapWorkspaceSearchProvider(value: unknown): WorkspaceSearchProvider {
  if (value === 'unsplash' || value === 'pexels') return value
  throw new Error('Invalid search payload: provider')
}

export type WorkspaceSearchResult = {
  provider: WorkspaceSearchProvider
  sourceId: string
  title?: string | undefined
  author?: string | undefined
  sourceUrl: string
  previewUrl: string
  originalUrl: string
  width: number
  height: number
}

export function toWorkspaceSearchResult(raw: unknown): WorkspaceSearchResult {
  if (!isRecord(raw)) throw new Error('Invalid search result payload')
  return {
    provider: mapWorkspaceSearchProvider(raw.provider),
    sourceId: requiredString(raw.source_id, 'source_id'),
    title: optionalString(raw.title),
    author: optionalString(raw.author),
    sourceUrl: requiredString(raw.source_url, 'source_url'),
    previewUrl: requiredString(raw.preview_url, 'preview_url'),
    originalUrl: requiredString(raw.original_url, 'original_url'),
    width: requiredNumber(raw.width, 'width'),
    height: requiredNumber(raw.height, 'height'),
  }
}

export type WorkspaceSearchPage = {
  items: WorkspaceSearchResult[]
  nextCursor: string
  hasMore: boolean
}

export type WorkspaceVisionQuery = {
  operationId: string
  status: string
  query: string
  replayed?: boolean | undefined
  createdAt: string
}

export function toWorkspaceVisionQuery(raw: unknown): WorkspaceVisionQuery {
  if (!isRecord(raw)) throw new Error('Invalid vision query payload')
  const operationId = requiredString(raw.operation_id, 'operation_id')
  if (!/^visq_[A-Za-z0-9_-]+$/.test(operationId)) {
    throw new Error('Invalid vision query payload: operation_id')
  }
  return {
    operationId,
    status: requiredString(raw.status, 'status'),
    query: requiredString(raw.query, 'query'),
    replayed: raw.replayed === true ? true : undefined,
    createdAt: requiredString(raw.created_at, 'created_at'),
  }
}

export type WorkspaceSearchRequestOptions = {
  requestId?: string | undefined
  idempotencyKey?: string | undefined
  signal?: AbortSignal | null | undefined
}

function workspaceSearchHeaders(
  options: WorkspaceSearchRequestOptions,
): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: options.requestId,
    idempotencyKey: options.idempotencyKey,
  })
}

export type WorkspaceSearchInput = {
  workspaceId: string
  query: string
  providers?: WorkspaceSearchProvider[] | undefined
  cursor?: string | undefined
  pageSize?: number | undefined
}

export type WorkspaceVisionQueryInput = {
  workspaceId: string
  userText?: string | undefined
  libraryAssetIds?: string[] | undefined
}

export const workspaceSearchApi = {
  search: async (
    input: WorkspaceSearchInput,
    options: WorkspaceSearchRequestOptions = {},
  ): Promise<WorkspaceSearchPage> => {
    const query = new URLSearchParams()
    query.set('q', normalizeSearchQuery(input.query))
    for (const provider of input.providers ?? []) query.append('provider', provider)
    if (input.cursor) query.set('cursor', input.cursor)
    if (input.pageSize != null) query.set('page_size', String(input.pageSize))
    const response = await api.request<unknown>(
      `${workspaceSearchSuffix(input.workspaceId)}?${query}`,
      {
        method: 'GET',
        headers: workspaceSearchHeaders(options),
        signal: options.signal,
      },
    )
    const parsed = parseContractEnvelope<unknown[]>(response)
    if (!parsed.ok) {
      const error = new Error(parsed.error.message || i18n.global.t('search.loadFailed')) as Error & {
        code?: string | undefined
      }
      error.code = parsed.error.code
      throw error
    }
    if (!Array.isArray(parsed.data)) throw new Error(i18n.global.t('search.loadFailed'))
    const meta = parsed.meta ?? {}
    const nextCursor = typeof meta.next_cursor === 'string' ? meta.next_cursor : ''
    return {
      items: parsed.data.map(toWorkspaceSearchResult),
      nextCursor,
      hasMore: meta.has_more === true,
    }
  },

  createVisionQuery: async (
    input: WorkspaceVisionQueryInput,
    options: WorkspaceSearchRequestOptions = {},
  ): Promise<WorkspaceVisionQuery> => {
    const response = await api.request<unknown>(workspaceVisionQueriesSuffix(input.workspaceId), {
      method: 'POST',
      headers: workspaceSearchHeaders(options),
      body: JSON.stringify({
        data: {
          ...(input.userText == null ? {} : { user_text: input.userText }),
          ...((input.libraryAssetIds ?? []).length === 0
            ? {}
            : { library_asset_ids: (input.libraryAssetIds ?? []).map(assertWorkspaceLibraryAssetId) }),
        },
      }),
      signal: options.signal,
    })
    return toWorkspaceVisionQuery(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('search.visionSearchFailed'),
      ),
    )
  },
}
