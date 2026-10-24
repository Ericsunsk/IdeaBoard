import {
  getSelfMemberQuota,
  listMemberQuotas,
  setMemberQuota,
} from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { MemberQuota } from '../../../../api/generated-query/public/model'
import { i18n } from '../../../../core/i18n'
import { isOpaqueId, opaqueIdPrefix } from '../../../../api/contractKernel'
import { requireAccountWorkspaceId, resolveWorkspaceBillingData } from '../workspaceBilling'

// Member cross-workspace consumption quota boundary (issue #946).
//
// A Billing Account (ba_) holds the unified balance; a Workspace records
// the consumption location and the User records the real operator. The
// quota caps one member's settled + pending consumption across every
// Workspace sharing the account (paid Task generations + Vision Search)
// per window: daily / weekly / monthly (UTC day, Monday-start week,
// calendar month), fixed (lifetime cumulative cap), or unlimited (no cap,
// the default). There are no sub-wallets and no reservations: adjusting
// the limit never clears usage, unused periodic quota never rolls over,
// and joining or rejoining never re-issues. Only Billing Owner/Admin
// writes (workspace admins cannot raise a pool-granted quota); managers
// list every row while members read only their own row.
export type MemberQuotaPeriod = 'daily' | 'weekly' | 'monthly' | 'fixed' | 'unlimited'

export type MemberQuotaSummary = {
  billingAccountId: string
  userId: string
  period: MemberQuotaPeriod
  limitCredits: number | null
  usedCredits: number
  remainingCredits: number | null
  periodStart: string | null
  periodEnd: string | null
}

/** Credits per-command ceiling mirrored from the backend quota bound. */
export const MAX_MEMBER_QUOTA_LIMIT = 9000000000000000

export const memberQuotaQueryKeys = {
  all: ['workspace', 'member-quota'] as const,
  self: (workspaceId: string) => ['workspace', 'member-quota', 'self', workspaceId] as const,
  list: (workspaceId: string) => ['workspace', 'member-quota', 'list', workspaceId] as const,
}

export function parseQuotaPeriodValue(value: unknown): MemberQuotaPeriod | null {
  if (
    value === 'daily' ||
    value === 'weekly' ||
    value === 'monthly' ||
    value === 'fixed' ||
    value === 'unlimited'
  ) {
    return value
  }
  return null
}

export function validateQuotaLimit(raw: string): number | null {
  const trimmed = raw.trim()
  if (!/^\d+$/.test(trimmed)) return null
  const value = Number(trimmed)
  if (!Number.isSafeInteger(value) || value <= 0 || value > MAX_MEMBER_QUOTA_LIMIT) {
    return null
  }
  return value
}

export function quotaPeriodLabelKey(period: MemberQuotaPeriod): string {
  switch (period) {
    case 'daily':
      return 'workspace.quota.periodDaily'
    case 'weekly':
      return 'workspace.quota.periodWeekly'
    case 'monthly':
      return 'workspace.quota.periodMonthly'
    case 'fixed':
      return 'workspace.quota.periodFixed'
    default:
      return 'workspace.quota.periodUnlimited'
  }
}

function requireQuotaPeriod(value: unknown): MemberQuotaPeriod {
  const period = parseQuotaPeriodValue(value)
  if (period != null) return period
  throw new Error('Invalid member quota period')
}

function optionalPositiveInt(value: unknown): number | null {
  if (value == null) return null
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) {
    throw new Error('Invalid member quota credits')
  }
  return value
}

function optionalDateTime(value: unknown): string | null {
  if (value == null) return null
  if (typeof value !== 'string' || !value) throw new Error('Invalid member quota window bound')
  return value
}

function requireQuotaUserId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'usr') {
    throw new Error('Invalid member quota user')
  }
  return value
}

export function parseMemberQuotaData(input: unknown): MemberQuotaSummary {
  const data = input as MemberQuota
  if (
    typeof data.billing_account_id !== 'string' ||
    !isOpaqueId(data.billing_account_id) ||
    opaqueIdPrefix(data.billing_account_id) !== 'ba'
  ) {
    throw new Error('Invalid member quota account')
  }
  const userId = requireQuotaUserId(data.user_id)
  const period = requireQuotaPeriod(data.period)
  const limitCredits = optionalPositiveInt(data.limit_credits ?? null)
  if (period !== 'unlimited' && limitCredits == null) {
    throw new Error('Capped member quota requires a limit')
  }
  if (period === 'unlimited' && limitCredits != null) {
    throw new Error('Unlimited member quota carries no limit')
  }
  const usedCredits = optionalPositiveInt(data.used_credits)
  if (usedCredits == null) throw new Error('Invalid member quota usage')
  const remainingCredits = optionalPositiveInt(data.remaining_credits ?? null)
  return {
    billingAccountId: data.billing_account_id,
    userId,
    period,
    limitCredits,
    usedCredits,
    remainingCredits,
    periodStart: optionalDateTime(data.period_start ?? null),
    periodEnd: optionalDateTime(data.period_end ?? null),
  }
}

/** True when a capped quota is set and usage reached it. */
export function isQuotaExhausted(quota: MemberQuotaSummary): boolean {
  return (
    quota.period !== 'unlimited' &&
    quota.limitCredits != null &&
    quota.usedCredits >= quota.limitCredits
  )
}

export function parseMemberQuotaList(data: unknown): MemberQuotaSummary[] {
  if (!Array.isArray(data)) throw new Error('Invalid member quota list response')
  return data.map((item) => parseMemberQuotaData(item))
}

export async function fetchSelfMemberQuota(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<MemberQuotaSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('workspace.quota.loadFailed')
  const raw = await resolveWorkspaceBillingData<unknown>(
    getSelfMemberQuota(id, { signal }),
    fallback,
  )
  return parseMemberQuotaData(raw)
}

export async function fetchMemberQuotaList(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<MemberQuotaSummary[]> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('workspace.quota.loadFailed')
  const raw = await resolveWorkspaceBillingData<unknown>(listMemberQuotas(id, undefined, { signal }), fallback)
  return parseMemberQuotaList(raw)
}

export type UpdateMemberQuotaInput =
  | { period: 'daily' | 'weekly' | 'monthly' | 'fixed'; limitCredits: number }
  | { period: 'unlimited' }

export function normalizeQuotaInput(input: UpdateMemberQuotaInput): {
  period: MemberQuotaPeriod
  limit_credits?: number
} {
  if (input.period === 'unlimited') return { period: 'unlimited' }
  if (
    !Number.isInteger(input.limitCredits) ||
    input.limitCredits <= 0 ||
    input.limitCredits > MAX_MEMBER_QUOTA_LIMIT
  ) {
    throw new Error(i18n.global.t('workspace.quota.invalidLimit', { max: MAX_MEMBER_QUOTA_LIMIT }))
  }
  return { period: input.period, limit_credits: input.limitCredits }
}

export async function updateMemberQuota(
  workspaceId: string,
  userId: string,
  input: UpdateMemberQuotaInput,
  signal?: AbortSignal,
): Promise<MemberQuotaSummary> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('workspace.quota.saveFailed')
  const body = normalizeQuotaInput(input)
  const raw = await resolveWorkspaceBillingData<unknown>(
    setMemberQuota(id, userId, { data: body }, { signal }),
    fallback,
  )
  return parseMemberQuotaData(raw)
}
