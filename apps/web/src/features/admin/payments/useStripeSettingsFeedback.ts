import { useAdminToast } from '../shared/lib/toast'
import type { AdminToastPort } from '../shared/lib/toast'

// 反馈公约（admin，见 settings useSettingsFeedback 文件头）：领域文案收口到本文件
// 的 feedback 工厂，调用方只表达意图，不直接拼 toast 文案。

export type StripeSettingsFeedback = {
  saved: () => void
}

export type PaymentOrdersFeedback = {
  refundSubmitted: () => void
}

function createStripeSettingsFeedback(toast: AdminToastPort): StripeSettingsFeedback {
  return {
    saved: () => toast.success('Stripe 设置已保存'),
  }
}

function createPaymentOrdersFeedback(toast: AdminToastPort): PaymentOrdersFeedback {
  return {
    refundSubmitted: () => toast.success('退款已提交，等待 Stripe 最终处理'),
  }
}

export function useStripeSettingsFeedback() {
  return createStripeSettingsFeedback(useAdminToast())
}

export function usePaymentOrdersFeedback() {
  return createPaymentOrdersFeedback(useAdminToast())
}
