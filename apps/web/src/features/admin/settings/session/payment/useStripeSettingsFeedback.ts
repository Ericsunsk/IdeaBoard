import { formatAdminDisplayError, useAdminToast } from '../../../shared/index'
import type { AdminToastPort } from '../../../shared/index'
import { PAYMENTS_MESSAGES } from './messages'

// 反馈公约（admin，见 settings useSettingsFeedback 文件头）：领域文案收口到本文件
// 的 feedback 工厂，调用方只表达意图，不直接拼 toast 文案。

export type StripeSettingsFeedback = {
  saved: () => void
  saveFailed: (error: unknown) => void
  loadFailed: (error: unknown) => void
}

function createStripeSettingsFeedback(toast: AdminToastPort): StripeSettingsFeedback {
  return {
    saved: () => toast.success(PAYMENTS_MESSAGES.stripeSettingsSaved),
    saveFailed: (error) =>
      toast.error(formatAdminDisplayError(error, PAYMENTS_MESSAGES.stripeSaveFailed)),
    loadFailed: (error) =>
      toast.error(formatAdminDisplayError(error, PAYMENTS_MESSAGES.stripeLoadFailed)),
  }
}

export function useStripeSettingsFeedback() {
  return createStripeSettingsFeedback(useAdminToast())
}
