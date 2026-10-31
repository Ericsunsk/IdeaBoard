import { describe, expect, it } from 'vitest'
import {
  isCorrectableTransaction,
  isRefundableCharge,
  validateAdminCreditCommand,
} from './creditCommandContract'

describe('Admin Credit command contract', () => {
  const publicId = 'usr_11111111111111111111111111111111'
  it('accepts valid Grant and Deduction inputs', () => {
    expect(
      validateAdminCreditCommand({ kind: 'grant', userPublicId: publicId, amountCredits: 20 }),
    ).toMatchObject({ valid: true })
    expect(
      validateAdminCreditCommand({ kind: 'deduction', userPublicId: publicId, amountCredits: 3 }),
    ).toMatchObject({ valid: true, amountError: '', targetError: '' })
  })

  it('allows Refund only from a Charge and rejects amounts above remaining refundable Credits', () => {
    expect(
      validateAdminCreditCommand({
        kind: 'refund',
        userPublicId: publicId,
        amountCredits: 6,

        chargeTransactionId: 'charge-1',
        remainingRefundableCredits: 5,
      }),
    ).toMatchObject({ valid: false, amountError: '退款不能超过可退余额 5 Ideas' })

    expect(
      validateAdminCreditCommand({
        kind: 'refund',
        userPublicId: publicId,
        amountCredits: 5,

        remainingRefundableCredits: 5,
      }),
    ).toMatchObject({ valid: false, amountError: '退款必须从原 Charge 流水发起' })

    expect(
      validateAdminCreditCommand({
        kind: 'refund',
        userPublicId: publicId,
        amountCredits: 5,

        chargeTransactionId: 'charge-1',
        remainingRefundableCredits: 5,
      }),
    ).toMatchObject({ valid: true })
  })

  it('requires positive integer Credits', () => {
    const result = validateAdminCreditCommand({
      kind: 'grant',
      userPublicId: publicId,
      amountCredits: 1.5,
    })
    expect(result.valid).toBe(false)
    expect(result.amountError).toBe('请输入大于 0 的整数Idea数量')
    expect(result.targetError).toBe('')
  })

  it('requires exactly one of user or billing account target', () => {
    const accountId = 'ba_orgpool00000000000000000000000001'
    const missing = validateAdminCreditCommand({ kind: 'grant', amountCredits: 5 })
    expect(missing.valid).toBe(false)
    expect(missing.targetError).toBe('请选择用户或资金账户中的其一作为操作目标')
    const ambiguous = validateAdminCreditCommand({
      kind: 'grant',
      userPublicId: publicId,
      billingAccountId: accountId,
      amountCredits: 5,
    })
    expect(ambiguous.valid).toBe(false)
    expect(ambiguous.targetError).toBe('请选择用户或资金账户中的其一作为操作目标')
    expect(
      validateAdminCreditCommand({ kind: 'grant', billingAccountId: accountId, amountCredits: 5 }),
    ).toMatchObject({ valid: true })
  })

  it('allows Correction only from an original transaction within its bound', () => {
    const overBound = validateAdminCreditCommand({
      kind: 'correction',
      userPublicId: publicId,
      amountCredits: 6,
      originalTransactionId: 'ctxn-1',
      originalAmountCredits: 5,
    })
    expect(overBound.valid).toBe(false)
    expect(overBound.amountError).toBe('修正不能超过原交易金额 5 Ideas')

    const unlinked = validateAdminCreditCommand({
      kind: 'correction',
      userPublicId: publicId,
      amountCredits: 5,
      originalAmountCredits: 5,
    })
    expect(unlinked.valid).toBe(false)
    expect(unlinked.amountError).toBe('修正必须从原交易流水发起')

    expect(
      validateAdminCreditCommand({
        kind: 'correction',
        userPublicId: publicId,
        amountCredits: 5,
        originalTransactionId: 'ctxn-1',
        originalAmountCredits: 8,
      }),
    ).toMatchObject({ valid: true })
  })

  it('classifies refundable and correctable transactions without DTOs', () => {
    expect(isRefundableCharge({ command_type: 'capture', charge: null })).toBe(false)
    expect(
      isRefundableCharge({
        command_type: 'capture',
        charge: { remaining_refundable_credits: 2 },
      }),
    ).toBe(true)
    expect(isCorrectableTransaction({ command_type: 'grant', amount_credits: 1 })).toBe(true)
    expect(isCorrectableTransaction({ command_type: 'capture', amount_credits: 1 })).toBe(false)
  })
})
