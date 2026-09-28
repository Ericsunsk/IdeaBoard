import {
  createWorkspaceVisionQuery as createWorkspaceVisionQueryTransport,
  searchWorkspace as searchWorkspaceTransport,
} from '../../../shared/api/generated-query/public/workspace-search/workspace-search'
import {
  buildContractRequestHeaders,
  requireContractData,
} from '../../../shared/api/contractKernel'
import { i18n } from '../../../shared/i18n'
import { assertWorkspaceId } from '../../../shared/utils/workspaceId'
import { assertOpaqueId, isOpaqueIdWithPrefix } from '../../../shared/utils/opaqueIds'
import { normalizeSearchQuery } from '../domain/queryPolicy'

// Workspace-scoped search resource (issue #806).
//
// Talks to the v1 endpoints under `/api/v1/workspaces/{id}/search`
// through the generated `public/workspace-search` hooks (`/api/v1/...`
// via `publicMutator`). Search UI must consume this module, never
// generated code directly.
//
// Migration note: `feed.ts` / `useSearchSession` still read the legacy
// `search` + `user` generated clients. They move onto this module once
// their call sites migrate; legacy routes are removed in #809. Query
// length validation stays server-owned: over-limit queries surface as
// 422 validation_failed (never a silent empty page).
//
// Transport note: the generated hooks own routes/envelopes; this adapter
// keeps query normalization, opaque-id asserts, idempotency plumbing,
// and error-code mapping that have no generated equivalent.
//
// TODO(2026-09-26, orval array serialization): the generated
// `getSearchWorkspaceUrl` serializes array params with `String(value)`
// (comma-joined, single `provider=`), while the backend reads
// `c.QueryArray("provider")` with no comma split (`ParseV1SearchProviders`
// 422s `unsplash,pexels`). Multi-provider filters therefore fail closed
// below instead of riding the hook. Missing shape: repeated `provider=`
// params in the generated URL helper. Single/empty filters ride the hook
// wire-identical to the retired transport.

export function assertWorkspaceLibraryAssetId(value: string): string {
  return assertOpaqueId(value, 'lasset', 'library asset id')
}

function assertWorkspaceUploadId(value: string): string {
  return assertOpaqueId(value, 'upload', 'workspace upload id')
}

function toGeneratedOptions(options: WorkspaceSearchRequestOptions): {
  headers: Record<string, string>
  signal?: AbortSignal | undefined
} {
  return {
    headers: workspaceSearchHeaders(options),
    ...(options.signal == null ? {} : { signal: options.signal }),
  }
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
  if (!isOpaqueIdWithPrefix(operationId, 'visq')) {
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
  /** Staged workspace upload refs (`upload_...`, stage-then-reference). */
  uploadIds?: string[] | undefined
  /** Allowlisted https image URLs (server-enforced CDN allowlist). */
  imageUrls?: string[] | undefined
}

export const workspaceSearchApi = {
  search: async (
    input: WorkspaceSearchInput,
    options: WorkspaceSearchRequestOptions = {},
  ): Promise<WorkspaceSearchPage> => {
    const providers = input.providers ?? []
    if (providers.length > 1) {
      // See the module TODO: the generated URL helper cannot express
      // repeated `provider=` params. Fail closed instead of sending a
      // comma-joined filter the backend 422s.
      throw new Error(
        `${i18n.global.t('search.loadFailed')}: multi-provider search needs repeated provider params`,
      )
    }
    const query: { q: string; provider?: ('unsplash' | 'pexels')[]; cursor?: string; page_size?: number } = {
      q: normalizeSearchQuery(input.query),
    }
    if (providers.length === 1) query.provider = [providers[0] as 'unsplash' | 'pexels']
    if (input.cursor) query.cursor = input.cursor
    if (input.pageSize != null) query.page_size = input.pageSize
    const page = await searchWorkspaceTransport(
      assertWorkspaceId(input.workspaceId),
      query,
      toGeneratedOptions(options),
    )
    if (!Array.isArray(page?.data)) throw new Error(i18n.global.t('search.loadFailed'))
    const meta = page.meta ?? { next_cursor: null, has_more: false }
    const nextCursor = typeof meta.next_cursor === 'string' ? meta.next_cursor : ''
    return {
      items: page.data.map(toWorkspaceSearchResult),
      nextCursor,
      hasMore: meta.has_more === true,
    }
  },

  createVisionQuery: async (
    input: WorkspaceVisionQueryInput,
    options: WorkspaceSearchRequestOptions = {},
  ): Promise<WorkspaceVisionQuery> => {
    const data: {
      user_text?: string
      library_asset_ids?: string[]
      upload_ids?: string[]
      image_urls?: string[]
    } = {}
    if (input.userText != null) data.user_text = input.userText
    if ((input.libraryAssetIds ?? []).length > 0) {
      data.library_asset_ids = (input.libraryAssetIds ?? []).map(assertWorkspaceLibraryAssetId)
    }
    if ((input.uploadIds ?? []).length > 0) {
      data.upload_ids = (input.uploadIds ?? []).map(assertWorkspaceUploadId)
    }
    if ((input.imageUrls ?? []).length > 0) {
      data.image_urls = (input.imageUrls ?? []).filter((url) => url.trim()).map((url) => url.trim())
    }
    const envelope = await createWorkspaceVisionQueryTransport(
      assertWorkspaceId(input.workspaceId),
      { data },
      toGeneratedOptions(options),
    )
    return toWorkspaceVisionQuery(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('search.visionSearchFailed'),
      ),
    )
  },
}
