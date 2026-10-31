import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchCreditWallet, creditsQueryKeys } from './resource'
import { WORKSPACE_SELECTION_REQUIRED_CODE } from '../workspaceBilling'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('@/api/http/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'

describe('creditsResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads the workspace balance through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: {
        billing_account_id: 'ba_1',
        workspace_id: WORKSPACE_ID,
        available_credits: 128,
        reserved_credits: 4,
        status: 'active',
      },
    })

    const controller = new AbortController()
    await expect(fetchCreditWallet(WORKSPACE_ID, controller.signal)).resolves.toMatchObject({
      billing_account_id: 'ba_1',
      public_id: WORKSPACE_ID,
      available_credits: 128,
      reserved_credits: 4,
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/credits/balance`,
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
    const scope = { workspaceId: WORKSPACE_ID, billingAccountId: 'ba_1', identity: 'user-1' }
    expect(creditsQueryKeys.wallet(scope.workspaceId, scope.billingAccountId, scope.identity)).toEqual([
      'workspace',
      'credits',
      'wallet',
      WORKSPACE_ID,
      'ba_1',
      'user-1',
    ])
  })

  it('fails closed when the response carries a different payer account', async () => {
    transport.request.mockResolvedValue({
      data: {
        billing_account_id: 'ba_other',
        workspace_id: WORKSPACE_ID,
        available_credits: 128,
        reserved_credits: 0,
        status: 'active',
      },
    })

    await expect(fetchCreditWallet(WORKSPACE_ID, undefined, 'ba_1')).rejects.toThrow()
    expect(transport.request).toHaveBeenCalledOnce()
  })

  it('fails closed with guidance when no workspace is selected', async () => {
    await expect(fetchCreditWallet('')).rejects.toMatchObject({
      code: WORKSPACE_SELECTION_REQUIRED_CODE,
    })
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('surfaces contract error feedback instead of returning an empty wallet', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'billing_wallet_restricted', message: '钱包受限' },
    })

    await expect(fetchCreditWallet(WORKSPACE_ID)).rejects.toThrow('钱包受限')
  })

  it('uses the resource fallback when API feedback contains sensitive details', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'internal_error', message: 'api_key=exposed' },
    })

    await expect(fetchCreditWallet(WORKSPACE_ID)).rejects.toThrow(
      'Could not load your Ideas balance.',
    )
  })
})
