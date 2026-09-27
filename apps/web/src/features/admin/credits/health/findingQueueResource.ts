import type {
  CreditReconciliationFinding,
  ListAdminCreditReconciliationFindingQueueParams,
} from '../shared/api'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../shared/api'
import { stripUndefined } from '../../../../shared/api/stripUndefined'
import { PAGE_SIZES } from '../../../../shared/config/pagination'

export type AdminCreditFindingStatus = 'open' | 'resolved'
export type AdminCreditFinding = CreditReconciliationFinding

export function createAdminCreditFindingQueueResource(
  api: Pick<AdminCreditsBillingApi, 'listReconciliationFindingQueue'> = adminCreditsBillingApi,
) {
  return {
    list: ({
      status,
      beforeId,
      signal,
    }: {
      status: AdminCreditFindingStatus
      beforeId?: number | undefined
      signal: AbortSignal
    }) =>
      api.listReconciliationFindingQueue(
        stripUndefined({
          status,
          before_id: beforeId,
          limit: PAGE_SIZES.adminFindingQueue,
        }) satisfies ListAdminCreditReconciliationFindingQueueParams,
        { signal },
      ),
  }
}
