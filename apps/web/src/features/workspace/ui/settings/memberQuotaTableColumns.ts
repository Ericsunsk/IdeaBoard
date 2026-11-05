// Member quota table view-model (Org-only quota management).
//
// Pure row projection with a single traversal: one pass builds rows and counts
// exhausted quotas. Status resolution funnels through `isQuotaExhausted` as the
// single exhaustion truth. Transport, validation and toasts stay in
// useMemberQuotaSection; this module never touches queries, i18n or toasts.
import {
  isQuotaExhausted,
  type MemberQuotaSummary,
} from '@/features/account/session/quota/resource'
import type { WorkspaceMemberRole } from '@/features/workspace/selection/workspaceMembersResource'

export type MemberQuotaStatus = 'normal' | 'nearExhausted' | 'unlimited' | 'exhausted'

/** Remaining/limit ratio below which a capped quota reads as 将耗尽. */
const NEAR_EXHAUSTED_RATIO = 0.2

export type MemberQuotaRow = {
  userId: string
  label: string
  period: MemberQuotaSummary['period']
  limitCredits: number | null
  usedCredits: number
  remainingCredits: number | null
  role: WorkspaceMemberRole | null
  status: MemberQuotaStatus
}

export type MemberQuotaView = {
  rows: MemberQuotaRow[]
  exhaustedCount: number
}

function resolveMemberQuotaStatus(quota: MemberQuotaSummary): MemberQuotaStatus {
  if (quota.period === 'unlimited') return 'unlimited'
  if (isQuotaExhausted(quota)) return 'exhausted'
  if (
    quota.limitCredits != null &&
    quota.remainingCredits != null &&
    quota.limitCredits > 0 &&
    quota.remainingCredits / quota.limitCredits < NEAR_EXHAUSTED_RATIO
  ) {
    return 'nearExhausted'
  }
  return 'normal'
}

export function buildMemberQuotaView(
  quotas: readonly MemberQuotaSummary[],
  resolveLabel: (userId: string) => string,
  resolveRole: (userId: string) => WorkspaceMemberRole | null,
): MemberQuotaView {
  const rows: MemberQuotaRow[] = []
  let exhaustedCount = 0
  for (const quota of quotas) {
    const status = resolveMemberQuotaStatus(quota)
    if (status === 'exhausted') exhaustedCount += 1
    rows.push({
      userId: quota.userId,
      label: resolveLabel(quota.userId),
      period: quota.period,
      limitCredits: quota.limitCredits,
      usedCredits: quota.usedCredits,
      remainingCredits: quota.remainingCredits,
      role: resolveRole(quota.userId),
      status,
    })
  }
  return { rows, exhaustedCount }
}
