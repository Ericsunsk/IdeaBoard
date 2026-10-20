import { describe, expect, it, vi } from 'vitest'
import { usePaymentOrdersFeedback } from './usePaymentOrdersFeedback'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('../../../shared', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../shared')>()),
  useAdminToast: () => toast,
}))

describe('PaymentOrders feedback', () => {
  it('keeps refund success and failure toasts explicit and safe', () => {
    const feedback = usePaymentOrdersFeedback()

    feedback.refundSubmitted()
    feedback.refundFailed(new Error('refund boom'))
    feedback.refundFailed(new Error('dial tcp 127.0.0.1:5432'))

    expect(toast.success).toHaveBeenCalledWith('退款已提交，等待 Stripe 最终处理')
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      'refund boom',
      '发起退款失败，请稍后重试',
    ])
  })
})
