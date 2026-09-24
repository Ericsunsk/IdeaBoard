import { api } from '../../../../shared/api/client'
import { resolveApiData, type ApiResponseWithData } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

// User billing records boundary (issue #809). The generated `user-billing`
// client is gone but the backend user plane still serves
// `GET /api/user/billing/records` with the shared envelope, so this boundary
// transports through the central `api.request` with local view types.
export type BillingRecordKind = string

export type BillingRecord = {
  record_id: string
  occurred_at: string
  kind: BillingRecordKind
  title: string
  credit_delta: number
  available_credits_after: number
}

export type BillingRecordPage = {
  records: BillingRecord[]
  next_before_sequence?: number
}

export type BillingRecordItem = BillingRecord
export type BillingRecordPageData = BillingRecordPage

export type ListUserBillingRecordsParams = {
  before_sequence?: number
  limit?: number
}

export const billingRecordsQueryKeys = {
  all: ['user', 'billing', 'records'] as const,
}

function billingRecordsUrl(params: ListUserBillingRecordsParams): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value !== undefined) search.append(key, String(value))
  }
  const query = search.toString()
  return query ? `/user/billing/records?${query}` : '/user/billing/records'
}

export async function fetchBillingRecords(
  params: ListUserBillingRecordsParams,
  signal?: AbortSignal,
): Promise<BillingRecordPage> {
  return resolveApiData(
    api.request<ApiResponseWithData<BillingRecordPage>>(billingRecordsUrl(params), {
      method: 'GET',
      ...(signal == null ? {} : { signal }),
    }),
    i18n.global.t('account.errors.billingLoadFailed'),
  )
}
