import {
  acceptWorkspaceBillingTransfer,
  cancelWorkspaceBillingTransfer,
  initiateWorkspaceBillingTransfer,
  listWorkspaceBillingTransfers,
  listWorkspaceBillingAccountMembers,
  getListWorkspaceBillingAccountMembersQueryKey,
  rejectWorkspaceBillingTransfer,
} from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import {
  buildContractRequestHeaders,
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
} from '../../../../api/contractKernel'
import { workspaceBillingTransferIdempotencyKey } from '../../../../api/idempotencyKey'
import { withSignal } from '../../../../core/utils/request'
import { asTrimmedString, isRecord } from '../../../../core/utils/guards'
import { i18n } from '../../../../core/i18n'
import { requireAccountWorkspaceId, resolveWorkspaceBillingData } from '../workspaceBilling'
import type { AuthFailureMode } from '../../../../api/client'

// Studio handover invitations (#948, invite-style ownership transfer).
//
// Only a Billing Owner may initiate; the recipient accepts or rejects, and
// the initiator may cancel while pending. Acceptance hands management to
// the recipient (new Owner, previous Owner steps down to Admin) and grants
// the recipient Workspace Owner on every non-Default business Workspace
// associated with the account; the Default Workspace never transfers.
// Identity, balance, Ledger, and history stay stable; no funds move to the
// recipient's other accounts, quotas never reset, and no welcome grant is
// minted.

type BillingTransferStatus = 'pending' | 'accepted' | 'rejected' | 'cancelled'

export type BillingTransferInviteSummary = {
  publicId: string
  billingAccountId: string
  fromUserId: string
  toUserId: string
  status: BillingTransferStatus
  scopeWorkspaceIds: string[]
  createdAt?: string | undefined
  expiresAt?: string | undefined
  decidedAt?: string | undefined
  decidedBy?: string | undefined
}

export function isTransferInviteLapsed(
  invite: BillingTransferInviteSummary,
  now = Date.now(),
): boolean {
  if (invite.status !== 'pending' || invite.expiresAt == null) return false
  const deadline = Date.parse(invite.expiresAt)
  return Number.isNaN(deadline) ? false : now >= deadline
}

export function isTransferInviteLive(
  invite: BillingTransferInviteSummary,
  now = Date.now(),
): boolean {
  return invite.status === 'pending' && !isTransferInviteLapsed(invite, now)
}

export const billingTransferQueryKeys = {
  all: ['workspace', 'billing-transfer'] as const,
  // Reuse the generated endpoint key with per-viewer isolation.
  members: (workspaceId: string, identity: string) =>
    [...getListWorkspaceBillingAccountMembersQueryKey(workspaceId), identity] as const,
  list: (workspaceId: string) => ['workspace', 'billing-transfer', 'list', workspaceId] as const,
}

function requireBillingAccountId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'ba') {
    throw new Error('Invalid billing account opaque ID')
  }
  return value
}

function requireTransferInviteId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'bti') {
    throw new Error('Invalid transfer invite opaque ID')
  }
  return value
}

function requireUserId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'usr') {
    throw new Error('Invalid user opaque ID')
  }
  return value
}

function requireWorkspaceId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'ws') {
    throw new Error('Invalid workspace opaque ID')
  }
  return value
}

function requireTransferStatus(value: unknown): BillingTransferStatus {
  if (
    value === 'pending' ||
    value === 'accepted' ||
    value === 'rejected' ||
    value === 'cancelled'
  ) {
    return value
  }
  throw new Error('Invalid transfer invite status')
}

function parseScopeWorkspaceIds(raw: unknown): string[] {
  if (!Array.isArray(raw)) return []
  return raw.map((id) => requireWorkspaceId(id))
}

function parseDecidedBy(raw: unknown): string | undefined {
  const decidedRaw = asTrimmedString(raw)
  return decidedRaw == null ? undefined : requireUserId(decidedRaw)
}

function parseTransferInvite(raw: unknown): BillingTransferInviteSummary {
  if (!isRecord(raw)) throw new Error('Invalid transfer invite payload')
  const publicId = requireTransferInviteId(raw.public_id ?? raw.publicId)
  const billingAccountId = requireBillingAccountId(raw.billing_account_id ?? raw.billingAccountId)
  const fromUserId = requireUserId(raw.from_user_id ?? raw.fromUserId)
  const toUserId = requireUserId(raw.to_user_id ?? raw.toUserId)
  const status = requireTransferStatus(raw.status)
  const scopeWorkspaceIds = parseScopeWorkspaceIds(raw.scope_workspace_ids ?? raw.scopeWorkspaceIds)
  const createdAt = asTrimmedString(raw.created_at ?? raw.createdAt)
  const expiresAt = asTrimmedString(raw.expires_at ?? raw.expiresAt)
  const decidedAt = asTrimmedString(raw.decided_at ?? raw.decidedAt)
  const decidedBy = parseDecidedBy(raw.decided_by ?? raw.decidedBy)
  return {
    publicId,
    billingAccountId,
    fromUserId,
    toUserId,
    status,
    scopeWorkspaceIds,
    ...(createdAt === undefined ? {} : { createdAt }),
    ...(expiresAt === undefined ? {} : { expiresAt }),
    ...(decidedAt === undefined ? {} : { decidedAt }),
    ...(decidedBy === undefined ? {} : { decidedBy }),
  }
}

export type BillingTransferRequestOptions = {
  signal?: AbortSignal | null | undefined
  requestId?: string | undefined
  idempotencyKey?: string | undefined
  authFailure?: AuthFailureMode | undefined
}

function mutationHeaders(
  options: BillingTransferRequestOptions | undefined,
  operation: 'invite' | 'accept' | 'reject' | 'cancel',
): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: asTrimmedString(options?.requestId) ?? newRequestId(),
    idempotencyKey:
      asTrimmedString(options?.idempotencyKey) ?? workspaceBillingTransferIdempotencyKey(operation),
  })
}

function readConfig(options: BillingTransferRequestOptions | undefined): {
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

export async function fetchTransferInvites(
  workspaceId: string,
  options?: BillingTransferRequestOptions,
): Promise<BillingTransferInviteSummary[]> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('account.errors.billingAccountLoadFailed')
  const data = await resolveWorkspaceBillingData<unknown>(
    listWorkspaceBillingTransfers(id, readConfig(options)),
    fallback,
  )
  if (!Array.isArray(data)) throw new Error(fallback)
  return data.map(parseTransferInvite)
}

export async function initiateTransferInvite(
  workspaceId: string,
  newOwnerUserId: string,
  options?: BillingTransferRequestOptions,
): Promise<BillingTransferInviteSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const target = requireUserId(newOwnerUserId)
  const fallback = i18n.global.t('workspace.switcher.handover.initiateFailed')
  const data = await resolveWorkspaceBillingData<unknown>(
    initiateWorkspaceBillingTransfer(
      id,
      { data: { new_owner_user_id: target } },
      {
        ...withSignal(options?.signal),
        headers: mutationHeaders(options, 'invite'),
        ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
      },
    ),
    fallback,
  )
  return parseTransferInvite(data)
}

export async function acceptTransferInvite(
  workspaceId: string,
  inviteId: string,
  options?: BillingTransferRequestOptions,
): Promise<BillingTransferInviteSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const invite = requireTransferInviteId(inviteId)
  const fallback = i18n.global.t('workspace.switcher.handover.acceptFailed')
  const data = await resolveWorkspaceBillingData<unknown>(
    acceptWorkspaceBillingTransfer(id, invite, {
      ...withSignal(options?.signal),
      headers: mutationHeaders(options, 'accept'),
      ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
    }),
    fallback,
  )
  return parseTransferInvite(data)
}

export async function rejectTransferInvite(
  workspaceId: string,
  inviteId: string,
  options?: BillingTransferRequestOptions,
): Promise<BillingTransferInviteSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const invite = requireTransferInviteId(inviteId)
  const fallback = i18n.global.t('workspace.switcher.handover.rejectFailed')
  const data = await resolveWorkspaceBillingData<unknown>(
    rejectWorkspaceBillingTransfer(id, invite, {
      ...withSignal(options?.signal),
      headers: mutationHeaders(options, 'reject'),
      ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
    }),
    fallback,
  )
  return parseTransferInvite(data)
}

export async function cancelTransferInvite(
  workspaceId: string,
  inviteId: string,
  options?: BillingTransferRequestOptions,
): Promise<BillingTransferInviteSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const invite = requireTransferInviteId(inviteId)
  const fallback = i18n.global.t('workspace.switcher.handover.cancelFailed')
  const data = await resolveWorkspaceBillingData<unknown>(
    cancelWorkspaceBillingTransfer(id, invite, {
      ...withSignal(options?.signal),
      headers: mutationHeaders(options, 'cancel'),
      ...(options?.authFailure == null ? {} : { authFailure: options.authFailure }),
    }),
    fallback,
  )
  return parseTransferInvite(data)
}

export type BillingManagerSummary = {
  userId: string
  role: 'owner' | 'admin'
}

export async function fetchBillingManagers(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<BillingManagerSummary[]> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('account.errors.billingAccountLoadFailed')
  const data = await resolveWorkspaceBillingData<unknown>(
    listWorkspaceBillingAccountMembers(id, { signal }),
    fallback,
  )
  if (!Array.isArray(data)) throw new Error(fallback)
  return data.map((entry): BillingManagerSummary => {
    if (!isRecord(entry) || (entry.role !== 'owner' && entry.role !== 'admin')) {
      throw new Error(fallback)
    }
    return { userId: requireUserId(entry.user_id), role: entry.role }
  })
}
