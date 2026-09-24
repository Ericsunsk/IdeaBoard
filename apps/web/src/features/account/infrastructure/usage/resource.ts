import { api } from '../../../../shared/api/client'
import { resolveApiData, type ApiResponseWithData } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

// User usage boundary (issue #809). Transports through the central
// `api.request` against the surviving `GET /api/user/usage` route with local
// view types.
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
  all: ['user', 'usage'] as const,
  current: (range: UsageRange) => ['user', 'usage', 'current', range] as const,
}

export async function fetchUsage(range: UsageRange, signal?: AbortSignal): Promise<AccountUsage> {
  const search = new URLSearchParams({ range })
  return resolveApiData(
    api.request<ApiResponseWithData<AccountUsage>>(`/user/usage?${search.toString()}`, {
      method: 'GET',
      ...(signal == null ? {} : { signal }),
    }),
    i18n.global.t('account.errors.usageLoadFailed'),
  )
}
