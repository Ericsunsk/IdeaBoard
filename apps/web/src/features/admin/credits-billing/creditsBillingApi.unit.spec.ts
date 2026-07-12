import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
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
  beforeEach(() => vi.clearAllMocks())

  it('keeps every generated Admin Credits call behind the feature-local adapter', async () => {
    const request = { user_id: 12, amount_credits: 5 }
    await adminCreditsBillingApi.getWallet(12)
    await adminCreditsBillingApi.listLedger(12, { limit: 25 })
    await adminCreditsBillingApi.grant(request as never)
    await adminCreditsBillingApi.deduct(request as never)
    await adminCreditsBillingApi.refund({ ...request, charge_transaction_id: 'charge-1' } as never)
    await adminCreditsBillingApi.startReconciliation({
      stale_hold_after_seconds: 3600,
      reason: '运营例行对账',
    })
    await adminCreditsBillingApi.listReconciliationRuns({ limit: 20 })
    await adminCreditsBillingApi.getReconciliationRun(7)
    await adminCreditsBillingApi.listReconciliationFindings(7, { status: 'open' })

    expect(apiMocks.getAdminCreditWallet).toHaveBeenCalledWith(12)
    expect(apiMocks.listAdminCreditLedger).toHaveBeenCalledWith(12, { limit: 25 })
    expect(apiMocks.grantAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.deductAdminCredits).toHaveBeenCalledOnce()
    expect(apiMocks.refundAdminCredits).toHaveBeenCalledOnce()
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
})
