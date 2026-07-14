import { formatAdminApiError, type AdminToastPort, useAdminToast } from '../shared/lib/adminToast'

export type AdminCreditsBillingOperationsFeedback = {
  walletLoadFailed: (error: unknown) => void
  ledgerLoadFailed: (error: unknown) => void
  commandSucceeded: (label: string, replayed: boolean) => void
  commandFailed: (error: unknown, label: string) => void
  reconciliationLoadFailed: (error: unknown) => void
  reconciliationStarted: () => void
  reconciliationStartFailed: (error: unknown) => void
}

export type AdminBillingPricingFeedback = {
  success: (message: string) => void
  error: (error: unknown, fallback: string) => void
}

export function createAdminCreditsBillingOperationsFeedback(
  toast: AdminToastPort,
): AdminCreditsBillingOperationsFeedback {
  return {
    walletLoadFailed: (error) =>
      toast.error(formatCreditsBillingError(error, '加载 Wallet 失败，请稍后重试')),
    ledgerLoadFailed: (error) =>
      toast.error(formatCreditsBillingError(error, '加载Idea流水失败，请稍后重试')),
    commandSucceeded: (label, replayed) =>
      toast.success(replayed ? `${label}已安全重放，未重复记账` : `${label}成功`),
    commandFailed: (error, label) =>
      toast.error(formatCreditsBillingError(error, `${label}失败，请刷新后重试`)),
    reconciliationLoadFailed: (error) =>
      toast.error(formatCreditsBillingError(error, '加载对账报告失败，请稍后重试')),
    reconciliationStarted: () => toast.success('对账任务已提交'),
    reconciliationStartFailed: (error) =>
      toast.error(formatCreditsBillingError(error, '启动对账失败，请稍后重试')),
  }
}

export function createAdminBillingPricingFeedback(
  toast: AdminToastPort,
): AdminBillingPricingFeedback {
  return {
    success: (message) => toast.success(message),
    error: (error, fallback) => toast.error(formatAdminApiError(error, fallback)),
  }
}

export function useAdminCreditsBillingOperationsFeedback() {
  return createAdminCreditsBillingOperationsFeedback(useAdminToast())
}

export function useAdminBillingPricingFeedback() {
  return createAdminBillingPricingFeedback(useAdminToast())
}

function formatCreditsBillingError(error: unknown, fallback: string) {
  if (error && typeof error === 'object') {
    const candidate = error as { status?: unknown; code?: unknown }
    if (candidate.status === 409 || candidate.code === 409) {
      return '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。'
    }
  }
  return formatAdminApiError(error, fallback)
}
