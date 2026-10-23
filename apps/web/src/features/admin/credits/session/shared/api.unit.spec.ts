import { beforeEach, describe, expect, it, vi } from 'vitest'

const controlMocks = vi.hoisted(() => ({
  createControlCreditGrant: vi.fn(),
  createControlCreditDeduction: vi.fn(),
  createControlCreditRefund: vi.fn(),
  createControlCreditCorrection: vi.fn(),
  getControlCreditWallet: vi.fn(),
  getControlBillingAccountWallet: vi.fn(),
  listControlCreditLedger: vi.fn(),
  listControlBillingAccountLedger: vi.fn(),
  restrictControlWallet: vi.fn(),
  releaseControlWalletRestriction: vi.fn(),
  getControlCreditHealth: vi.fn(),
  startControlReconciliation: vi.fn(),
  listControlReconciliationRuns: vi.fn(),
  getControlReconciliationRun: vi.fn(),
  listControlReconciliationFindings: vi.fn(),
}))

const billingMocks = vi.hoisted(() => ({
  listControlBillingAccounts: vi.fn(),
  getControlBillingAccount: vi.fn(),
}))

vi.mock(
  '../../../../../api/generated-query/control/control-credits/control-credits',
  () => controlMocks,
)

vi.mock(
  '../../../../../api/generated-query/control/control-billing/control-billing',
  () => billingMocks,
)

import type { ControlWalletRestrictionResult } from '@/api/generated-query/control/model/controlWalletRestrictionResult'
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
    controlMocks.createControlCreditCorrection.mockResolvedValue(result('correction'))
    controlMocks.getControlBillingAccountWallet.mockResolvedValue({
      data: {
        billing_account_id: 'ba_orgpool00000000000000000000000001',
        owner_type: 'organization',
        status: 'active',
        available_credits: 5000,
        reserved_credits: 10,
        active_restrictions: [],
      },
    })
    controlMocks.listControlBillingAccountLedger.mockResolvedValue({
      data: [],
      meta: { next_cursor: null, has_more: false },
    })
    controlMocks.restrictControlWallet.mockResolvedValue({
      data: {
        restriction_id: '00000000-0000-4000-8000-000000000001',
        replayed: false,
      } satisfies ControlWalletRestrictionResult,
    })
    controlMocks.releaseControlWalletRestriction.mockResolvedValue({
      data: {
        restriction_id: '00000000-0000-4000-8000-000000000001',
        replayed: false,
      } satisfies ControlWalletRestrictionResult,
    })
    billingMocks.listControlBillingAccounts.mockResolvedValue({
      data: [
        {
          billing_account_id: 'ba_orgpool00000000000000000000000001',
          owner_type: 'organization',
          created_at: '2026-09-23T00:00:00Z',
          updated_at: '2026-09-23T00:00:00Z',
        },
      ],
      meta: { next_cursor: null, has_more: false },
    })
    billingMocks.getControlBillingAccount.mockResolvedValue({
      data: {
        billing_account_id: 'ba_orgpool00000000000000000000000001',
        owner_type: 'organization',
        linked_workspaces: [],
        members: [],
        wallet: { available_credits: 5000, reserved_credits: 10, status: 'active' },
        created_at: '2026-09-23T00:00:00Z',
        updated_at: '2026-09-23T00:00:00Z',
      },
    })

    const run = { id: 7, status: 'completed' }
    const runs = { runs: [run] }
    const findings = { findings: [] }
    const health = {
      status: 'healthy',
      open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
      restricted_wallet_count: 0,
    }
    controlMocks.startControlReconciliation.mockResolvedValue({ data: run })
    controlMocks.listControlReconciliationRuns.mockResolvedValue({ data: runs })
    controlMocks.getControlReconciliationRun.mockResolvedValue({ data: run })
    controlMocks.listControlReconciliationFindings.mockResolvedValue({ data: findings })
    controlMocks.getControlCreditHealth.mockResolvedValue({ data: health })
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

  it('sends control commands with request and idempotency headers', async () => {
    const options = { idempotencyKey: 'control-ui:grant:op-1' }
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
      expect(headers).toEqual({
        'Request-Id': expect.any(String),
        'Idempotency-Key': expect.stringMatching(/^control-ui:(grant|deduction|refund):/),
      })
    }
    // Bodies carry no idempotency key (header-only contract).
    expect(controlMocks.createControlCreditGrant.mock.calls[0]?.[0]).not.toHaveProperty(
      'idempotency_key',
    )
  })

  it('addresses explicit billing accounts without user linkage', async () => {
    const accountId = 'ba_orgpool00000000000000000000000001'
    const wallet = await adminCreditsBillingApi.getAccountWallet(accountId)
    expect(wallet).toMatchObject({
      billing_account_id: accountId,
      owner_type: 'organization',
      available_credits: 5000,
    })
    expect(wallet.public_id).toBe(accountId)
    expect(controlMocks.getControlBillingAccountWallet).toHaveBeenCalledWith(accountId, undefined)

    const ledger = await adminCreditsBillingApi.listAccountLedger(accountId, { page_size: 25 })
    expect(ledger).toEqual({ transactions: [], nextCursor: undefined, hasMore: false })

    const correction = await adminCreditsBillingApi.correct(
      {
        billing_account_id: accountId,
        amount_credits: 5,
        business_reference_id: 'control-ui:correction:op-1',
        original_transaction_id: 'ctxn-1',
      },
      { idempotencyKey: 'control-ui:correction:op-1' },
    )
    expect(correction.transaction_id).toBe('transaction-1')
    const correctionHeaders = controlMocks.createControlCreditCorrection.mock.calls[0]?.[1]
      ?.headers as Record<string, string>
    expect(correctionHeaders['Idempotency-Key']).toBe('control-ui:correction:op-1')
  })

  it('creates and releases wallet restrictions on the explicit account', async () => {
    const accountId = 'ba_orgpool00000000000000000000000001'
    const created = await adminCreditsBillingApi.restrictWallet(
      {
        billing_account_id: accountId,
        type: 'spending',
        source: 'control_review',
        related_object_type: 'finding',
        related_object_id: '42',
      },
      { idempotencyKey: 'control-ui:restrict:op-1' },
    )
    expect(created.restriction_id).toBe('00000000-0000-4000-8000-000000000001')

    const released = await adminCreditsBillingApi.releaseWalletRestriction(
      '00000000-0000-4000-8000-000000000001',
      {},
      { idempotencyKey: 'control-ui:release:op-1' },
    )
    expect(released.restriction_id).toBe('00000000-0000-4000-8000-000000000001')
    expect(controlMocks.releaseControlWalletRestriction).toHaveBeenCalledWith(
      '00000000-0000-4000-8000-000000000001',
      {},
      expect.objectContaining({
        headers: expect.objectContaining({ 'Idempotency-Key': 'control-ui:release:op-1' }),
      }),
    )
  })

  it('lists and reads billing accounts for admin audit', async () => {
    const page = await adminCreditsBillingApi.listBillingAccounts({ owner_scope: 'organization' })
    expect(page.accounts).toHaveLength(1)
    expect(page.accounts[0]?.billing_account_id).toBe('ba_orgpool00000000000000000000000001')
    expect(page.hasMore).toBe(false)

    const detail = await adminCreditsBillingApi.getBillingAccount(
      'ba_orgpool00000000000000000000000001',
    )
    expect(detail.owner_type).toBe('organization')
    expect(detail.wallet?.available_credits).toBe(5000)
  })

  it('reads reconciliation and health through the control adapter', async () => {
    const startedRun = await adminCreditsBillingApi.startReconciliation()
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
        { idempotencyKey: 'k1' },
      ),
    ).rejects.toMatchObject({ code: 'billing_wallet_restricted' })
  })
})
