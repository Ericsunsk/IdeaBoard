import { getUserUsage } from '../../shared/api/generated-query/user-usage/user-usage'
import type { UserUsage } from '../../shared/api/generated-query/model'

type ApiEnvelope<T> = { code?: number; msg?: string; data?: T }

export type AccountUsage = UserUsage

export const userUsageQueryKeys = {
  all: ['user', 'usage'] as const,
  lifetime: ['user', 'usage', 'lifetime'] as const,
}

export async function fetchUserUsage(): Promise<AccountUsage> {
  const response = (await getUserUsage()) as ApiEnvelope<AccountUsage>
  if (response.code !== 0 || response.data == null) {
    throw new Error(response.msg || '加载用量统计失败')
  }
  return response.data
}
