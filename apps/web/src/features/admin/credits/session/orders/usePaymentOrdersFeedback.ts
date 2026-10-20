import { formatAdminDisplayError, useAdminToast } from '../../../shared'
import type { AdminToastPort } from '../../../shared'

// 反馈公约（admin）：订单退款 toast 文案收口到本 feedback 工厂，调用方只表达意图。

const ORDERS_REFUND_SUBMITTED = '退款已提交，等待 Stripe 最终处理'
const ORDERS_REFUND_FAILED = '发起退款失败，请稍后重试'

export type PaymentOrdersFeedback = {
  refundSubmitted: () => void
  refundFailed: (error: unknown) => void
}

export function createPaymentOrdersFeedback(toast: AdminToastPort): PaymentOrdersFeedback {
  return {
    refundSubmitted: () => toast.success(ORDERS_REFUND_SUBMITTED),
    refundFailed: (error) => toast.error(formatAdminDisplayError(error, ORDERS_REFUND_FAILED)),
  }
}

export function usePaymentOrdersFeedback() {
  return createPaymentOrdersFeedback(useAdminToast())
}
