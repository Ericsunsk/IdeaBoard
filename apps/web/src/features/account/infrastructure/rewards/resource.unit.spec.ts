import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchRewardRedemptions, redeemRewardCode, rewardRedemptionsQueryKeys } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'

// Rewards ride the v1 Public Data Plane with workspace scope; the deleted
// `/api/user/rewards/*` routes are gone.
describe('rewardsResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('redeems a code through the v1 public plane with a header-only idempotency key', async () => {
    transport.request.mockResolvedValue({
      data: {
        redemption: {
          id: 7,
          reward_code_id: 3,
          user_public_id: 'usr_1',
          display_name: '夏日灵感奖励',
          credit_amount: 80,
          credit_transaction_id: 'txn_1',
          available_balance_after: 1080,
          redeemed_at: '2026-07-15T10:00:00Z',
        },
        history_item: {
          id: 7,
          reward_code_display_name: '夏日灵感奖励',
          credited_amount: 80,
          masked_code_hint: '•••• 2922',
          redeemed_at: '2026-07-15T10:00:00Z',
        },
        credited_amount: 80,
        available_balance: 1080,
        replayed: false,
      },
    })

    await expect(redeemRewardCode(WORKSPACE_ID, '  IDEA2026  ')).resolves.toMatchObject({
      credited_amount: 80,
      available_balance: 1080,
      replayed: false,
      history_item: { reward_code_display_name: '夏日灵感奖励' },
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/rewards/redemptions`,
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ data: { code: 'IDEA2026' } }),
      }),
    )
    const config = transport.request.mock.calls[0]?.[1] as { headers?: Record<string, string> }
    expect(config.headers?.['Idempotency-Key']).toMatch(/^rewards:redeem:/)
  })

  it('reads a cursor page of redemption history through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: [
        {
          id: 7,
          reward_code_display_name: '夏日灵感奖励',
          credited_amount: 80,
          masked_code_hint: '•••• 2922',
          redeemed_at: '2026-07-15T10:00:00Z',
        },
      ],
      meta: { next_cursor: 'cursor-next', has_more: true },
    })

    const controller = new AbortController()
    await expect(
      fetchRewardRedemptions(
        WORKSPACE_ID,
        { cursor: 'cursor-prev', page_size: 20 },
        controller.signal,
      ),
    ).resolves.toMatchObject({
      redemptions: [{ id: 7, reward_code_display_name: '夏日灵感奖励' }],
      next_cursor: 'cursor-next',
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/rewards/redemptions?cursor=cursor-prev&page_size=20`,
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
  })

  it('keeps the workspace-scoped redemptions query root', () => {
    expect(rewardRedemptionsQueryKeys.all).toEqual(['workspace', 'rewards', 'redemptions'])
    expect(rewardRedemptionsQueryKeys.list(WORKSPACE_ID)).toEqual([
      'workspace',
      'rewards',
      'redemptions',
      WORKSPACE_ID,
    ])
  })
})
