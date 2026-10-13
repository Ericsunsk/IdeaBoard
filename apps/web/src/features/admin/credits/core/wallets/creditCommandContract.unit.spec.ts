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
    expect(
      validateAdminCreditCommand({ kind: 'grant', userPublicId: publicId, amountCredits: 1.5 }),
    ).toEqual({ valid: false, amountError: '请输入大于 0 的整数Idea数量' })
  })
})
