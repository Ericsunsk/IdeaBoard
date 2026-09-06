import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchCreditWallet } from './creditsResource'

const generated = vi.hoisted(() => ({
  getUserCreditWallet: vi.fn(),
}))

vi.mock('../../../../shared/api/generated-query/user-credits/user-credits', () => generated)

describe('creditsResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads only the authenticated user wallet endpoint', async () => {
    generated.getUserCreditWallet.mockResolvedValue({
      code: 0,
      data: { user_id: 7, status: 'active', available_credits: 128, reserved_credits: 4 },
    })

    await expect(fetchCreditWallet()).resolves.toMatchObject({
      user_id: 7,
      available_credits: 128,
    })
    expect(generated.getUserCreditWallet).toHaveBeenCalledWith()
  })

  it('surfaces API feedback instead of returning an empty wallet', async () => {
    generated.getUserCreditWallet.mockResolvedValue({
      code: 404,
      msg: 'Idea钱包不存在',
      message_safe: true,
    })

    await expect(fetchCreditWallet()).rejects.toThrow('Idea钱包不存在')
  })

  it('uses the resource fallback when API feedback contains sensitive details', async () => {
    generated.getUserCreditWallet.mockResolvedValue({ code: 500, msg: 'api_key=exposed' })

    await expect(fetchCreditWallet()).rejects.toThrow('Could not load your Ideas balance.')
  })
})
