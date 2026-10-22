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
//
// Billing scope (ADR-0038) is the creation scope and nothing else:
// omitted organization_id creates a Personal workspace on the caller's
// Personal account, an organization_id creates an Organization workspace
// on the shared org pool. The scope is fixed at creation; there is no
// billing choice to pass and no payer switch afterwards.

import {
  createWorkspace,
  deleteWorkspace as deleteWorkspaceTransport,
  getListWorkspacesQueryKey,
  listWorkspaces,
  updateWorkspace,
} from '../../../api/generated-query/public/workspaces/workspaces'
import {
  addOrganizationMember as addOrganizationMemberTransport,
  closeOrganization as closeOrganizationTransport,
  createOrganization as createOrganizationTransport,
  listOrganizationMembers as listOrganizationMembersTransport,
  listOrganizations,
  removeOrganizationMember as removeOrganizationMemberTransport,
  restoreOrganization as restoreOrganizationTransport,
  updateOrganizationMember as updateOrganizationMemberTransport,
} from '../../../api/generated-query/public/organizations/organizations'
import { getGetUserQueryKey, getUser } from '../../../api/generated-query/public/user/user'
import type { ListWorkspacesParams } from '../../../api/generated-query/public/model'
import {
  buildContractRequestHeaders,
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
  requireContractData,
} from '../../../api/contractKernel'
import {
  workspaceCreationIdempotencyKey,
  workspaceDeletionIdempotencyKey,
} from '../../../api/idempotencyKey'
import { normalizePageSize, PAGE_SIZES } from '../../../core/config/pagination'
import type { AuthFailureMode } from '../../../api/client'
import { withSignal } from '../../../core/utils/request'
import { asTrimmedString, isRecord } from '../../../core/utils/guards'
import { resolveApiUrl } from '../../../core/config/runtimeUrls'
import { userErrorCode } from '../../../core/errors/errorCatalog'

export type WorkspaceSummary = {
  publicId: string
  name: string
  organizationId: string | null
  // Default workspaces cannot be deleted or renamed.
  isDefault: boolean
  createdAt?: string | undefined
  // Stable funds identity (#944): workspaces sharing one account report the
  // same ba_ value; omitted for legacy payloads without the field.
  billingAccountId?: string | undefined
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

function optionalBillingAccountId(value: unknown): string | undefined {
  if (value == null) return undefined
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'ba')
    throw new Error('Invalid billing account opaque ID')
  return value
}

export function parseWorkspaceItem(raw: unknown): WorkspaceSummary {
  if (!isRecord(raw)) throw new Error('Invalid workspace payload')
  // Agreed shape carries `public_id`; the kernel stub only has `id`.
  const publicId = requireOpaqueId(raw.public_id ?? raw.id, 'ws', 'workspace')
  // Stub workspaces carry no `name` yet; default keeps list rendering stable
  // until the backend slice lands (agreed shape always has `name`).
  const name = typeof raw.name === 'string' ? raw.name : ''
  const organizationId = optionalOpaqueId(raw.organization_id, 'org', 'organization')
  const createdAt = typeof raw.created_at === 'string' ? raw.created_at : undefined
  const billingAccountId = optionalBillingAccountId(raw.billing_account_id ?? raw.billingAccountId)
  return {
    publicId,
    name,
    organizationId,
    isDefault: raw.is_default === true,
    ...(createdAt === undefined ? {} : { createdAt }),
    ...(billingAccountId === undefined ? {} : { billingAccountId }),
  }
}

export function parseWorkspacePage(payload: unknown): WorkspaceListPage {
  const data = requireContractData<unknown>(payload, 'Invalid workspace list response')
  if (!Array.isArray(data)) throw new Error('Invalid workspace list response')
  const items = data.map(parseWorkspaceItem)
  const metaRaw = isRecord(payload) ? payload.meta : null
  const meta = isRecord(metaRaw) ? metaRaw : null
  const nextCursor = typeof meta?.next_cursor === 'string' ? meta.next_cursor : null
  const hasMore = meta?.has_more === true
  return { items, nextCursor, hasMore }
}

export function parsePrincipal(payload: unknown): PrincipalSummary {
  const data = requireContractData<unknown>(payload, 'Invalid principal response')
  if (!isRecord(data)) throw new Error('Invalid principal response')
  // Agreed shape carries `public_id`/`display_name`; the kernel stub `User`
  // only has `id` so far.
  const publicId = requireOpaqueId(data.public_id ?? data.id, 'usr', 'principal')
  const displayName = typeof data.display_name === 'string' ? data.display_name : ''
  const avatar = asTrimmedString(data.avatar)
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

// 404 (hidden, including never-shared) vs 403 (visible-but-forbidden) stay
// distinct so callers can render "not found" vs "no access" honestly.
export function classifyWorkspaceAccessError(error: unknown): WorkspaceAccessKind | null {
  const status = readErrorStatus(error)
  if (status === 404) return 'hidden'
  if (status === 403) return 'forbidden'
  const code = userErrorCode(error)?.toLowerCase()
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
    ...withSignal(options?.signal),
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
  const organizationId = optionalOpaqueId(input.organizationId, 'org', 'organization')
  const response = await createWorkspace(
    {
      data: {
        name,
        ...(organizationId == null ? {} : { organization_id: organizationId }),
      },
    },
    {
      ...withSignal(options?.signal),
      headers: buildWorkspaceCreateHeaders({
        ...(options?.idempotencyKey === undefined
          ? {}
          : { idempotencyKey: options.idempotencyKey }),
        ...(options?.requestId === undefined ? {} : { requestId: options.requestId }),
      }),
    },
  )
  const data = requireContractData<unknown>(response, 'Workspace creation failed')
  return parseWorkspaceItem(data)
}

// Explicit PATCH rename (owner/admin only server-side, never the Default
// row: it renders 403 workspace_default_protected). No Idempotency-Key:
// renames are idempotent by value, so retries replay safely without one.
export async function renameWorkspace(
  workspaceId: string,
  input: { name: string },
  options?: WorkspaceRequestOptions,
): Promise<WorkspaceSummary> {
  const targetId = requireOpaqueId(workspaceId, 'ws', 'workspace')
  const name = typeof input.name === 'string' ? input.name.trim() : ''
  if (!name) throw new Error('Workspace name is required')
  if (name.length > WORKSPACE_NAME_MAX_LENGTH) {
    throw new Error('Workspace name is too long')
  }
  const response = await updateWorkspace(
    targetId,
    {
      data: {
        name,
      },
    },
    toRequestConfig(options),
  )
  const data = requireContractData<unknown>(response, 'Workspace rename failed')
  return parseWorkspaceItem(data)
}

// Explicit DELETE (owner/admin only server-side, never the Default row:
// it renders 403 workspace_default_protected). The backend fails closed
// without an `Idempotency-Key`, so every attempt mints one unless the
// caller reuses a key for a retry of the same attempt. Resolves once the
// server confirms `{ data: { deleted: true } }`; there is no recovery
// window and no restore path.
export async function deleteWorkspace(
  workspaceId: string,
  options?: WorkspaceRequestOptions & { idempotencyKey?: string | undefined },
): Promise<void> {
  const targetId = requireOpaqueId(workspaceId, 'ws', 'workspace')
  const response = await deleteWorkspaceTransport(targetId, {
    ...withSignal(options?.signal),
    headers: buildContractRequestHeaders({
      requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
      idempotencyKey: asTrimmedString(options?.idempotencyKey) ?? workspaceDeletionIdempotencyKey(),
    }),
    ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
  })
  const data = requireContractData<unknown>(response, 'Workspace deletion failed')
  if (!isRecord(data) || data.deleted !== true) throw new Error('Workspace deletion failed')
}

// Canonical agreed-contract URLs, centralized on runtimeUrls. Transport still
// flows through the generated client above (never direct fetch); these URLs
// are the reference for deep-link building. Deletes ride `workspaceDetail`
// (`DELETE` on the workspace path): the old archive sub-path is gone.
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
  organizations: (): string => resolveApiUrl('/api/v1/organizations'),
  organizationDetail: (organizationId: string): string =>
    resolveApiUrl(`/api/v1/organizations/${encodeURIComponent(organizationId)}`),
  organizationWorkspaces: (organizationId: string): string =>
    resolveApiUrl(`/api/v1/organizations/${encodeURIComponent(organizationId)}/workspaces`),
  organizationMembers: (organizationId: string): string =>
    resolveApiUrl(`/api/v1/organizations/${encodeURIComponent(organizationId)}/members`),
  organizationMemberDetail: (organizationId: string, memberId: string): string =>
    resolveApiUrl(
      `/api/v1/organizations/${encodeURIComponent(organizationId)}/members/${encodeURIComponent(memberId)}`,
    ),
}

// Single source of truth for server state: TanStack Query caches under these
// keys; selection itself lives in the workspace context (URL param + narrow
// persisted store), never as a duplicated copy of server data.
export const workspaceQueryKeys = {
  all: ['workspaces'] as const,
  list: getListWorkspacesQueryKey(),
  principal: getGetUserQueryKey(),
  organizations: ['organizations'] as const,
  organizationMembers: (organizationId: string) =>
    ['organizations', organizationId, 'members'] as const,
}

export type OrganizationStatus = 'active' | 'closed'

export type OrganizationSummary = {
  publicId: string
  name: string
  status: OrganizationStatus
  closedAt?: string | undefined
}

function parseOrganizationStatus(value: unknown): OrganizationStatus {
  if (value === 'closed') return 'closed'
  return 'active'
}

function parseOrganization(raw: unknown): OrganizationSummary {
  if (!isRecord(raw) || typeof raw.name !== 'string' || !raw.name.trim()) {
    throw new Error('Invalid organization payload')
  }
  const closedAt = typeof raw.closed_at === 'string' && raw.closed_at ? raw.closed_at : undefined
  return {
    publicId: requireOpaqueId(raw.id, 'org', 'organization'),
    name: raw.name,
    status: parseOrganizationStatus(raw.status),
    ...(closedAt === undefined ? {} : { closedAt }),
  }
}

export async function fetchOrganizations(): Promise<OrganizationSummary[]> {
  const items: OrganizationSummary[] = []
  const visited = new Set<string>()
  let cursor: string | undefined
  while (true) {
    const page = await listOrganizations({
      page_size: WORKSPACE_PAGE_SIZE_MAX,
      ...(cursor ? { cursor } : {}),
    })
    const data = requireContractData<unknown>(page, 'Organization list failed')
    if (!Array.isArray(data)) throw new Error('Invalid organization list')
    items.push(...data.map(parseOrganization))
    if (!page.meta.has_more) return items
    const next = page.meta.next_cursor
    if (!next || visited.has(next)) throw new Error('Invalid organization cursor')
    visited.add(next)
    cursor = next
  }
}

export async function provisionOrganization(
  name: string,
  idempotencyKey: string,
): Promise<OrganizationSummary> {
  const normalized = name.trim()
  if (!normalized || normalized.length > WORKSPACE_NAME_MAX_LENGTH)
    throw new Error('Invalid organization name')
  const key = asTrimmedString(idempotencyKey)
  if (!key) throw new Error('Organization creation requires an idempotency key')
  const response = await createOrganizationTransport(
    { data: { name: normalized } },
    {
      headers: buildContractRequestHeaders({
        requestId: newRequestId(),
        idempotencyKey: key,
      }),
    },
  )
  return parseOrganization(requireContractData<unknown>(response, 'Organization creation failed'))
}

export type OrganizationRole = 'owner' | 'admin' | 'member'

export type OrganizationMemberSummary = {
  userId: string
  role: OrganizationRole
  createdAt?: string | undefined
}

function parseOrganizationRole(value: unknown): OrganizationRole {
  if (value === 'owner' || value === 'admin' || value === 'member') return value
  throw new Error('Invalid organization member role')
}

export function parseOrganizationMember(raw: unknown): OrganizationMemberSummary {
  if (!isRecord(raw)) throw new Error('Invalid organization member payload')
  const userId = requireOpaqueId(raw.user_id ?? raw.userId, 'usr', 'organization member')
  const role = parseOrganizationRole(raw.role)
  const createdAt = typeof raw.created_at === 'string' ? raw.created_at : undefined
  return { userId, role, ...(createdAt === undefined ? {} : { createdAt }) }
}

export function isPersonalWorkspace(item: Pick<WorkspaceSummary, 'organizationId'>): boolean {
  return item.organizationId == null
}

export function isOrganizationWorkspace(
  item: Pick<WorkspaceSummary, 'organizationId'>,
  organizationId?: string | null,
): boolean {
  if (item.organizationId == null) return false
  if (organizationId == null) return true
  return item.organizationId === organizationId
}

// The organization itself is the management entry (#951): it carries no
// boards, content, or consumption. Workspace access follows the role
// mapping — owner/admin automatic on every organization workspace, member
// only with an explicit per-workspace grant. Non-members hold nothing
// until they join the organization.
export function effectiveOrganizationWorkspaceAccess(input: {
  organizationRole: OrganizationRole | null
  hasWorkspaceGrant: boolean
}): boolean {
  if (input.organizationRole === 'owner' || input.organizationRole === 'admin') return true
  if (input.organizationRole === 'member') return input.hasWorkspaceGrant
  return false
}

export async function fetchOrganizationMembers(
  organizationId: string,
  options?: WorkspaceRequestOptions,
): Promise<OrganizationMemberSummary[]> {
  const orgId = requireOpaqueId(organizationId, 'org', 'organization')
  const response = await listOrganizationMembersTransport(orgId, toRequestConfig(options))
  const data = requireContractData<unknown>(response, 'Organization member list failed')
  if (!Array.isArray(data)) throw new Error('Invalid organization member list')
  return data.map(parseOrganizationMember)
}

export async function inviteOrganizationMember(
  organizationId: string,
  input: { userId: string; role: Exclude<OrganizationRole, 'owner'> },
  options?: WorkspaceRequestOptions & { idempotencyKey?: string | undefined },
): Promise<OrganizationMemberSummary> {
  const orgId = requireOpaqueId(organizationId, 'org', 'organization')
  const userId = requireOpaqueId(input.userId, 'usr', 'organization member')
  const role = parseOrganizationRole(input.role)
  if (role === 'owner') throw new Error('Organization owner change requires explicit handover')
  const response = await addOrganizationMemberTransport(
    orgId,
    { data: { user_id: userId, role } },
    {
      ...withSignal(options?.signal),
      headers: buildContractRequestHeaders({
        requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
        idempotencyKey: asTrimmedString(options?.idempotencyKey) ?? newRequestId(),
      }),
      ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
    },
  )
  return parseOrganizationMember(
    requireContractData<unknown>(response, 'Organization member invite failed'),
  )
}

export async function updateOrganizationMember(
  organizationId: string,
  memberId: string,
  input: { role: Exclude<OrganizationRole, 'owner'> },
  options?: WorkspaceRequestOptions,
): Promise<OrganizationMemberSummary> {
  const orgId = requireOpaqueId(organizationId, 'org', 'organization')
  const userId = requireOpaqueId(memberId, 'usr', 'organization member')
  const role = parseOrganizationRole(input.role)
  if (role === 'owner') throw new Error('Organization owner change requires explicit handover')
  const response = await updateOrganizationMemberTransport(
    orgId,
    userId,
    { data: { role } },
    toRequestConfig(options),
  )
  return parseOrganizationMember(
    requireContractData<unknown>(response, 'Organization member update failed'),
  )
}

export async function removeOrganizationMember(
  organizationId: string,
  memberId: string,
  options?: WorkspaceRequestOptions & { idempotencyKey?: string | undefined },
): Promise<void> {
  const orgId = requireOpaqueId(organizationId, 'org', 'organization')
  const userId = requireOpaqueId(memberId, 'usr', 'organization member')
  const response = await removeOrganizationMemberTransport(orgId, userId, {
    ...withSignal(options?.signal),
    headers: buildContractRequestHeaders({
      requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
      idempotencyKey: asTrimmedString(options?.idempotencyKey) ?? newRequestId(),
    }),
    ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
  })
  const data = requireContractData<unknown>(response, 'Organization member removal failed')
  if (!isRecord(data) || data.deleted !== true)
    throw new Error('Organization member removal failed')
}

// Organization closure collapses access and spending at once (#955): only
// the Owner closes; the shared pool refuses while in-flight Tasks, payment
// orders, refunds, disputes, or a pending handover invite settle. The
// remaining balance stays in the closed account (never Personal) with a
// fail-closed retained disposition until recharge terms land.
export async function closeOrganization(
  organizationId: string,
  input: { reason?: string | undefined },
  options?: WorkspaceRequestOptions & { idempotencyKey?: string | undefined },
): Promise<OrganizationSummary> {
  const orgId = requireOpaqueId(organizationId, 'org', 'organization')
  const reason = (input.reason ?? '').trim().slice(0, 280)
  const response = await closeOrganizationTransport(
    orgId,
    { data: { ...(reason ? { reason } : {}) } },
    {
      ...withSignal(options?.signal),
      headers: buildContractRequestHeaders({
        requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
        idempotencyKey: asTrimmedString(options?.idempotencyKey) ?? newRequestId(),
      }),
      ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
    },
  )
  return parseOrganization(requireContractData<unknown>(response, 'Organization closure failed'))
}

// Organization restoration reopens a closed organization (#955): only the
// Owner restores; retained memberships and permissions resume.
export async function restoreOrganization(
  organizationId: string,
  options?: WorkspaceRequestOptions & { idempotencyKey?: string | undefined },
): Promise<OrganizationSummary> {
  const orgId = requireOpaqueId(organizationId, 'org', 'organization')
  const response = await restoreOrganizationTransport(orgId, {
    ...withSignal(options?.signal),
    headers: buildContractRequestHeaders({
      requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
      idempotencyKey: asTrimmedString(options?.idempotencyKey) ?? newRequestId(),
    }),
    ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
  })
  return parseOrganization(
    requireContractData<unknown>(response, 'Organization restoration failed'),
  )
}
