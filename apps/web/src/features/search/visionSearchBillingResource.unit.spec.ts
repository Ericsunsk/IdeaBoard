import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchVisionSearchPrice } from './visionSearchBillingResource'

const generated = vi.hoisted(() => ({ quote: vi.fn() }))

vi.mock('../../shared/api/generated-query/billing/billing', () => ({
  createBillingQuote: generated.quote,
}))

describe('visionSearchBillingResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads the configured default price for one vision search', async () => {
    generated.quote.mockResolvedValue({ code: 0, data: { total_credits: 12 } })
    const controller = new AbortController()

    await expect(fetchVisionSearchPrice(controller.signal)).resolves.toBe(12)
    expect(generated.quote).toHaveBeenCalledWith(
      { sku: 'ai-search:vision', output_resolution: 'default', quantity: 1 },
      { signal: controller.signal },
    )
  })
})
