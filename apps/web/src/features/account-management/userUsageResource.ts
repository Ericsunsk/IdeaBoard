import { getUserUsage } from '../../shared/api/generated-query/user-usage/user-usage'
import type { GetUserUsageRange, UserUsage } from '../../shared/api/generated-query/model'
import { requireApiData } from '../../shared/api/apiResponse'

export type AccountUsage = UserUsage
export type UserUsageRange = GetUserUsageRange

export const DEFAULT_USER_USAGE_RANGE: UserUsageRange = '30d'

export const userUsageQueryKeys = {
  all: ['user', 'usage'] as const,
  current: (range: UserUsageRange) => ['user', 'usage', 'current', range] as const,
}

export async function fetchUserUsage(
  range: UserUsageRange,
  signal?: AbortSignal,
): Promise<AccountUsage> {
  return requireApiData(
    await getUserUsage({ range }, signal ? { signal } : undefined),
    '加载用量统计失败',
  )
}
