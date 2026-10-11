import { describe, expect, it, vi } from 'vitest'
import { createAdminCreditsBillingOperationsFeedback } from './feedback'

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
    feedback.commandFailed({ code: 'idempotency_conflict' }, '调整余额')
    feedback.commandFailed({ code: 'billing_wallet_restricted' }, '调整余额')
    feedback.commandFailed({ code: 'control_reason_required' }, '调整余额')

    expect(toast.error).toHaveBeenNthCalledWith(
      1,
      '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。',
    )
    expect(toast.error).toHaveBeenNthCalledWith(
      2,
      '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。',
    )
    expect(toast.error).toHaveBeenNthCalledWith(
      3,
      '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。',
    )
    expect(toast.error).toHaveBeenNthCalledWith(4, '账户当前受限，暂时无法操作。请先处理账户限制。')
    expect(toast.error).toHaveBeenNthCalledWith(5, '需要填写操作原因后重试。')
  })
})
