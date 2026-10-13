import { formatAdminApiError, type AdminToastPort, useAdminToast } from '../../../shared'

export type AdminCreditsBillingOperationsFeedback = {
  walletLoadFailed: (error: unknown) => void
  ledgerLoadFailed: (error: unknown) => void
  commandSucceeded: (label: string, replayed: boolean) => void
  commandFailed: (error: unknown, label: string) => void
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
  }
}

export function useAdminCreditsBillingOperationsFeedback() {
  return createAdminCreditsBillingOperationsFeedback(useAdminToast())
}

function formatCreditsBillingError(error: unknown, fallback: string) {
  if (error && typeof error === 'object') {
    const candidate = error as { status?: unknown; code?: unknown }
    // Control error-code branches (issue 808): match on code, never copy.
    if (
      candidate.code === 'idempotency_conflict' ||
      candidate.code === 409 ||
      candidate.status === 409
    ) {
      return '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。'
    }
    if (candidate.code === 'billing_wallet_restricted') {
      return '账户当前受限，暂时无法操作。请先处理账户限制。'
    }
  }
  return formatAdminApiError(error, fallback)
}
