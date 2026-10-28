import { defineFeedback } from '../../../shared'

const ORDERS_REFUND_SUBMITTED = '退款已提交，等待 Stripe 最终处理'
const ORDERS_REFUND_FAILED = '发起退款失败，请稍后重试'
const ORDERS_LOAD_FAILED = '支付订单加载失败，请重试。'
const ORDERS_DETAIL_LOAD_FAILED = '支付订单详情加载失败，请重试。'

export type PaymentOrdersFeedback = {
  refundSubmitted: () => void
  refundFailed: (error: unknown) => void
  loadFailed: (error: unknown) => void
  detailFailed: (error: unknown) => void
}

const PAYMENT_ORDERS_COPY = {
  refundSubmitted: { success: ORDERS_REFUND_SUBMITTED },
  refundFailed: { error: ORDERS_REFUND_FAILED },
  loadFailed: { error: ORDERS_LOAD_FAILED },
  detailFailed: { error: ORDERS_DETAIL_LOAD_FAILED },
}

export function usePaymentOrdersFeedback(): PaymentOrdersFeedback {
  return defineFeedback<PaymentOrdersFeedback>(PAYMENT_ORDERS_COPY)
}
