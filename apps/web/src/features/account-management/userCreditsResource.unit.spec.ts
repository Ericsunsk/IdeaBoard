import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchUserCreditLedger, fetchUserCreditWallet } from './userCreditsResource'

const generated = vi.hoisted(() => ({
  getUserCreditWallet: vi.fn(),
  listUserCreditLedger: vi.fn(),
}))

vi.mock('../../shared/api/generated-query/user-credits/user-credits', () => generated)

describe('userCreditsResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads only the authenticated user wallet endpoint', async () => {
    generated.getUserCreditWallet.mockResolvedValue({
      code: 0,
      data: { user_id: 7, status: 'active', available_credits: 128, reserved_credits: 4 },
    })

    await expect(fetchUserCreditWallet()).resolves.toMatchObject({
      user_id: 7,
      available_credits: 128,
    })
    expect(generated.getUserCreditWallet).toHaveBeenCalledWith()
  })

  it('forwards keyset pagination without accepting a user id', async () => {
    generated.listUserCreditLedger.mockResolvedValue({
      code: 0,
      data: { transactions: [], next_before_sequence: 8 },
    })

    await fetchUserCreditLedger({ before_sequence: 16, limit: 20 })

    expect(generated.listUserCreditLedger).toHaveBeenCalledWith({
      before_sequence: 16,
      limit: 20,
    })
  })

  it('surfaces API feedback instead of returning an empty wallet', async () => {
    generated.getUserCreditWallet.mockResolvedValue({ code: 404, msg: '点子钱包不存在' })

    await expect(fetchUserCreditWallet()).rejects.toThrow('点子钱包不存在')
  })
})
