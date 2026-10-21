import { getWorkspaceUsage } from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { WorkspaceUsage } from '../../../../api/generated-query/public/model'
import type { BillingConsumptionReadScope } from '../billing-account/readScope'
import { i18n } from '../../../../core/i18n'
import { requireAccountWorkspaceId, resolveWorkspaceBillingData } from '../workspaceBilling'

// User usage boundary (v1 public plane).
//
// Reads go through `GET /api/v1/workspaces/{id}/usage?range=`, scoped to
// the Workspace's stable Billing Account and attributed to the real
// operator and the consuming Workspace. There is no owner-wallet
// projection and no fallback.
export const UsageRange = {
  '7d': '7d',
  '30d': '30d',
  '1y': '1y',
} as const

export type UsageRange = (typeof UsageRange)[keyof typeof UsageRange]
export type GetUserUsageRange = UsageRange
export type AccountUsage = UserUsage

export type UserUsageTrendPoint = {
  date: string
  credits: number
}

export type UserUsage = {
  timezone: string
  lifetime: {
    total_credits: number
    peak_daily_credits: number
    average_daily_credits: number
    task_count: number
    active_days: number
  }
  trend: UserUsageTrendPoint[]
}

export const DEFAULT_USAGE_RANGE: UsageRange = '30d'

export const usageQueryKeys = {
  all: ['workspace', 'usage'] as const,
  current: (scope: BillingConsumptionReadScope, range: UsageRange) =>
    ['workspace', 'usage', 'current', scope, range] as const,
}

export async function fetchUsage(
  workspaceId: string,
  range: UsageRange,
  signal?: AbortSignal,
): Promise<AccountUsage> {
  const id = requireAccountWorkspaceId(workspaceId)
  const usage = await resolveWorkspaceBillingData<WorkspaceUsage>(
    getWorkspaceUsage(id, { range }, { signal }),
    i18n.global.t('account.errors.usageLoadFailed'),
  )
  return {
    timezone: usage.timezone,
    lifetime: {
      total_credits: usage.lifetime.total_credits,
      peak_daily_credits: usage.lifetime.peak_daily_credits,
      average_daily_credits: usage.lifetime.average_daily_credits,
      task_count: usage.lifetime.task_count,
      active_days: usage.lifetime.active_days,
    },
    trend: usage.trend.map((point) => ({ date: point.date, credits: point.credits })),
  }
}
