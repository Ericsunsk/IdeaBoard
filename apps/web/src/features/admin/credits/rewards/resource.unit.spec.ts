import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCodeResource } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

const rewardCode = {
  id: 7,
  display_name: '新用户奖励',
  masked_code_hint: '•••• 2026',
  credit_amount: 80,
  max_redemptions: 100,
  redemption_count: 12,
  enabled: true,
  status: 'enabled' as const,
  created_at: '2026-07-20T10:00:00Z',
  updated_at: '2026-07-20T10:00:00Z',
}

describe('AdminRewardCodeResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps search metadata without exposing digest or plaintext', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_codes: [rewardCode], total: 42, next_before_id: 6 },
    })
    const signal = new AbortController().signal

    const page = await createAdminRewardCodeResource().search({
      query: ' 新用户奖励 ',
      status: 'enabled',
      beforeId: 8,
      signal,
    })

    expect(transport.request).toHaveBeenCalledWith(
      '/admin/rewards/codes/search',
      expect.objectContaining({
        method: 'POST',
        body: { query: '新用户奖励', status: 'enabled', before_id: 8, limit: 20 },
        signal,
      }),
    )
    expect(page).toEqual({
      rewardCodes: [
        expect.objectContaining({
          id: 7,
          displayName: '新用户奖励',
          maskedCodeHint: '•••• 2026',
          status: 'enabled',
        }),
      ],
      total: 42,
      nextBeforeId: 6,
    })
  })

  it('omits an empty query and unset status from the body', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_codes: [], total: 0 },
    })

    await createAdminRewardCodeResource().search({ query: '   ' })

    expect(transport.request).toHaveBeenCalledWith(
      '/admin/rewards/codes/search',
      expect.objectContaining({ method: 'POST', body: { limit: 20 } }),
    )
  })

  it('preserves the client idempotency key and converts Shanghai expiration', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_code: rewardCode, plaintext: 'ABCD12342026', replayed: false },
    })

    const result = await createAdminRewardCodeResource().create({
      displayName: ' 新用户奖励 ',
      code: 'ABCD-1234-2026',
      creditAmount: 80,
      maxRedemptions: 100,
      expiresAt: '2026-08-01T00:00',
      idempotencyKey: 'stable-create-key',
    })

    expect(transport.request).toHaveBeenCalledWith(
      '/admin/rewards/codes',
      expect.objectContaining({
        method: 'POST',
        body: {
          display_name: '新用户奖励',
          code: 'ABCD-1234-2026',
          credit_amount: 80,
          max_redemptions: 100,
          expires_at: '2026-07-31T16:00:00.000Z',
          idempotency_key: 'stable-create-key',
        },
      }),
    )
    expect(result).toEqual({
      rewardCode: expect.objectContaining({ id: 7, displayName: '新用户奖励' }),
      plaintext: 'ABCD12342026',
      replayed: false,
    })
  })

  it('maps atomic rule and Code updates, enabled intent, and delete through admin APIs', async () => {
    transport.request
      .mockResolvedValueOnce({
        code: 0,
        msg: 'ok',
        data: {
          reward_code: {
            ...rewardCode,
            display_name: '八月奖励',
            masked_code_hint: '•••• WXYZ',
            expires_at: undefined,
          },
          plaintext: 'NEWC-ODE1-WXYZ',
          replayed: false,
        },
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: 'ok',
        data: { ...rewardCode, enabled: false, status: 'disabled' },
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: 'ok',
        data: { reward_code_id: 7 },
      })
    const resource = createAdminRewardCodeResource()

    await expect(
      resource.update(7, {
        displayName: ' 八月奖励 ',
        creditAmount: 120,
        maxRedemptions: 0,
        expiresAt: '',
        code: ' NEWC-ODE1-WXYZ ',
      }),
    ).resolves.toMatchObject({
      rewardCode: { displayName: '八月奖励', maskedCodeHint: '•••• WXYZ' },
      plaintext: 'NEWC-ODE1-WXYZ',
      replayed: false,
    })
    expect(transport.request).toHaveBeenNthCalledWith(
      1,
      '/admin/rewards/codes/7',
      expect.objectContaining({
        method: 'PATCH',
        body: {
          display_name: '八月奖励',
          credit_amount: 120,
          max_redemptions: 0,
          expires_at: null,
          code: 'NEWC-ODE1-WXYZ',
        },
      }),
    )

    await expect(resource.setEnabled(7, false)).resolves.toMatchObject({
      enabled: false,
      status: 'disabled',
    })
    expect(transport.request).toHaveBeenNthCalledWith(
      2,
      '/admin/rewards/codes/7/enabled',
      expect.objectContaining({ method: 'PATCH', body: { enabled: false } }),
    )

    await expect(resource.delete(7)).resolves.toBeUndefined()
    expect(transport.request).toHaveBeenNthCalledWith(
      3,
      '/admin/rewards/codes/7',
      expect.objectContaining({ method: 'DELETE' }),
    )
  })

  it('omits Code from a rule-only update', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { reward_code: rewardCode },
    })

    await createAdminRewardCodeResource().update(7, {
      displayName: '新用户奖励',
      creditAmount: 80,
      maxRedemptions: 100,
      expiresAt: '',
    })

    expect(transport.request).toHaveBeenCalledWith(
      '/admin/rewards/codes/7',
      expect.objectContaining({
        method: 'PATCH',
        body: {
          display_name: '新用户奖励',
          credit_amount: 80,
          max_redemptions: 100,
          expires_at: null,
        },
      }),
    )
  })
})
