import type {
  CreditReconciliationFinding,
  ListAdminCreditReconciliationFindingQueueParams,
} from '../../../../shared/api/generated-query/model'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../api'
import { stripUndefined } from '../../../../shared/api/stripUndefined'

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
          limit: 25,
        }) satisfies ListAdminCreditReconciliationFindingQueueParams,
        { signal },
      ),
  }
}
