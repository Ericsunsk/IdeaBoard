import { listWorkspaceBillingRecords } from '../../../../api/generated-query/public/workspace-billing/workspace-billing'
import type { PaginatedBillingRecords } from '../../../../api/generated-query/public/model'
import { i18n } from '../../../../core/i18n'
import {
  asWorkspaceBillingError,
  readContractListBody,
  requireAccountWorkspaceId,
} from '../workspaceBilling'

// User billing records boundary (v1 public plane).
//
// Reads go through `GET /api/v1/workspaces/{id}/billing-records`, cursor
// pagination over the opaque `next_cursor`, scoped to the Workspace's
// stable Billing Account. Settlement attributes the real operator and the
// consuming Workspace and never rewrites history on member exit or owner
// change. There is no owner-wallet projection and no fallback.
export type BillingRecordKind = string

export type BillingRecord = {
  record_id: string
  occurred_at: string
  kind: BillingRecordKind
  title: string
  credit_delta: number
  available_credits_after?: number
}

export type BillingRecordPage = {
  records: BillingRecord[]
  next_cursor?: string
}

export type ListUserBillingRecordsParams = {
  cursor?: string
  page_size?: number
}

export const billingRecordsQueryKeys = {
  all: ['workspace', 'billing', 'records'] as const,
  // Scalar-only key: workspace + account + identity + scope. Never embed a
  // scope object so key equality stays referentially stable across renders.
  list: (
    workspaceId: string,
    billingAccountId: string,
    identity: string,
    consumptionScope: string,
  ) =>
    [
      'workspace',
      'billing',
      'records',
      workspaceId,
      billingAccountId,
      identity,
      consumptionScope,
    ] as const,
}

// Named handover-scope matcher for billing-records keys: the single owner of
// the ['workspace','billing','records', workspaceId, billingAccountId,
// identity, ...] shape. Handover invalidation predicates consult this instead
// of raw queryKey[3..5] subscripts at the call site, so key-shape drift
// surfaces here (request-count gate: scalar-only keys registered in
// queryKeyRequestBudget.contract.spec.ts).
export function isBillingRecordsQueryForHandover(
  queryKey: readonly unknown[],
  workspaces: ReadonlySet<string>,
  billingAccountId: string | undefined,
  viewer: string,
): boolean {
  if (billingAccountId == null || billingAccountId === '') return false
  if (queryKey.length < 6) return false
  if (queryKey[0] !== 'workspace' || queryKey[1] !== 'billing' || queryKey[2] !== 'records') {
    return false
  }
  const workspaceId = queryKey[3]
  return (
    typeof workspaceId === 'string' &&
    workspaces.has(workspaceId) &&
    queryKey[4] === billingAccountId &&
    queryKey[5] === viewer
  )
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
      ...(record.available_credits_after === undefined
        ? {}
        : { available_credits_after: record.available_credits_after }),
    })),
    ...(page.meta.has_more && page.meta.next_cursor ? { next_cursor: page.meta.next_cursor } : {}),
  }
}
