import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  deductAdminCredits: vi.fn(),
  getAdminCreditHealth: vi.fn(),
  getAdminCreditReconciliationRun: vi.fn(),
  getAdminCreditWallet: vi.fn(),
  grantAdminCredits: vi.fn(),
  listAdminCreditLedger: vi.fn(),
  listAdminCreditReconciliationFindingQueue: vi.fn(),
  listAdminCreditReconciliationRuns: vi.fn(),
  refundAdminCredits: vi.fn(),
  startAdminCreditReconciliation: vi.fn(),
}))

vi.mock('../../../shared/api/generated-query/admin-credits/admin-credits', () => apiMocks)

import { adminCreditsBillingApi } from './creditsBillingApi'

describe('credits billing API adapter', () => {
  const publicId = '11111111-1111-4111-8111-111111111111'
  beforeEach(() => {
    vi.resetAllMocks()

    const command = { transaction_id: 'transaction-1' }
    const wallet = { public_id: publicId, display_name: '测试用户', available_credits: 1200, reserved_credits: 0 }
    const ledger = { transactions: [] }
    const run = { id: 7, status: 'completed' }
    const runs = { runs: [run] }
    const findings = { findings: [] }
    const health = {
      status: 'healthy',
      open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
      restricted_wallet_count: 0,
    }

    apiMocks.getAdminCreditWallet.mockResolvedValue(success(wallet))
    apiMocks.listAdminCreditLedger.mockResolvedValue(success(ledger))
    apiMocks.grantAdminCredits.mockResolvedValue(success(command))
    apiMocks.deductAdminCredits.mockResolvedValue(success(command))
    apiMocks.refundAdminCredits.mockResolvedValue(success(command))
    apiMocks.startAdminCreditReconciliation.mockResolvedValue(success(run))
    apiMocks.listAdminCreditReconciliationRuns.mockResolvedValue(success(runs))
    apiMocks.getAdminCreditReconciliationRun.mockResolvedValue(success(run))
    apiMocks.listAdminCreditReconciliationFindingQueue.mockResolvedValue(success(findings))
    apiMocks.getAdminCreditHealth.mockResolvedValue(success(health))
  })

  it('keeps every generated Admin Credits call behind the feature-local adapter and returns domain data', async () => {
    const request = { user_public_id: publicId, amount_credits: 5 }
    const wallet = await adminCreditsBillingApi.getWallet(publicId)
    const ledger = await adminCreditsBillingApi.listLedger(publicId, { limit: 25 })
    const grant = await adminCreditsBillingApi.grant(request as never)
    const deduction = await adminCreditsBillingApi.deduct(request as never)
    const refund = await adminCreditsBillingApi.refund({
      ...request,
      charge_transaction_id: 'charge-1',
    } as never)
    const startedRun = await adminCreditsBillingApi.startReconciliation({
      reason: '运营例行检查',
    })
    const runs = await adminCreditsBillingApi.listReconciliationRuns({ limit: 20 })
    const run = await adminCreditsBillingApi.getReconciliationRun(7)
    const findingQueue = await adminCreditsBillingApi.listReconciliationFindingQueue({
      status: 'open',
    })
    const health = await adminCreditsBillingApi.getCreditHealth()

    expect(wallet).toEqual({ public_id: publicId, display_name: '测试用户', available_credits: 1200, reserved_credits: 0 })
    expect(ledger).toEqual({ transactions: [] })
    expect(grant).toEqual({ transaction_id: 'transaction-1' })
    expect(deduction).toEqual({ transaction_id: 'transaction-1' })
    expect(refund).toEqual({ transaction_id: 'transaction-1' })
    expect(startedRun).toEqual({ id: 7, status: 'completed' })
    expect(runs).toEqual({ runs: [{ id: 7, status: 'completed' }] })
    expect(run).toEqual({ id: 7, status: 'completed' })
    expect(findingQueue).toEqual({ findings: [] })
    expect(health).toEqual({
      status: 'healthy',
      open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
      restricted_wallet_count: 0,
    })

    expect(apiMocks.getAdminCreditWallet).toHaveBeenCalledWith(publicId)
    expect(apiMocks.listAdminCreditLedger).toHaveBeenCalledWith(publicId, { limit: 25 })
    expect(apiMocks.grantAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.deductAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.refundAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.startAdminCreditReconciliation).toHaveBeenCalledWith({
      reason: '运营例行检查',
    })
    expect(apiMocks.listAdminCreditReconciliationRuns).toHaveBeenCalledWith({ limit: 20 })
    expect(apiMocks.getAdminCreditReconciliationRun).toHaveBeenCalledWith(7)
    expect(apiMocks.listAdminCreditReconciliationFindingQueue).toHaveBeenCalledWith({
      status: 'open',
    })
    expect(apiMocks.getAdminCreditHealth).toHaveBeenCalledOnce()
  })

  it('rejects every Admin Credits business failure with its sanitized domain fallback', async () => {
    for (const mock of Object.values(apiMocks)) {
      mock.mockResolvedValue({ code: 503, msg: 'dial tcp 127.0.0.1:5432' })
    }

    const request = { user_public_id: publicId, amount_credits: 5 }
    const cases: Array<[Promise<unknown>, string]> = [
      [adminCreditsBillingApi.getWallet(publicId), '加载 Wallet 失败'],
      [adminCreditsBillingApi.listLedger(publicId), '加载 Ledger 失败'],
      [adminCreditsBillingApi.grant(request as never), '赠送Idea失败'],
      [adminCreditsBillingApi.deduct(request as never), '扣减Idea失败'],
      [adminCreditsBillingApi.refund(request as never), '退款Idea失败'],
      [
        adminCreditsBillingApi.startReconciliation({
          reason: '运营例行检查',
        }),
        '启动账本检查失败',
      ],
      [adminCreditsBillingApi.listReconciliationRuns(), '加载账本检查记录失败'],
      [adminCreditsBillingApi.getReconciliationRun(7), '加载账本检查详情失败'],
      [
        adminCreditsBillingApi.listReconciliationFindingQueue({ status: 'open' }),
        '加载账本异常队列失败',
      ],
      [adminCreditsBillingApi.getCreditHealth(), '加载账本健康状态失败'],
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
