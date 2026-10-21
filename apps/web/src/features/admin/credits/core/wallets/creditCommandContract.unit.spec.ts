import { describe, expect, it } from 'vitest'
import { buildAdminCreditCommandRequest, validateAdminCreditCommand } from './creditCommandContract'

describe('Admin Credit command contract', () => {
  const publicId = 'usr_11111111111111111111111111111111'
  it('builds auditable Grant and Deduction requests with independent idempotency identities', () => {
    const grant = buildAdminCreditCommandRequest(
      { kind: 'grant', userPublicId: publicId, amountCredits: 20 },
      () => 'grant-operation',
    )
    const deduction = buildAdminCreditCommandRequest(
      { kind: 'deduction', userPublicId: publicId, amountCredits: 3 },
      () => 'deduct-operation',
    )

    expect(grant).toEqual({
      body: {
        user_id: publicId,
        amount_credits: 20,
        business_reference_id: 'control-ui:grant:grant-operation',
      },
      idempotencyKey: 'control-ui:grant:grant-operation',
    })
    expect(deduction.idempotencyKey).toBe('control-ui:deduction:deduct-operation')
    expect(deduction.body).not.toHaveProperty('idempotency_key')
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
      buildAdminCreditCommandRequest(
        {
          kind: 'refund',
          userPublicId: publicId,
          amountCredits: 5,

          chargeTransactionId: 'charge-1',
          remainingRefundableCredits: 5,
        },
        () => 'refund-operation',
      ),
    ).toMatchObject({
      body: {
        user_id: publicId,
        charge_transaction_id: 'charge-1',
      },
      idempotencyKey: 'control-ui:refund:refund-operation',
    })
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
      buildAdminCreditCommandRequest(
        { kind: 'grant', billingAccountId: accountId, amountCredits: 5 },
        () => 'account-grant-operation',
      ),
    ).toMatchObject({
      body: {
        billing_account_id: accountId,
        amount_credits: 5,
      },
      idempotencyKey: 'control-ui:grant:account-grant-operation',
    })
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
      buildAdminCreditCommandRequest(
        {
          kind: 'correction',
          userPublicId: publicId,
          amountCredits: 5,
          originalTransactionId: 'ctxn-1',
          originalAmountCredits: 8,
        },
        () => 'correction-operation',
      ),
    ).toMatchObject({
      body: {
        user_id: publicId,
        original_transaction_id: 'ctxn-1',
      },
      idempotencyKey: 'control-ui:correction:correction-operation',
    })
  })
})
