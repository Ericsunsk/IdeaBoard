import { listUserBillingRecords } from '../../shared/api/generated-query/user-billing/user-billing'
import type {
  BillingRecord,
  BillingRecordPage,
  ListUserBillingRecordsParams,
} from '../../shared/api/generated-query/model'
import { resolveApiData } from '../../shared/api/apiResponse'
import { i18n } from '../../shared/i18n'

export type UserBillingRecord = BillingRecord
export type UserBillingRecordPage = BillingRecordPage

export const userBillingRecordsQueryKeys = {
  all: ['user', 'billing', 'records'] as const,
}

export async function fetchUserBillingRecords(
  params: ListUserBillingRecordsParams,
  signal?: AbortSignal,
): Promise<UserBillingRecordPage> {
  return resolveApiData(listUserBillingRecords(params, { signal }), i18n.global.t('account.errors.billingLoadFailed'))
}
