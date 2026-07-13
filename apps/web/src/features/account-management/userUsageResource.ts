import { getUserUsage } from '../../shared/api/generated-query/user-usage/user-usage'
import type { GetUserUsageRange, UserUsage } from '../../shared/api/generated-query/model'

type ApiEnvelope<T> = { code?: number; msg?: string; data?: T }

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
  const response = (await getUserUsage(
    { range },
    signal ? { signal } : undefined,
  )) as ApiEnvelope<AccountUsage>
  if (response.code !== 0 || response.data == null) {
    throw new Error(response.msg || '加载用量统计失败')
  }
  return response.data
}
