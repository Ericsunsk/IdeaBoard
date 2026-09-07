import type {
  CreditReconciliationRun,
  ListAdminCreditReconciliationRunsParams,
} from '../../../../shared/api/generated-query/model'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../creditsBillingApi'
import { stripUndefined } from '../../../../shared/api/stripUndefined'

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
          limit: 20,
        }) satisfies ListAdminCreditReconciliationRunsParams,
        { signal },
      ),
    get: (runId: number, signal: AbortSignal) => api.getReconciliationRun(runId, { signal }),
  }
}
