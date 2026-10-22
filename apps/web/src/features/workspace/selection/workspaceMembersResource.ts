import type { QueryClient, QueryKey, UseQueryOptions } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  addWorkspaceMember as addWorkspaceMemberRequest,
  getListWorkspaceMembersQueryKey,
  listWorkspaceMembers as listWorkspaceMembersRequest,
  lookupWorkspaceMemberByEmail as lookupWorkspaceMemberRequest,
  removeWorkspaceMember as removeWorkspaceMemberRequest,
  updateWorkspaceMember as updateWorkspaceMemberRequest,
} from '../../../api/generated-query/public/workspace-members/workspace-members'
import type { MembershipRole } from '../../../api/generated-query/public/model/membershipRole'
import { workspaceMemberIdempotencyKey } from '../../../api/idempotencyKey'
import { userErrorCode } from '../../../core/errors/errorCatalog'
import { accountScoped, noRetry } from '../../../core/query/queryDefaults'
import { withSignal } from '../../../core/utils/request'

// Workspace member transport and roster Query ownership.
//
// Sole feature-owned boundary over the generated `public/workspace-members`
// client. Components and composables consume this module, never
// `generated-query` directly. Every call is workspace-scoped with an explicit
// `ws_` id; there is no ambient workspace inference here (the dialog manages
// several workspaces at once, so fail-closed resolution from a selection
// store would pick the wrong scope).
//
// Wire semantics (server-enforced, mirrored here for fail-fast validation):
// - list: members-only visibility; entries carry `user_id` (usr_), `role`
//   (owner/admin/member), optional display-only `email` and `avatar`.
// - lookup: owner/admin only, exact email match, returns only usr_;
//   unknown emails converge to 404 `workspace_member_not_found`.
// - invite (POST): owner/admin only, requires `Idempotency-Key` per attempt;
//   re-adding keeps the current role; admin inviting owner renders 403.
//   Personal Default is private: invites there render 403
//   `workspace_default_protected`. Other Personal workspaces invite freely
//   with no per-invite holder approval: consumption charges the holder's
//   shared Personal account with the real operator on each record, and an
//   unconfigured collaborator quota reads as unlimited (the holder sets it
//   later without clearing usage).
// - role change (PATCH): no `Idempotency-Key`; promoting to owner requires
//   owner; the last owner cannot be demoted (`membership_transition_forbidden`).
// - removal (DELETE): requires `Idempotency-Key` per attempt; self-leave is
//   allowed but the last owner is protected; board grants are swept server-side.
//   Removal never deletes quota state or historical consumption: rejoining
//   restores the same (Billing Account, User) row and usage.

/** Workspace membership role on the wire. */
export type WorkspaceMemberRole = 'owner' | 'admin' | 'member'

/** One roster entry: User public identity, role and display-only profile. */
export type WorkspaceMemberEntry = {
  userId: string
  role: WorkspaceMemberRole
  /**
   * Workspace-visible member email for display assistance only. Omitted
   * when the server has none; never used as a write identity (writes stay
   * usr_ with unknown users converging to 404).
   */
  email?: string | undefined
  avatar_url?: string | undefined
}

export type WorkspaceMembersRequestOptions = {
  signal?: AbortSignal | null | undefined
  idempotencyKey?: string | undefined
}

/** Stable failure reasons for member reads/writes (never prose). */
export type WorkspaceMemberFailureReason =
  | 'unknown-user'
  | 'hidden'
  | 'forbidden'
  | 'default-protected'
  | 'last-owner'
  | 'member-limit'
  | 'idempotency-conflict'
  | 'invalid'
  | 'sync-failed'

const WORKSPACE_MEMBER_ROLES: readonly WorkspaceMemberRole[] = ['owner', 'admin', 'member']

const USER_PUBLIC_ID_PATTERN = /^usr_[A-Za-z0-9_-]+$/

/** The server answered with a member envelope this client cannot use. */
export class WorkspaceMemberPayloadError extends Error {
  constructor(field: string) {
    super(`Invalid workspace member payload: ${field}`)
    this.name = 'WorkspaceMemberPayloadError'
  }
}

function requireWorkspaceId(workspaceId: string): string {
  const normalized = workspaceId.trim()
  if (!/^ws_[A-Za-z0-9_-]+$/.test(normalized)) throw new WorkspaceMemberPayloadError('workspace_id')
  return normalized
}

function toMemberUserId(value: unknown, field: string): string {
  if (typeof value === 'string' && USER_PUBLIC_ID_PATTERN.test(value.trim())) {
    return value.trim()
  }
  throw new WorkspaceMemberPayloadError(field)
}

function toMemberRole(value: unknown, field: string): WorkspaceMemberRole {
  if (typeof value === 'string' && (WORKSPACE_MEMBER_ROLES as readonly string[]).includes(value)) {
    return value as WorkspaceMemberRole
  }
  throw new WorkspaceMemberPayloadError(field)
}

function toMemberEntry(data: unknown): WorkspaceMemberEntry {
  if (!data || typeof data !== 'object') throw new WorkspaceMemberPayloadError('member')
  const record = data as Record<string, unknown>
  const entry: WorkspaceMemberEntry = {
    userId: toMemberUserId(record.user_id, 'user_id'),
    role: toMemberRole(record.role, 'role'),
  }
  // Email is display assistance only: missing or blank stays omitted
  // (fail-closed), never rejects the roster.
  const rawEmail = typeof record.email === 'string' ? record.email.trim() : ''
  if (rawEmail) entry.email = rawEmail
  const rawAvatar = typeof record.avatar === 'string' ? record.avatar.trim() : ''
  if (rawAvatar) entry.avatar_url = rawAvatar
  return entry
}

function readConfig(options: WorkspaceMembersRequestOptions | undefined): {
  signal?: AbortSignal | undefined
} {
  return { ...withSignal(options?.signal) }
}

function writeConfig(
  options: WorkspaceMembersRequestOptions | undefined,
  operation: 'invite' | 'remove',
): { headers: Record<string, string>; signal?: AbortSignal | undefined } {
  const idempotencyKey = options?.idempotencyKey?.trim() || workspaceMemberIdempotencyKey(operation)
  if (!idempotencyKey) throw new WorkspaceMemberPayloadError('idempotencyKey')
  return { headers: { 'Idempotency-Key': idempotencyKey }, ...withSignal(options?.signal) }
}

/** Lists the workspace roster (current members only). */
export async function fetchWorkspaceMembers(
  workspaceId: string,
  options: WorkspaceMembersRequestOptions = {},
): Promise<WorkspaceMemberEntry[]> {
  const workspace = requireWorkspaceId(workspaceId)
  const response = await listWorkspaceMembersRequest(workspace, readConfig(options))
  return (response.data ?? []).map(toMemberEntry)
}

/**
 * Resolves one registered user to its usr_ identifier by exact email match.
 * Owner/admin only; unknown emails converge to 404
 * `workspace_member_not_found`.
 */
export async function lookupWorkspaceMemberId(
  workspaceId: string,
  email: string,
  options: WorkspaceMembersRequestOptions = {},
): Promise<string> {
  const workspace = requireWorkspaceId(workspaceId)
  const normalizedEmail = email.trim()
  if (!normalizedEmail) throw new WorkspaceMemberPayloadError('email')
  const envelope = await lookupWorkspaceMemberRequest(
    workspace,
    { email: normalizedEmail },
    readConfig(options),
  )
  return toMemberUserId(envelope.data?.user_id, 'user_id')
}

/** Invites a member with a workspace role (owner/admin only). */
export async function inviteWorkspaceMember(
  workspaceId: string,
  input: { userId: string; role: WorkspaceMemberRole },
  options: WorkspaceMembersRequestOptions = {},
): Promise<WorkspaceMemberEntry> {
  const workspace = requireWorkspaceId(workspaceId)
  const userId = toMemberUserId(input.userId, 'user_id')
  const role: MembershipRole = toMemberRole(input.role, 'role')
  const envelope = await addWorkspaceMemberRequest(
    workspace,
    { data: { user_id: userId, role } },
    writeConfig(options, 'invite'),
  )
  return toMemberEntry(envelope.data)
}

/**
 * Changes a member role (owner-only promotion, last-owner protected).
 * Idempotent by value: no `Idempotency-Key` is sent.
 */
export async function changeWorkspaceMemberRole(
  workspaceId: string,
  memberId: string,
  input: { role: WorkspaceMemberRole },
  options: WorkspaceMembersRequestOptions = {},
): Promise<WorkspaceMemberEntry> {
  const workspace = requireWorkspaceId(workspaceId)
  const member = toMemberUserId(memberId, 'member_id')
  const role: MembershipRole = toMemberRole(input.role, 'role')
  const envelope = await updateWorkspaceMemberRequest(
    workspace,
    member,
    { data: { role } },
    readConfig(options),
  )
  return toMemberEntry(envelope.data)
}

/**
 * Removes a workspace member (self-leave allowed, last owner protected).
 * There is no Undo: the server also sweeps the member's board grants in
 * the same transaction, so callers must confirm before sending.
 */
export async function removeWorkspaceMemberEntry(
  workspaceId: string,
  memberId: string,
  options: WorkspaceMembersRequestOptions = {},
): Promise<void> {
  const workspace = requireWorkspaceId(workspaceId)
  const member = toMemberUserId(memberId, 'member_id')
  await removeWorkspaceMemberRequest(workspace, member, writeConfig(options, 'remove'))
}

/** Maps stable backend codes to member failure reasons (never prose). */
export function classifyWorkspaceMemberError(error: unknown): WorkspaceMemberFailureReason {
  switch (userErrorCode(error)) {
    case 'workspace_member_not_found':
      return 'unknown-user'
    case 'workspace_not_found':
      return 'hidden'
    case 'workspace_forbidden':
      return 'forbidden'
    case 'workspace_default_protected':
      return 'default-protected'
    case 'membership_transition_forbidden':
      return 'last-owner'
    case 'workspace_member_limit_reached':
      return 'member-limit'
    case 'workspace_idempotency_conflict':
      return 'idempotency-conflict'
    case 'bad_request':
    case 'validation_failed':
    case 'idempotency_key_required':
      return 'invalid'
    default:
      return 'sync-failed'
  }
}

/** Fail-fast roster query for one workspace (members-only read). */
export function workspaceMembersQueryOptions(
  workspaceId: MaybeRefOrGetter<string>,
  fetch: typeof fetchWorkspaceMembers = fetchWorkspaceMembers,
) {
  type QueryContext = { signal?: AbortSignal }
  const queryFn = ({ signal }: QueryContext) =>
    fetch(toValue(workspaceId), { signal: signal ?? null })
  // The TanStack satisfies below is also the naming contract: `enabled`
  // stays verbatim because it is the query client's public API, not an
  // owned predicate (mirrors the generated `*_queryOptions` shape).
  return {
    queryKey: getListWorkspaceMembersQueryKey(workspaceId),
    queryFn,
    ...accountScoped(),
    ...noRetry(),
    enabled: computed(() => {
      const current = toValue(workspaceId)
      return typeof current === 'string' && current.trim().length > 0
    }),
  } satisfies UseQueryOptions<WorkspaceMemberEntry[], Error, WorkspaceMemberEntry[], QueryKey>
}

/**
 * Invalidates one workspace roster after a member write. Failures keep the
 * confirmed state: invalidation only refetches, it never writes facts.
 */
export function invalidateWorkspaceMemberQueries(
  client: Pick<QueryClient, 'invalidateQueries'>,
  workspaceId: string,
): void {
  void client.invalidateQueries({
    queryKey: getListWorkspaceMembersQueryKey(workspaceId),
    exact: true,
  })
}
