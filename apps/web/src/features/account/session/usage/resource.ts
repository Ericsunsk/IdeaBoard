import { getWorkspaceUsage } from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { WorkspaceUsage } from '../../../../api/generated-query/public/model'
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
  // Scalar-only key: workspace + account + identity + scope + range. Never
  // embed a scope object so key equality stays referentially stable.
  current: (
    workspaceId: string,
    billingAccountId: string,
    identity: string,
    consumptionScope: string,
    range: UsageRange,
  ) =>
    [
      'workspace',
      'usage',
      'current',
      workspaceId,
      billingAccountId,
      identity,
      consumptionScope,
      range,
    ] as const,
}

// Named handover-scope matcher for usage keys: the single owner of the
// ['workspace','usage','current', workspaceId, billingAccountId, identity,
// ...] shape. Handover invalidation predicates consult this instead of raw
// queryKey[3..5] subscripts at the call site, so key-shape drift surfaces
// here (request-count gate: scalar-only keys registered in
// queryKeyRequestBudget.contract.spec.ts).
export function isUsageQueryForHandover(
  queryKey: readonly unknown[],
  workspaces: ReadonlySet<string>,
  billingAccountId: string | undefined,
  viewer: string,
): boolean {
  if (billingAccountId == null || billingAccountId === '') return false
  if (queryKey.length < 6) return false
  if (queryKey[0] !== 'workspace' || queryKey[1] !== 'usage') return false
  const workspaceId = queryKey[3]
  return (
    typeof workspaceId === 'string' &&
    workspaces.has(workspaceId) &&
    queryKey[4] === billingAccountId &&
    queryKey[5] === viewer
  )
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
