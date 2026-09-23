import { beforeEach, describe, expect, it, vi } from 'vitest'

const controlMocks = vi.hoisted(() => ({
  createControlCreditGrant: vi.fn(),
  createControlCreditDeduction: vi.fn(),
  createControlCreditRefund: vi.fn(),
  getControlCreditWallet: vi.fn(),
  listControlCreditLedger: vi.fn(),
}))

const legacyMocks = vi.hoisted(() => ({
  getAdminCreditHealth: vi.fn(),
  getAdminCreditReconciliationRun: vi.fn(),
  listAdminCreditReconciliationFindingQueue: vi.fn(),
  listAdminCreditReconciliationRuns: vi.fn(),
  startAdminCreditReconciliation: vi.fn(),
}))

vi.mock(
  '../../../../shared/api/generated-query/control/control-credits/control-credits',
  () => controlMocks,
)
vi.mock('../../../../shared/api/generated-query/admin-credits/admin-credits', () => legacyMocks)

import { adminCreditsBillingApi } from './api'

describe('credits billing API adapter', () => {
  const userId = 'usr_11111111111111111111111111111111'
  beforeEach(() => {
    vi.resetAllMocks()

    controlMocks.getControlCreditWallet.mockResolvedValue({
      data: {
        user_id: userId,
        display_name: '测试用户',
        status: 'active',
        available_credits: 1200,
        reserved_credits: 0,
        active_restrictions: [],
      },
    })
    controlMocks.listControlCreditLedger.mockResolvedValue({
      data: [],
      meta: { next_cursor: null, has_more: false },
    })
    const result = (type: string) => ({
      data: {
        transaction_id: 'transaction-1',
        command_type: type,
        amount_credits: 5,
        replayed: false,
        created_at: '2026-09-23T00:00:00Z',
        wallet: {
          user_id: userId,
          display_name: '测试用户',
          available_credits: 1200,
          reserved_credits: 0,
        },
      },
    })
    controlMocks.createControlCreditGrant.mockResolvedValue(result('grant'))
    controlMocks.createControlCreditDeduction.mockResolvedValue(result('deduction'))
    controlMocks.createControlCreditRefund.mockResolvedValue(result('refund'))

    const run = { id: 7, status: 'completed' }
    const runs = { runs: [run] }
    const findings = { findings: [] }
    const health = {
      status: 'healthy',
      open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
      restricted_wallet_count: 0,
    }
    legacyMocks.startAdminCreditReconciliation.mockResolvedValue(success(run))
    legacyMocks.listAdminCreditReconciliationRuns.mockResolvedValue(success(runs))
    legacyMocks.getAdminCreditReconciliationRun.mockResolvedValue(success(run))
    legacyMocks.listAdminCreditReconciliationFindingQueue.mockResolvedValue(success(findings))
    legacyMocks.getAdminCreditHealth.mockResolvedValue(success(health))
  })

  it('reads wallets and ledgers from control and maps them to the dialog model', async () => {
    const wallet = await adminCreditsBillingApi.getWallet(userId)
    const ledger = await adminCreditsBillingApi.listLedger(userId, { page_size: 25 })

    expect(wallet).toEqual({
      public_id: userId,
      display_name: '测试用户',
      status: 'active',
      available_credits: 1200,
      reserved_credits: 0,
      active_restrictions: [],
    })
    expect(ledger).toEqual({ transactions: [], nextCursor: undefined, hasMore: false })
    expect(controlMocks.getControlCreditWallet).toHaveBeenCalledWith(userId, undefined)
    expect(controlMocks.listControlCreditLedger).toHaveBeenCalledWith(
      userId,
      { page_size: 25 },
      undefined,
    )
  })

  it('sends control commands with header-only idempotency keys and operator reasons', async () => {
    const options = { idempotencyKey: 'control-ui:grant:op-1', operatorReason: '运营补偿' }
    const grant = await adminCreditsBillingApi.grant(
      { user_id: userId, amount_credits: 5, business_reference_id: 'control-ui:grant:op-1' },
      options,
    )
    const deduction = await adminCreditsBillingApi.deduct(
      { user_id: userId, amount_credits: 5, business_reference_id: 'control-ui:deduction:op-1' },
      { ...options, idempotencyKey: 'control-ui:deduction:op-1' },
    )
    const refund = await adminCreditsBillingApi.refund(
      {
        user_id: userId,
        amount_credits: 5,
        business_reference_id: 'control-ui:refund:op-1',
        charge_transaction_id: 'charge-1',
      },
      { ...options, idempotencyKey: 'control-ui:refund:op-1' },
    )

    expect(grant.transaction_id).toBe('transaction-1')
    expect(deduction.transaction_id).toBe('transaction-1')
    expect(refund.transaction_id).toBe('transaction-1')
    for (const mock of [
      controlMocks.createControlCreditGrant,
      controlMocks.createControlCreditDeduction,
      controlMocks.createControlCreditRefund,
    ]) {
      expect(mock).toHaveBeenCalledOnce()
      const headers = mock.mock.calls[0]?.[1]?.headers as Record<string, string>
      expect(headers['Idempotency-Key']).toMatch(/^control-ui:(grant|deduction|refund):/)
      expect(headers['X-Operator-Reason']).toBe('运营补偿')
    }
    // Bodies carry no idempotency key (header-only contract).
    expect(controlMocks.createControlCreditGrant.mock.calls[0]?.[0]).not.toHaveProperty(
      'idempotency_key',
    )
  })

  it('keeps reconciliation and health reads on the legacy adapter', async () => {
    const startedRun = await adminCreditsBillingApi.startReconciliation({
      reason: '运营例行检查',
    })
    const runs = await adminCreditsBillingApi.listReconciliationRuns({ limit: 20 })
    const run = await adminCreditsBillingApi.getReconciliationRun(7)
    const findingQueue = await adminCreditsBillingApi.listReconciliationFindingQueue({
      status: 'open',
    })
    const health = await adminCreditsBillingApi.getCreditHealth()

    expect(startedRun).toEqual({ id: 7, status: 'completed' })
    expect(runs).toEqual({ runs: [{ id: 7, status: 'completed' }] })
    expect(run).toEqual({ id: 7, status: 'completed' })
    expect(findingQueue).toEqual({ findings: [] })
    expect(health).toEqual({
      status: 'healthy',
      open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
      restricted_wallet_count: 0,
    })
  })

  it('rejects control failures with domain fallbacks', async () => {
    controlMocks.getControlCreditWallet.mockResolvedValue({
      error: { code: 'control_not_found', message: 'missing', request_id: 'r1' },
    })
    await expect(adminCreditsBillingApi.getWallet(userId)).rejects.toThrow()
  })

  it('branches control error codes to operator copy', async () => {
    controlMocks.createControlCreditDeduction.mockRejectedValue(
      Object.assign(new Error('restricted'), { code: 'billing_wallet_restricted' }),
    )
    // The adapter surfaces the thrown error; feedback maps the code branch.
    await expect(
      adminCreditsBillingApi.deduct(
        { user_id: userId, amount_credits: 1, business_reference_id: 'b1' },
        { idempotencyKey: 'k1', operatorReason: 'r' },
      ),
    ).rejects.toMatchObject({ code: 'billing_wallet_restricted' })
  })
})

function success<T>(data: T) {
  return { code: 0, msg: 'ok', data }
}
