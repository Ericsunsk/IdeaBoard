import { listWorkspaceBillingRecords } from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { PaginatedBillingRecords } from '../../../../api/generated-query/public/model'
import { i18n } from '../../../../core/i18n'
import { asWorkspaceBillingError, readContractListBody, requireAccountWorkspaceId } from '../workspaceBilling'

// User billing records boundary (W2 transports). Reads go through the Public
// Data Plane (`GET /api/v1/workspaces/{id}/billing-records`, cursor
// pagination over the opaque `next_cursor`) with local view types; the
// deleted `GET /api/user/billing/records` route is gone.
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
  next_cursor?: string
}

export type BillingRecordItem = BillingRecord
export type BillingRecordPageData = BillingRecordPage

export type ListUserBillingRecordsParams = {
  cursor?: string
  page_size?: number
}

export const billingRecordsQueryKeys = {
  all: ['workspace', 'billing', 'records'] as const,
  list: (workspaceId: string) => ['workspace', 'billing', 'records', workspaceId] as const,
}

export async function fetchBillingRecords(
  workspaceId: string,
  params: ListUserBillingRecordsParams,
  signal?: AbortSignal,
): Promise<BillingRecordPage> {
  const id = requireAccountWorkspaceId(workspaceId)
  const fallback = i18n.global.t('account.errors.billingLoadFailed')
  // Paginated list bodies already carry the contract `{data, meta}` shape,
  // so they are read directly instead of unwrapped like `{data}` envelopes.
  let body: unknown
  try {
    body = await listWorkspaceBillingRecords(
      id,
      {
        ...(params.cursor === undefined ? {} : { cursor: params.cursor }),
        ...(params.page_size === undefined ? {} : { page_size: params.page_size }),
      },
      { signal },
    )
  } catch (error) {
    throw asWorkspaceBillingError(error, fallback)
  }
  const page = readContractListBody<PaginatedBillingRecords>(body, fallback)
  if (!Array.isArray(page?.data) || page?.meta == null) throw new Error(fallback)
  return {
    records: page.data.map((record) => ({
      record_id: record.record_id,
      occurred_at: record.occurred_at,
      kind: record.kind,
      title: record.title,
      credit_delta: record.credit_delta,
      available_credits_after: record.available_credits_after,
    })),
    ...(page.meta.has_more && page.meta.next_cursor ? { next_cursor: page.meta.next_cursor } : {}),
  }
}
