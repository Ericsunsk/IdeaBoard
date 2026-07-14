import { listUserBillingRecords } from '../../shared/api/generated-query/user-billing/user-billing'
import type {
  BillingRecord,
  BillingRecordPage,
  ListUserBillingRecordsParams,
} from '../../shared/api/generated-query/model'
import { requireApiData } from '../../shared/api/apiResponse'

export type UserBillingRecord = BillingRecord
export type UserBillingRecordPage = BillingRecordPage

export const userBillingRecordsQueryKeys = {
  all: ['user', 'billing', 'records'] as const,
}

export async function fetchUserBillingRecords(
  params: ListUserBillingRecordsParams,
  signal?: AbortSignal,
): Promise<UserBillingRecordPage> {
  return requireApiData(
    await listUserBillingRecords(params, { signal }),
    '加载账单记录失败',
  )
}
