import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildWorkspaceBillingCheckoutHeaders,
  classifyWorkspaceBillingAccessError,
  createWorkspaceCheckoutSession,
  fetchWorkspaceBillingAccount,
  fetchWorkspaceBillingRecordsPage,
  fetchWorkspaceCreditsBalance,
  fetchWorkspaceLedgerPage,
  fetchWorkspaceUsage,
  isWorkspaceBillingRestricted,
  workspaceBillingQueryKeys,
  workspaceBillingUrls,
} from './resource'

const apiRequest = vi.hoisted(() => vi.fn())

vi.mock('../../../../shared/api/client', () => ({
  api: { request: apiRequest },
}))

const WORKSPACE_ID = 'ws_abc123'

function contractOk(data: unknown, meta?: Record<string, unknown>) {
  return meta === undefined ? { data } : { data, meta }
}

describe('workspaceBillingResource', () => {
  beforeEach(() => apiRequest.mockReset())

  it('builds canonical workspace billing URLs without generated-query drift', () => {
    expect(workspaceBillingUrls.billingAccount(WORKSPACE_ID)).toContain(
      `/api/v1/workspaces/${WORKSPACE_ID}/billing-account`,
    )
    expect(workspaceBillingUrls.creditsBalance(WORKSPACE_ID)).toContain(
      `/api/v1/workspaces/${WORKSPACE_ID}/credits/balance`,
    )
    expect(workspaceBillingUrls.checkoutSessions(WORKSPACE_ID)).toContain(
      `/api/v1/workspaces/${WORKSPACE_ID}/checkout-sessions`,
    )
    expect(workspaceBillingUrls.checkoutSession(WORKSPACE_ID, 'sess_1')).toContain(
      `/api/v1/workspaces/${WORKSPACE_ID}/checkout-sessions/sess_1`,
    )
  })

  it('maps the billing account to the workspace owner binding', async () => {
    apiRequest.mockResolvedValue(
      contractOk({
        workspace_id: WORKSPACE_ID,
        owner_type: 'workspace',
        created_at: '2026-09-23T12:00:00Z',
      }),
    )

    await expect(fetchWorkspaceBillingAccount(WORKSPACE_ID)).resolves.toMatchObject({
      workspaceId: WORKSPACE_ID,
      ownerType: 'workspace',
      createdAt: '2026-09-23T12:00:00Z',
    })
    expect(apiRequest).toHaveBeenCalledWith(
      expect.stringContaining('/billing-account'),
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('maps integer minor-unit balances with restriction types', async () => {
    apiRequest.mockResolvedValue(
      contractOk({
        workspace_id: WORKSPACE_ID,
        available_credits: 980,
        reserved_credits: 20,
        status: 'active',
        active_restrictions: ['payment_dispute'],
      }),
    )

    await expect(fetchWorkspaceCreditsBalance(WORKSPACE_ID)).resolves.toEqual({
      workspaceId: WORKSPACE_ID,
      availableCredits: 980,
      reservedCredits: 20,
      status: 'active',
      activeRestrictions: ['payment_dispute'],
    })
  })

  it('maps cursor ledger pages with opaque next_cursor + has_more', async () => {
    apiRequest
      .mockResolvedValueOnce(
        contractOk(
          [
            {
              transaction_id: 'tx-3',
              command_type: 'capture',
              amount_credits: 20,
              available_credits_after: 940,
              reserved_credits_after: 0,
              created_at: '2026-09-23T12:03:00Z',
            },
          ],
          { next_cursor: 'opaque-cursor', has_more: true },
        ),
      )
      .mockResolvedValueOnce(contractOk([], { next_cursor: '', has_more: false }))

    const first = await fetchWorkspaceLedgerPage(WORKSPACE_ID, { pageSize: 1 })
    expect(first.entries).toHaveLength(1)
    expect(first.entries[0]).toMatchObject({ transactionId: 'tx-3', amountCredits: 20 })
    expect(first.nextCursor).toBe('opaque-cursor')
    expect(first.hasMore).toBe(true)

    const second = await fetchWorkspaceLedgerPage(WORKSPACE_ID, { cursor: 'opaque-cursor' })
    expect(second.entries).toHaveLength(0)
    expect(second.hasMore).toBe(false)
    expect(apiRequest.mock.calls[1]?.[0]).toContain('cursor=opaque-cursor')
  })

  it('maps billing records without parsing ledger source/reason', async () => {
    apiRequest.mockResolvedValue(
      contractOk(
        [
          {
            record_id: 'record-1',
            occurred_at: '2026-09-23T12:00:00Z',
            kind: 'top_up',
            title: 'Idea 充值',
            credit_delta: 1000,
            available_credits_after: 1980,
          },
        ],
        { next_cursor: '', has_more: false },
      ),
    )

    const page = await fetchWorkspaceBillingRecordsPage(WORKSPACE_ID)
    expect(page.records).toEqual([
      {
        recordId: 'record-1',
        occurredAt: '2026-09-23T12:00:00Z',
        kind: 'top_up',
        title: 'Idea 充值',
        creditDelta: 1000,
        availableCreditsAfter: 1980,
      },
    ])
  })

  it('reads the server-owned usage snapshot for a validated range', async () => {
    apiRequest.mockResolvedValue(
      contractOk({
        timezone: 'Asia/Shanghai',
        lifetime: {
          total_credits: 120,
          peak_daily_credits: 60,
          average_daily_credits: 30,
          task_count: 4,
          active_days: 2,
        },
        trend: [{ date: '2026-09-22', credits: 60 }],
      }),
    )

    const snapshot = await fetchWorkspaceUsage(WORKSPACE_ID, '7d')
    expect(snapshot.lifetime.totalCredits).toBe(120)
    expect(snapshot.trend).toEqual([{ date: '2026-09-22', credits: 60 }])
    await expect(fetchWorkspaceUsage(WORKSPACE_ID, '9d' as never)).rejects.toThrow()
    expect(apiRequest).toHaveBeenCalledTimes(1)
  })

  it('creates checkout with header-only idempotency key and no price fields', async () => {
    apiRequest.mockResolvedValue(
      contractOk({
        session_id: 'sess_1',
        idea_quantity: 1000,
        checkout_url: 'https://checkout.example/s/sess_1',
      }),
    )

    const session = await createWorkspaceCheckoutSession(
      WORKSPACE_ID,
      { ideaQuantity: 1000, termsAccepted: true },
      { idempotencyKey: 'fixed-key-1' },
    )
    expect(session).toMatchObject({ sessionId: 'sess_1', ideaQuantity: 1000 })

    const [, config] = apiRequest.mock.calls[0] as [string, Record<string, unknown>]
    expect(config.method).toBe('POST')
    const headers = config.headers as Record<string, string>
    expect(headers['Idempotency-Key']).toBe('fixed-key-1')
    expect(headers['Request-Id']).toBeTruthy()
    const body = config.body as { data: Record<string, unknown> }
    expect(body.data.idea_quantity).toBe(1000)
    expect(body.data).not.toHaveProperty('unit_price')
    expect(body.data).not.toHaveProperty('amount')
  })

  it('generates a fresh checkout idempotency key per attempt by default', () => {
    const first = buildWorkspaceBillingCheckoutHeaders()
    const second = buildWorkspaceBillingCheckoutHeaders()
    expect(first['Idempotency-Key']).toBeTruthy()
    expect(second['Idempotency-Key']).toBeTruthy()
    expect(first['Idempotency-Key']).not.toBe(second['Idempotency-Key'])
  })

  it('rejects non-workspace ids before any network call', async () => {
    await expect(fetchWorkspaceCreditsBalance('usr_notaws')).rejects.toThrow()
    await expect(fetchWorkspaceBillingAccount('org_123')).rejects.toThrow()
    expect(apiRequest).not.toHaveBeenCalled()
  })

  it('classifies hidden 404 vs forbidden 403 vs restricted wallet', () => {
    expect(classifyWorkspaceBillingAccessError({ status: 404 })).toBe('hidden')
    expect(
      classifyWorkspaceBillingAccessError({ status: 404, code: 'workspace_not_found' }),
    ).toBe('hidden')
    expect(classifyWorkspaceBillingAccessError({ status: 403 })).toBe('forbidden')
    expect(
      classifyWorkspaceBillingAccessError({ status: 403, code: 'billing_wallet_restricted' }),
    ).toBe('restricted')
    expect(
      classifyWorkspaceBillingAccessError({ status: 200, code: 'billing_wallet_restricted' }),
    ).toBe('restricted')
    expect(isWorkspaceBillingRestricted({ status: 403 })).toBe(false)
    expect(isWorkspaceBillingRestricted({ status: 403, code: 'billing_wallet_restricted' })).toBe(
      true,
    )
    expect(
      classifyWorkspaceBillingAccessError({
        status: 403,
        data: { error: { code: 'billing_wallet_restricted' } },
      }),
    ).toBe('restricted')
    expect(workspaceBillingQueryKeys.balance(WORKSPACE_ID)).toEqual([
      'workspace',
      'billing',
      WORKSPACE_ID,
      'credits',
      'balance',
    ])
  })
})
