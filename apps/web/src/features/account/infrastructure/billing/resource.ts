import { listUserBillingRecords } from '../../../../shared/api/generated-query/user-billing/user-billing'
import type {
  BillingRecord,
  BillingRecordPage,
  ListUserBillingRecordsParams,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

export type BillingRecordItem = BillingRecord
export type BillingRecordPageData = BillingRecordPage

export const billingRecordsQueryKeys = {
  all: ['user', 'billing', 'records'] as const,
}

export async function fetchBillingRecords(
  params: ListUserBillingRecordsParams,
  signal?: AbortSignal,
): Promise<BillingRecordPage> {
  return resolveApiData(
    listUserBillingRecords(params, { signal }),
    i18n.global.t('account.errors.billingLoadFailed'),
  )
}
