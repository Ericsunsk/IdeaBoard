import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  correctAdminCredits: vi.fn(),
  deductAdminCredits: vi.fn(),
  getAdminCreditReconciliationRun: vi.fn(),
  getAdminCreditWallet: vi.fn(),
  grantAdminCredits: vi.fn(),
  listAdminCreditLedger: vi.fn(),
  listAdminCreditReconciliationFindings: vi.fn(),
  listAdminCreditReconciliationRuns: vi.fn(),
  refundAdminCredits: vi.fn(),
  startAdminCreditReconciliation: vi.fn(),
}))

vi.mock('../../../shared/api/generated-query/admin-credits/admin-credits', () => apiMocks)

import { adminCreditsBillingApi } from './creditsBillingApi'

describe('credits billing API adapter', () => {
  beforeEach(() => {
    vi.resetAllMocks()

    const command = { transaction_id: 'transaction-1' }
    const wallet = { user_id: 12, available_credits: 1200 }
    const ledger = { transactions: [] }
    const run = { id: 7, status: 'completed' }
    const runs = { runs: [run] }
    const findings = { findings: [] }

    apiMocks.getAdminCreditWallet.mockResolvedValue(success(wallet))
    apiMocks.listAdminCreditLedger.mockResolvedValue(success(ledger))
    apiMocks.grantAdminCredits.mockResolvedValue(success(command))
    apiMocks.deductAdminCredits.mockResolvedValue(success(command))
    apiMocks.refundAdminCredits.mockResolvedValue(success(command))
    apiMocks.correctAdminCredits.mockResolvedValue(success(command))
    apiMocks.startAdminCreditReconciliation.mockResolvedValue(success(run))
    apiMocks.listAdminCreditReconciliationRuns.mockResolvedValue(success(runs))
    apiMocks.getAdminCreditReconciliationRun.mockResolvedValue(success(run))
    apiMocks.listAdminCreditReconciliationFindings.mockResolvedValue(success(findings))
  })

  it('keeps every generated Admin Credits call behind the feature-local adapter and returns domain data', async () => {
    const request = { user_id: 12, amount_credits: 5 }
    const wallet = await adminCreditsBillingApi.getWallet(12)
    const ledger = await adminCreditsBillingApi.listLedger(12, { limit: 25 })
    const grant = await adminCreditsBillingApi.grant(request as never)
    const deduction = await adminCreditsBillingApi.deduct(request as never)
    const refund = await adminCreditsBillingApi.refund({
      ...request,
      charge_transaction_id: 'charge-1',
    } as never)
    const correction = await adminCreditsBillingApi.correct(request as never)
    const startedRun = await adminCreditsBillingApi.startReconciliation({
      stale_hold_after_seconds: 3600,
      reason: '运营例行对账',
    })
    const runs = await adminCreditsBillingApi.listReconciliationRuns({ limit: 20 })
    const run = await adminCreditsBillingApi.getReconciliationRun(7)
    const findings = await adminCreditsBillingApi.listReconciliationFindings(7, { status: 'open' })

    expect(wallet).toEqual({ user_id: 12, available_credits: 1200 })
    expect(ledger).toEqual({ transactions: [] })
    expect(grant).toEqual({ transaction_id: 'transaction-1' })
    expect(deduction).toEqual({ transaction_id: 'transaction-1' })
    expect(refund).toEqual({ transaction_id: 'transaction-1' })
    expect(correction).toEqual({ transaction_id: 'transaction-1' })
    expect(startedRun).toEqual({ id: 7, status: 'completed' })
    expect(runs).toEqual({ runs: [{ id: 7, status: 'completed' }] })
    expect(run).toEqual({ id: 7, status: 'completed' })
    expect(findings).toEqual({ findings: [] })

    expect(apiMocks.getAdminCreditWallet).toHaveBeenCalledWith(12)
    expect(apiMocks.listAdminCreditLedger).toHaveBeenCalledWith(12, { limit: 25 })
    expect(apiMocks.grantAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.deductAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.refundAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.correctAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.startAdminCreditReconciliation).toHaveBeenCalledWith({
      stale_hold_after_seconds: 3600,
      reason: '运营例行对账',
    })
    expect(apiMocks.listAdminCreditReconciliationRuns).toHaveBeenCalledWith({ limit: 20 })
    expect(apiMocks.getAdminCreditReconciliationRun).toHaveBeenCalledWith(7)
    expect(apiMocks.listAdminCreditReconciliationFindings).toHaveBeenCalledWith(7, {
      status: 'open',
    })
  })

  it('rejects every Admin Credits business failure with its sanitized domain fallback', async () => {
    for (const mock of Object.values(apiMocks)) {
      mock.mockResolvedValue({ code: 503, msg: 'dial tcp 127.0.0.1:5432' })
    }

    const request = { user_id: 12, amount_credits: 5 }
    const cases: Array<[Promise<unknown>, string]> = [
      [adminCreditsBillingApi.getWallet(12), '加载 Wallet 失败'],
      [adminCreditsBillingApi.listLedger(12), '加载 Ledger 失败'],
      [adminCreditsBillingApi.grant(request as never), '赠送Idea失败'],
      [adminCreditsBillingApi.deduct(request as never), '扣减Idea失败'],
      [adminCreditsBillingApi.refund(request as never), '退款Idea失败'],
      [adminCreditsBillingApi.correct(request as never), '校正Idea失败'],
      [
        adminCreditsBillingApi.startReconciliation({
          stale_hold_after_seconds: 3600,
          reason: '运营例行对账',
        }),
        '启动Idea对账失败',
      ],
      [adminCreditsBillingApi.listReconciliationRuns(), '加载Idea对账记录失败'],
      [adminCreditsBillingApi.getReconciliationRun(7), '加载Idea对账详情失败'],
      [adminCreditsBillingApi.listReconciliationFindings(7), '加载Idea对账异常失败'],
    ]

    for (const [operation, fallback] of cases) {
      await expect(operation).rejects.toThrow(fallback)
    }
  })

  it('preserves a safe business error from the generated response', async () => {
    apiMocks.grantAdminCredits.mockResolvedValue({ code: 409, msg: 'Wallet 已停用' })

    await expect(adminCreditsBillingApi.grant({} as never)).rejects.toThrow('Wallet 已停用')
  })
})

function success<T>(data: T) {
  return { code: 0, msg: 'ok', data }
}
