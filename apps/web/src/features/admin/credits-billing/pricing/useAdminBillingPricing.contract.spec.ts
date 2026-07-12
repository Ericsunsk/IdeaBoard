import { describe, expect, it, vi } from 'vitest'
import type { AdminBillingPricingResource } from './billingPricingResource'
import type { AdminBillingCatalogItem, AdminBillingPrice } from './billingPricingTypes'
import { useAdminBillingPricing } from './useAdminBillingPricing'

describe('useAdminBillingPricing', () => {
  it('loads and refreshes the server-owned AI Workflow coverage matrix with observable errors', async () => {
    const resource = createResource()
    const coverage = [
      {
        feature: 'sketch-to-image' as const,
        model: 'gpt-image-1',
        sku: 'ai-image:sketch-to-image.abc123',
        skuConfigured: false,
        skuEnabled: false,
        priceConfigured: false,
        priceEnabled: false,
      },
    ]
    resource.listCoverage.mockResolvedValueOnce(coverage)
    const feedback = { success: vi.fn(), error: vi.fn() }
    const pricing = useAdminBillingPricing({ resource, feedback })

    await pricing.loadCoverage()
    expect(pricing.coverageItems.value).toEqual(coverage)
    expect(pricing.coverageLoading.value).toBe(false)
    expect(pricing.coverageError.value).toBe('')

    resource.listCoverage.mockRejectedValueOnce(new Error('coverage unavailable'))
    await pricing.loadCoverage()
    expect(pricing.coverageItems.value).toEqual(coverage)
    expect(pricing.coverageError.value).toBe('AI Workflow 计费覆盖加载失败')
    expect(feedback.error).toHaveBeenCalledWith(expect.any(Error), 'AI Workflow 计费覆盖加载失败')
  })

  it('preserves Catalog and price keyset cursors while keeping mutation refresh failures observable', async () => {
    const pageOne = [makeItem('test:a'), makeItem('test:b')]
    const pageTwo = [makeItem('test:c')]
    const resource = createResource()
    resource.listSKUs.mockImplementation(async ({ afterSku } = {}) =>
      afterSku ? { items: pageTwo } : { items: pageOne, nextAfterSku: 'test:b' },
    )
    resource.listPrices.mockImplementation(async (_sku, { beforeVersion } = {}) =>
      beforeVersion ? { prices: [makePrice(1)] } : { prices: [makePrice(2)], nextBeforeVersion: 2 },
    )
    const feedback = { success: vi.fn(), error: vi.fn() }
    const pricing = useAdminBillingPricing({ resource, feedback })

    await pricing.loadCatalog()
    expect(pricing.selectedSKU.value?.sku).toBe('test:a')
    expect(pricing.currentCatalogPage.value).toBe(1)

    await pricing.nextCatalogPage()
    expect(resource.listSKUs).toHaveBeenLastCalledWith({ afterSku: 'test:b', limit: 24 })
    expect(pricing.catalogItems.value).toEqual(pageTwo)
    expect(pricing.currentCatalogPage.value).toBe(2)

    await pricing.previousCatalogPage()
    expect(resource.listSKUs).toHaveBeenLastCalledWith({ afterSku: undefined, limit: 24 })
    expect(pricing.currentCatalogPage.value).toBe(1)

    await pricing.nextPricePage()
    expect(resource.listPrices).toHaveBeenLastCalledWith('test:a', {
      beforeVersion: 2,
      limit: 20,
    })
    expect(pricing.prices.value).toEqual([makePrice(1)])

    resource.createSKU.mockResolvedValueOnce(makeItem('test:z'))
    resource.listSKUs.mockRejectedValueOnce(new Error('refresh unavailable'))
    await expect(
      pricing.createSKU({
        sku: 'test:z',
        displayName: 'test:z',
        description: '',
        enabled: true,
        reason: 'Create test SKU',
      }),
    ).resolves.toBe(true)

    expect(pricing.selectedSKU.value?.sku).toBe('test:z')
    expect(feedback.success).toHaveBeenCalledWith('SKU 已创建；该标识将永久占用且不可复用')
    expect(feedback.error).toHaveBeenCalledWith(expect.any(Error), 'SKU Catalog 加载失败')
  })
})

function createResource() {
  return {
    listCoverage: vi.fn<AdminBillingPricingResource['listCoverage']>(),
    listSKUs: vi.fn<AdminBillingPricingResource['listSKUs']>(),
    createSKU: vi.fn<AdminBillingPricingResource['createSKU']>(),
    updateSKU: vi.fn<AdminBillingPricingResource['updateSKU']>(),
    listPrices: vi.fn<AdminBillingPricingResource['listPrices']>(),
    createPrice: vi.fn<AdminBillingPricingResource['createPrice']>(),
    setPriceEnabled: vi.fn<AdminBillingPricingResource['setPriceEnabled']>(),
  }
}

function makeItem(sku: string): AdminBillingCatalogItem {
  return {
    sku,
    displayName: sku,
    description: '',
    enabled: true,
    createdAt: '2026-07-12T08:00:00Z',
    updatedAt: '2026-07-12T08:00:00Z',
  }
}

function makePrice(version: number): AdminBillingPrice {
  return {
    priceVersionId: version,
    version,
    unitCredits: version * 10,
    enabled: true,
    createdBy: 1,
    createdAt: '2026-07-12T08:00:00Z',
    updatedBy: 1,
    updatedAt: '2026-07-12T08:00:00Z',
  }
}
