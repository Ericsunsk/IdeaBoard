import { describe, expect, it, vi } from 'vitest'
import { createAdminCreditsBillingOperationsFeedback } from './creditsBillingFeedback'

describe('Admin Credits Billing feedback', () => {
  it('keeps idempotency conflicts explicit without implying a repeated posting', () => {
    const toast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
      info: vi.fn(),
    }
    const feedback = createAdminCreditsBillingOperationsFeedback(toast)

    feedback.commandFailed({ status: 409 }, '调整余额')
    feedback.commandFailed({ code: 409 }, '调整余额')

    expect(toast.error).toHaveBeenNthCalledWith(
      1,
      '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。',
    )
    expect(toast.error).toHaveBeenNthCalledWith(
      2,
      '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。',
    )
  })
})
