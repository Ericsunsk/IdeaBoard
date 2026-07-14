import { listUserBillingRecords } from '../../shared/api/generated-query/user-billing/user-billing'
import type {
  BillingRecord,
  BillingRecordPage,
  ListUserBillingRecordsParams,
} from '../../shared/api/generated-query/model'

type ApiEnvelope<T> = { code?: number; msg?: string; data?: T }

export type UserBillingRecord = BillingRecord
export type UserBillingRecordPage = BillingRecordPage

export const userBillingRecordsQueryKeys = {
  all: ['user', 'billing', 'records'] as const,
}

export async function fetchUserBillingRecords(
  params: ListUserBillingRecordsParams,
  signal?: AbortSignal,
): Promise<UserBillingRecordPage> {
  return requireBillingRecordsData(
    await listUserBillingRecords(params, { signal }),
    '加载账单记录失败',
  )
}

function requireBillingRecordsData<T>(response: ApiEnvelope<T>, fallbackMessage: string): T {
  if (response.code !== 0 || response.data == null) {
    throw new Error(response.msg || fallbackMessage)
  }
  return response.data
}
