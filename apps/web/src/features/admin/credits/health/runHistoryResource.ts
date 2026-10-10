import type {
  CreditReconciliationRun,
  ListAdminCreditReconciliationRunsParams,
} from '../shared/api'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../shared/api'
import { stripUndefined } from '../../../../api/stripUndefined'
import { PAGE_SIZES } from '../../../../core/config/pagination'

export type AdminCreditRun = CreditReconciliationRun

export function createAdminCreditRunHistoryResource(
  api: Pick<
    AdminCreditsBillingApi,
    'listReconciliationRuns' | 'getReconciliationRun'
  > = adminCreditsBillingApi,
) {
  return {
    list: ({ beforeId, signal }: { beforeId?: number | undefined; signal: AbortSignal }) =>
      api.listReconciliationRuns(
        stripUndefined({
          before_id: beforeId,
          limit: PAGE_SIZES.standard,
        }) satisfies ListAdminCreditReconciliationRunsParams,
        { signal },
      ),
    get: (runId: number, signal: AbortSignal) => api.getReconciliationRun(runId, { signal }),
  }
}
