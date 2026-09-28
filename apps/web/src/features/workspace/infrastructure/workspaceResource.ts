// Workspace lifecycle resource (issue #801, frontend slice).
//
// Sole feature-owned boundary over the Public generated client for the
// Workspace / Organization / Principal lifecycle. Route pages and components
// must consume this module, never `generated-query` directly. Only
// `generated-query/public` is wrapped here: never admin/control, never
// direct fetch (transport stays inside the generated `orvalMutator`).
//
// The module codes against the AGREED contract (backend sibling implements
// the real `/api/v1/*` endpoints in parallel):
// - `GET /api/v1/user` -> `{ data: { public_id, display_name, avatar? } }`
// - `GET /api/v1/workspaces?cursor=&page_size=` -> `{ data: [...], meta }`
// - `POST /api/v1/workspaces` with a required `Idempotency-Key` header
// - Errors as `{ error: { code, message, details, request_id } }`
//
// DRIFT (2026-09-23): the checked-in public stubs still expose kernel-only
// shapes (`id` instead of `public_id`, no `name`/`archived_at`/`display_name`,
// transport paths without the `/api/v1` prefix). Parsers below accept both
// the agreed shape and the stub shape so `npm run api:generate` stays green
// before the backend slice lands; stub-only defaults are marked inline.
// When the backend regenerates, stub tolerance narrows without changing the
// domain types exported here.

import {
  createWorkspace,
  getListWorkspacesQueryKey,
  listWorkspaces,
} from '../../../shared/api/generated-query/public/workspaces/workspaces'
import { getGetUserQueryKey, getUser } from '../../../shared/api/generated-query/public/user/user'
import type { ListWorkspacesParams } from '../../../shared/api/generated-query/public/model'
import {
  buildContractRequestHeaders,
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
  parseContractEnvelope,
} from '../../../shared/api/contractKernel'
import { workspaceCreationIdempotencyKey } from '../../../shared/api/idempotencyKey'
import { normalizePageSize, PAGE_SIZES } from '../../../shared/config/pagination'
import type { AuthFailureMode } from '../../../shared/api/client'
import { resolveApiUrl } from '../../../shared/config/runtimeUrls'

export type WorkspaceSummary = {
  publicId: string
  name: string
  organizationId: string | null
  archivedAt: string | null
  createdAt?: string | undefined
}

export type PrincipalSummary = {
  publicId: string
  displayName: string
  avatar?: string | undefined
}

export type WorkspaceListPage = {
  items: WorkspaceSummary[]
  nextCursor: string | null
  hasMore: boolean
}

// Hidden (unknown to the caller, including never-shared) surfaces as 404;
// visible-but-forbidden surfaces as 403. Branch on `kind`, not on copy.
export type WorkspaceAccessKind = 'hidden' | 'forbidden'

export const WORKSPACE_PAGE_SIZE_DEFAULT = PAGE_SIZES.standard
export const WORKSPACE_PAGE_SIZE_MAX = PAGE_SIZES.max
export const WORKSPACE_NAME_MAX_LENGTH = 120

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function asTrimmedString(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed ? trimmed : null
}

function requireOpaqueId(value: unknown, prefix: 'usr' | 'org' | 'ws', what: string): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== prefix) {
    throw new Error(`Invalid ${what} opaque ID`)
  }
  return value
}

function optionalOpaqueId(
  value: unknown,
  prefix: 'usr' | 'org' | 'ws',
  what: string,
): string | null {
  if (value == null) return null
  return requireOpaqueId(value, prefix, what)
}

export function parseWorkspaceItem(raw: unknown): WorkspaceSummary {
  if (!isRecord(raw)) throw new Error('Invalid workspace payload')
  // Agreed shape carries `public_id`; the kernel stub only has `id`.
  const publicId = requireOpaqueId(raw.public_id ?? raw.id, 'ws', 'workspace')
  // Stub workspaces carry no `name` yet; default keeps list rendering stable
  // until the backend slice lands (agreed shape always has `name`).
  const name = typeof raw.name === 'string' ? raw.name : ''
  const organizationId = optionalOpaqueId(raw.organization_id, 'org', 'organization')
  const archivedAt = typeof raw.archived_at === 'string' ? raw.archived_at : null
  const createdAt = typeof raw.created_at === 'string' ? raw.created_at : undefined
  return {
    publicId,
    name,
    organizationId,
    archivedAt,
    ...(createdAt === undefined ? {} : { createdAt }),
  }
}

export function parseWorkspacePage(payload: unknown): WorkspaceListPage {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || 'Invalid workspace list response') as Error & {
      code?: string | undefined
      requestId?: string | undefined
    }
    error.code = parsed.error.code
    error.requestId = parsed.error.request_id
    throw error
  }
  if (!Array.isArray(parsed.data)) throw new Error('Invalid workspace list response')
  const items = parsed.data.map(parseWorkspaceItem)
  const meta = isRecord(parsed.meta) ? parsed.meta : null
  const nextCursor = typeof meta?.next_cursor === 'string' ? meta.next_cursor : null
  const hasMore = meta?.has_more === true
  return { items, nextCursor, hasMore }
}

export function parsePrincipal(payload: unknown): PrincipalSummary {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || 'Invalid principal response') as Error & {
      code?: string | undefined
      requestId?: string | undefined
    }
    error.code = parsed.error.code
    error.requestId = parsed.error.request_id
    throw error
  }
  if (!isRecord(parsed.data)) throw new Error('Invalid principal response')
  // Agreed shape carries `public_id`/`display_name`; the kernel stub `User`
  // only has `id` so far.
  const publicId = requireOpaqueId(parsed.data.public_id ?? parsed.data.id, 'usr', 'principal')
  const displayName = typeof parsed.data.display_name === 'string' ? parsed.data.display_name : ''
  const avatar = asTrimmedString(parsed.data.avatar) ?? undefined
  return {
    publicId,
    displayName,
    ...(avatar === undefined ? {} : { avatar }),
  }
}

export function normalizeWorkspaceListParams(input?: {
  cursor?: string | null | undefined
  pageSize?: number | undefined
}): ListWorkspacesParams {
  const params: ListWorkspacesParams = {}
  const cursor = typeof input?.cursor === 'string' ? input.cursor.trim() : ''
  if (cursor) params.cursor = cursor
  const requested = input?.pageSize
  const pageSize = normalizePageSize(
    requested,
    WORKSPACE_PAGE_SIZE_DEFAULT,
    WORKSPACE_PAGE_SIZE_MAX,
  )
  params.page_size = pageSize
  return params
}

export function buildWorkspaceCreateHeaders(options?: {
  idempotencyKey?: string | undefined
  requestId?: string | undefined
}): Record<string, string> {
  const idempotencyKey =
    asTrimmedString(options?.idempotencyKey) ?? workspaceCreationIdempotencyKey()
  return buildContractRequestHeaders({
    requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
    idempotencyKey,
  })
}

function readErrorStatus(error: unknown): number | null {
  if (!isRecord(error)) return null
  const status = error.status
  return typeof status === 'number' && Number.isFinite(status) ? status : null
}

function readErrorCode(error: unknown): string | null {
  if (!isRecord(error)) return null
  const code = (error as { code?: unknown }).code
  if (typeof code === 'string' && code) return code.toLowerCase()
  const data = isRecord(error.data) ? (error.data as Record<string, unknown>) : null
  const nested = data?.code ?? data?.error_code
  return typeof nested === 'string' && nested ? nested.toLowerCase() : null
}

// 404 (hidden, including never-shared) vs 403 (visible-but-forbidden) stay
// distinct so callers can render "not found" vs "no access" honestly.
export function classifyWorkspaceAccessError(error: unknown): WorkspaceAccessKind | null {
  const status = readErrorStatus(error)
  if (status === 404) return 'hidden'
  if (status === 403) return 'forbidden'
  const code = readErrorCode(error)
  if (!code) return null
  if (code.includes('not_found') || code === 'workspace_hidden') return 'hidden'
  if (code.includes('forbidden') || code.includes('denied')) return 'forbidden'
  return null
}

export type WorkspaceRequestOptions = {
  signal?: AbortSignal | null | undefined
  requestId?: string | undefined
  // Per-request 401 policy, forwarded to the generated transport. Reads on
  // public/guest surfaces pass `silent`; writes and authed-surface reads
  // keep the default redirect.
  authFailure?: AuthFailureMode | undefined
}

function toRequestConfig(options: WorkspaceRequestOptions | undefined): {
  signal?: AbortSignal | undefined
  headers: Record<string, string>
  authFailure?: AuthFailureMode | undefined
} {
  return {
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: buildContractRequestHeaders({
      requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
    }),
    ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
  }
}

export async function fetchWorkspacePage(
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceRequestOptions,
): Promise<WorkspaceListPage> {
  const response = await listWorkspaces(
    normalizeWorkspaceListParams(params),
    toRequestConfig(options),
  )
  return parseWorkspacePage(response)
}

// Full cursor walk for selection bootstrapping. Bounded by `maxPages` so a
// runaway `has_more` can never loop forever; later pages never override the
// caller's abort signal.
export async function fetchAllWorkspaces(
  params?: { pageSize?: number | undefined; maxPages?: number | undefined },
  options?: WorkspaceRequestOptions,
): Promise<WorkspaceSummary[]> {
  const pageSize = params?.pageSize
  const maxPages = params?.maxPages ?? 10
  const items: WorkspaceSummary[] = []
  let cursor: string | null | undefined = undefined
  for (let page = 0; page < Math.max(1, maxPages); page += 1) {
    const result = await fetchWorkspacePage(
      { ...(cursor == null ? {} : { cursor }), ...(pageSize === undefined ? {} : { pageSize }) },
      options,
    )
    items.push(...result.items)
    if (!result.hasMore || result.nextCursor == null) break
    cursor = result.nextCursor
  }
  return items
}

export async function fetchPrincipal(options?: WorkspaceRequestOptions): Promise<PrincipalSummary> {
  const response = await getUser(toRequestConfig(options))
  return parsePrincipal(response)
}

export async function provisionWorkspace(
  input: { name: string; organizationId?: string | undefined },
  options?: WorkspaceRequestOptions & { idempotencyKey?: string | undefined },
): Promise<WorkspaceSummary> {
  const name = typeof input.name === 'string' ? input.name.trim() : ''
  if (!name) throw new Error('Workspace name is required')
  if (name.length > WORKSPACE_NAME_MAX_LENGTH) {
    throw new Error('Workspace name is too long')
  }
  const organization_id = optionalOpaqueId(input.organizationId, 'org', 'organization')
  const response = await createWorkspace(
    {
      data: {
        name,
        ...(organization_id == null ? {} : { organization_id }),
      },
    },
    {
      ...(options?.signal == null ? {} : { signal: options.signal }),
      headers: buildWorkspaceCreateHeaders({
        ...(options?.idempotencyKey === undefined
          ? {}
          : { idempotencyKey: options.idempotencyKey }),
        ...(options?.requestId === undefined ? {} : { requestId: options.requestId }),
      }),
    },
  )
  const parsed = parseContractEnvelope<unknown>(response)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || 'Workspace creation failed') as Error & {
      code?: string | undefined
      requestId?: string | undefined
    }
    error.code = parsed.error.code
    error.requestId = parsed.error.request_id
    throw error
  }
  return parseWorkspaceItem(parsed.data)
}

// Canonical agreed-contract URLs, centralized on runtimeUrls. Transport still
// flows through the generated client above (never direct fetch); these URLs
// are the reference for deep-link building and for the endpoints the backend
// slice is adding (detail/archive/organizations are not in the stubs yet).
function withCursorQuery(
  base: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
): string {
  const normalized = normalizeWorkspaceListParams(params)
  const search = new URLSearchParams()
  if (normalized.cursor) search.set('cursor', normalized.cursor)
  if (normalized.page_size !== undefined) search.set('page_size', String(normalized.page_size))
  const query = search.toString()
  return query ? `${base}?${query}` : base
}

export const workspaceApiUrls = {
  user: (): string => resolveApiUrl('/api/v1/user'),
  workspaces: (params?: {
    cursor?: string | null | undefined
    pageSize?: number | undefined
  }): string => withCursorQuery(resolveApiUrl('/api/v1/workspaces'), params),
  workspaceDetail: (workspaceId: string): string =>
    resolveApiUrl(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}`),
  archiveWorkspace: (workspaceId: string): string =>
    resolveApiUrl(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}:archive`),
  organizations: (): string => resolveApiUrl('/api/v1/organizations'),
  organizationDetail: (organizationId: string): string =>
    resolveApiUrl(`/api/v1/organizations/${encodeURIComponent(organizationId)}`),
  organizationWorkspaces: (organizationId: string): string =>
    resolveApiUrl(`/api/v1/organizations/${encodeURIComponent(organizationId)}/workspaces`),
}

// Single source of truth for server state: TanStack Query caches under these
// keys; selection itself lives in the workspace context (URL param + narrow
// persisted store), never as a duplicated copy of server data.
export const workspaceQueryKeys = {
  all: ['workspaces'] as const,
  list: getListWorkspacesQueryKey(),
  principal: getGetUserQueryKey(),
}
