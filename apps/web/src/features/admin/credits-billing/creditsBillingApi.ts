import {
  correctAdminCredits,
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
import { requireApiData } from '../../../shared/api/apiResponse'

async function getWallet(...args: Parameters<typeof getAdminCreditWallet>) {
  return requireApiData(await getAdminCreditWallet(...args), '加载 Wallet 失败')
}

async function listLedger(...args: Parameters<typeof listAdminCreditLedger>) {
  return requireApiData(await listAdminCreditLedger(...args), '加载 Ledger 失败')
}

async function grant(...args: Parameters<typeof grantAdminCredits>) {
  return requireApiData(await grantAdminCredits(...args), '赠送Idea失败')
}

async function deduct(...args: Parameters<typeof deductAdminCredits>) {
  return requireApiData(await deductAdminCredits(...args), '扣减Idea失败')
}

async function refund(...args: Parameters<typeof refundAdminCredits>) {
  return requireApiData(await refundAdminCredits(...args), '退款Idea失败')
}

async function correct(...args: Parameters<typeof correctAdminCredits>) {
  return requireApiData(await correctAdminCredits(...args), '校正Idea失败')
}

async function startReconciliation(...args: Parameters<typeof startAdminCreditReconciliation>) {
  return requireApiData(await startAdminCreditReconciliation(...args), '启动Idea对账失败')
}

async function listReconciliationRuns(
  ...args: Parameters<typeof listAdminCreditReconciliationRuns>
) {
  return requireApiData(
    await listAdminCreditReconciliationRuns(...args),
    '加载Idea对账记录失败',
  )
}

async function getReconciliationRun(...args: Parameters<typeof getAdminCreditReconciliationRun>) {
  return requireApiData(await getAdminCreditReconciliationRun(...args), '加载Idea对账详情失败')
}

async function listReconciliationFindings(
  ...args: Parameters<typeof listAdminCreditReconciliationFindings>
) {
  return requireApiData(
    await listAdminCreditReconciliationFindings(...args),
    '加载Idea对账异常失败',
  )
}

export const adminCreditsBillingApi = {
  getWallet,
  listLedger,
  grant,
  deduct,
  refund,
  correct,
  startReconciliation,
  listReconciliationRuns,
  getReconciliationRun,
  listReconciliationFindings,
}

export type AdminCreditsBillingApi = typeof adminCreditsBillingApi
