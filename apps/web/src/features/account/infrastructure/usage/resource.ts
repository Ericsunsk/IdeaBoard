import { getUserUsage } from '../../../../shared/api/generated-query/user-usage/user-usage'
import type { GetUserUsageRange, UserUsage } from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

export type AccountUsage = UserUsage
export type UsageRange = GetUserUsageRange

export const DEFAULT_USAGE_RANGE: UsageRange = '30d'

export const usageQueryKeys = {
  all: ['user', 'usage'] as const,
  current: (range: UsageRange) => ['user', 'usage', 'current', range] as const,
}

export async function fetchUsage(range: UsageRange, signal?: AbortSignal): Promise<AccountUsage> {
  return resolveApiData(
    getUserUsage({ range }, signal ? { signal } : undefined),
    i18n.global.t('account.errors.usageLoadFailed'),
  )
}
