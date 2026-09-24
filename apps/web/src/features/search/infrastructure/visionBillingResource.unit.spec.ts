import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchVisionSearchPrice } from './visionBillingResource'

const transport = vi.hoisted(() => ({ request: vi.fn() }))

vi.mock('../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

describe('visionBillingResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads the configured default price for one vision search', async () => {
    transport.request.mockResolvedValue({ code: 0, data: { total_credits: 12 } })
    const controller = new AbortController()

    await expect(fetchVisionSearchPrice(controller.signal)).resolves.toBe(12)
    expect(transport.request).toHaveBeenCalledWith(
      '/user/billing/quotes',
      expect.objectContaining({
        method: 'POST',
        body: { sku: 'ai-search:vision', output_resolution: 'default', quantity: 1 },
        signal: controller.signal,
      }),
    )
  })
})
