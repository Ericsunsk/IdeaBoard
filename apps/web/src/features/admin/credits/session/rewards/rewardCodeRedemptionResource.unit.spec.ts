import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createRewardCodeRedemptionResource } from './rewardCodeRedemptionResource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('@/api/http/client', () => ({
  api: { request: transport.request },
}))

function contractOk(data: unknown): Record<string, unknown> {
  return { data }
}

function redemptionFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 51,
    reward_code_id: 7,
    user_public_id: 'usr_1',
    display_name: '夏日灵感奖励',
    credited_amount: 80,
    redeemed_at: '2026-07-15T10:00:00Z',
    ...overrides,
  }
}

// Reward redemptions ride the Control Plane with the unified contract
// envelope (`{ data }` / `{ error: { code, message, request_id } }`).
describe('RewardCodeRedemptionResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('lists redemptions through the control plane', async () => {
    transport.request.mockResolvedValue(
      contractOk({ redemptions: [redemptionFixture()], next_before_id: 51 }),
    )
    const signal = new AbortController().signal

    const page = await createRewardCodeRedemptionResource().list(7, {
      beforeId: 60,
      limit: 10,
      signal,
    })

    expect(page).toMatchObject({
      redemptions: [{ id: 51, rewardCodeId: 7, displayName: '夏日灵感奖励' }],
      nextBeforeId: 51,
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/control/v1/rewards/codes/7/redemptions?before_id=60&limit=10',
      expect.objectContaining({ method: 'GET', signal }),
    )
  })

  it('reads one redemption detail through the control plane', async () => {
    transport.request.mockResolvedValue(
      contractOk({
        ...redemptionFixture(),
        credit_transaction_id: 'txn_1',
        masked_code_hint: '•••• 2922',
      }),
    )

    const detail = await createRewardCodeRedemptionResource().get(7, 51)

    expect(detail).toMatchObject({
      id: 51,
      creditTransactionId: 'txn_1',
      maskedCodeHint: '•••• 2922',
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/control/v1/rewards/codes/7/redemptions/51',
      expect.objectContaining({ method: 'GET' }),
    )
  })
})
