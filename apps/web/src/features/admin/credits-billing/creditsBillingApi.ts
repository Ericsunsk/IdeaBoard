import {
  deductAdminCredits,
  getAdminCreditReconciliationRun,
  getAdminCreditWallet,
  grantAdminCredits,
  listAdminCreditLedger,
  listAdminCreditReconciliationFindings,
  listAdminCreditReconciliationRuns,
  refundAdminCredits,
  startAdminCreditReconciliation,
} from '../../../shared/api/generated-query/admin-credits/admin-credits'

export const adminCreditsBillingApi = {
  getWallet: getAdminCreditWallet,
  listLedger: listAdminCreditLedger,
  grant: grantAdminCredits,
  deduct: deductAdminCredits,
  refund: refundAdminCredits,
  startReconciliation: startAdminCreditReconciliation,
  listReconciliationRuns: listAdminCreditReconciliationRuns,
  getReconciliationRun: getAdminCreditReconciliationRun,
  listReconciliationFindings: listAdminCreditReconciliationFindings,
}

export type AdminCreditsBillingApi = typeof adminCreditsBillingApi
