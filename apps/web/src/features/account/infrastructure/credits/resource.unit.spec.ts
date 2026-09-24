import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchCreditWallet } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

describe('creditsResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads only the authenticated user wallet endpoint', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      data: { user_id: 7, status: 'active', available_credits: 128, reserved_credits: 4 },
    })

    await expect(fetchCreditWallet()).resolves.toMatchObject({
      user_id: 7,
      available_credits: 128,
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/user/credits/wallet',
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('surfaces API feedback instead of returning an empty wallet', async () => {
    transport.request.mockResolvedValue({
      code: 404,
      msg: 'Idea钱包不存在',
      message_safe: true,
    })

    await expect(fetchCreditWallet()).rejects.toThrow('Idea钱包不存在')
  })

  it('uses the resource fallback when API feedback contains sensitive details', async () => {
    transport.request.mockResolvedValue({ code: 500, msg: 'api_key=exposed' })

    await expect(fetchCreditWallet()).rejects.toThrow('Could not load your Ideas balance.')
  })
})
