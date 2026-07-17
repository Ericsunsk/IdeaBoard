import {
  deductAdminCredits,
  getAdminCreditHealth,
  getAdminCreditReconciliationRun,
  getAdminCreditWallet,
  grantAdminCredits,
  listAdminCreditLedger,
  listAdminCreditReconciliationFindingQueue,
  listAdminCreditReconciliationRuns,
  refundAdminCredits,
  startAdminCreditReconciliation,
} from '../../../shared/api/generated-query/admin-credits/admin-credits'
import { resolveApiData } from '../../../shared/api/apiResponse'

async function getWallet(...args: Parameters<typeof getAdminCreditWallet>) {
  return resolveApiData(getAdminCreditWallet(...args), '加载 Wallet 失败')
}

async function listLedger(...args: Parameters<typeof listAdminCreditLedger>) {
  return resolveApiData(listAdminCreditLedger(...args), '加载 Ledger 失败')
}

async function grant(...args: Parameters<typeof grantAdminCredits>) {
  return resolveApiData(grantAdminCredits(...args), '赠送Idea失败')
}

async function deduct(...args: Parameters<typeof deductAdminCredits>) {
  return resolveApiData(deductAdminCredits(...args), '扣减Idea失败')
}

async function refund(...args: Parameters<typeof refundAdminCredits>) {
  return resolveApiData(refundAdminCredits(...args), '退款Idea失败')
}

async function getCreditHealth(...args: Parameters<typeof getAdminCreditHealth>) {
  return resolveApiData(getAdminCreditHealth(...args), '加载账本健康状态失败')
}

async function startReconciliation(...args: Parameters<typeof startAdminCreditReconciliation>) {
  return resolveApiData(startAdminCreditReconciliation(...args), '启动Idea对账失败')
}

async function listReconciliationRuns(
  ...args: Parameters<typeof listAdminCreditReconciliationRuns>
) {
  return resolveApiData(listAdminCreditReconciliationRuns(...args), '加载Idea对账记录失败')
}

async function getReconciliationRun(...args: Parameters<typeof getAdminCreditReconciliationRun>) {
  return resolveApiData(getAdminCreditReconciliationRun(...args), '加载Idea对账详情失败')
}

async function listReconciliationFindingQueue(
  ...args: Parameters<typeof listAdminCreditReconciliationFindingQueue>
) {
  return resolveApiData(listAdminCreditReconciliationFindingQueue(...args), '加载账本异常队列失败')
}

export const adminCreditsBillingApi = {
  getWallet,
  listLedger,
  grant,
  deduct,
  refund,
  getCreditHealth,
  startReconciliation,
  listReconciliationRuns,
  getReconciliationRun,
  listReconciliationFindingQueue,
}

export type AdminCreditsBillingApi = typeof adminCreditsBillingApi
