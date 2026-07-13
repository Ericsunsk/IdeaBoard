import { describe, expect, it } from 'vitest'
import { buildAdminCreditCommandRequest, validateAdminCreditCommand } from './creditCommandContract'

describe('Admin Credit command contract', () => {
  it('builds auditable Grant and Deduction requests with independent idempotency identities', () => {
    const grant = buildAdminCreditCommandRequest(
      { kind: 'grant', userId: 12, amountCredits: 20, reason: ' 活动补偿 ' },
      () => 'grant-operation',
    )
    const deduction = buildAdminCreditCommandRequest(
      { kind: 'deduction', userId: 12, amountCredits: 3, reason: '误发回收' },
      () => 'deduct-operation',
    )

    expect(grant).toEqual({
      user_id: 12,
      amount_credits: 20,
      reason: '活动补偿',
      idempotency_key: 'admin-ui:grant:grant-operation',
      business_reference_id: 'admin-ui:grant:grant-operation',
    })
    expect(deduction.idempotency_key).toBe('admin-ui:deduction:deduct-operation')
  })

  it('allows Refund only from a Charge and rejects amounts above remaining refundable Credits', () => {
    expect(
      validateAdminCreditCommand({
        kind: 'refund',
        userId: 12,
        amountCredits: 6,
        reason: '服务失败',
        chargeTransactionId: 'charge-1',
        remainingRefundableCredits: 5,
      }),
    ).toMatchObject({ valid: false, amountError: '退款不能超过可退余额 5 Ideas' })

    expect(
      validateAdminCreditCommand({
        kind: 'refund',
        userId: 12,
        amountCredits: 5,
        reason: '服务失败',
        remainingRefundableCredits: 5,
      }),
    ).toMatchObject({ valid: false, reasonError: '退款必须从原 Charge 流水发起' })

    expect(
      buildAdminCreditCommandRequest(
        {
          kind: 'refund',
          userId: 12,
          amountCredits: 5,
          reason: '服务失败',
          chargeTransactionId: 'charge-1',
          remainingRefundableCredits: 5,
        },
        () => 'refund-operation',
      ),
    ).toMatchObject({
      charge_transaction_id: 'charge-1',
      idempotency_key: 'admin-ui:refund:refund-operation',
    })
  })

  it('requires positive integer Credits and a non-empty reason', () => {
    expect(
      validateAdminCreditCommand({
        kind: 'grant',
        userId: 12,
        amountCredits: 1.5,
        reason: ' ',
      }),
    ).toEqual({
      valid: false,
      amountError: '请输入大于 0 的整数Idea数量',
      reasonError: '请填写操作原因',
    })
  })
})
