import { describe, expect, it } from 'vitest'
import {
  filterBillingCatalog,
  isCatalogItemBillable,
  parseAIWorkflowSKU,
  validateBillingPriceForm,
  validateBillingSKUForm,
} from './billingPricingModel'
import type { AdminBillingCatalogItem } from './billingPricingTypes'

describe('billing pricing model', () => {
  it('recognizes only real canonical AI workflow SKU identities', () => {
    const parsed = parseAIWorkflowSKU(
      'ai-image:sketch-to-image.77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18',
    )

    expect(parsed).toMatchObject({
      feature: { slug: 'sketch-to-image', label: '草图成图' },
      productFingerprint: '77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18',
    })
    expect(
      parseAIWorkflowSKU(
        'ai-image:unknown.77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18',
      ),
    ).toBeUndefined()
    expect(parseAIWorkflowSKU('ai-image:sketch-to-image.fake-model')).toBeUndefined()
  })

  it('filters the server page without inventing missing workflow or model combinations', () => {
    const aiSKU = makeItem({
      sku: 'ai-image:pattern-gen.348fdd008de3164eb4a4992efcac81356355e27910dcaaa2472cb048241bd9b6',
      displayName: '图案生成',
    })
    const unpriced = makeItem({
      sku: 'test:unpriced',
      displayName: 'No price',
      currentPrice: undefined,
    })

    expect(filterBillingCatalog([aiSKU, unpriced], '', 'ai-image')).toEqual([aiSKU])
    expect(filterBillingCatalog([aiSKU, unpriced], '', 'unpriced')).toEqual([unpriced])
    expect(filterBillingCatalog([aiSKU, unpriced], 'NO PRICE', 'all')).toEqual([unpriced])
  })

  it('distinguishes configured prices from actually billable state', () => {
    expect(isCatalogItemBillable(makeItem())).toBe(true)
    expect(isCatalogItemBillable(makeItem({ enabled: false }))).toBe(false)
    expect(
      isCatalogItemBillable(
        makeItem({ currentPrice: { ...makeItem().currentPrice!, enabled: false } }),
      ),
    ).toBe(false)
  })

  it('validates immutable SKU syntax and safe integer Credit prices', () => {
    expect(
      validateBillingSKUForm(
        {
          sku: 'INVALID SKU',
          displayName: '',
          description: 'x'.repeat(1001),
          enabled: true,
          reason: '',
        },
        false,
      ),
    ).toEqual({
      sku: expect.any(String),
      displayName: expect.any(String),
      description: expect.any(String),
      reason: expect.any(String),
    })
    expect(
      validateBillingSKUForm(
        {
          sku: 'ignored while editing',
          displayName: 'Price',
          description: '',
          enabled: true,
          reason: '  调整展示文案  ',
        },
        true,
      ),
    ).toEqual({})
    expect(
      validateBillingPriceForm({ unitCredits: 0, enabled: true, reason: '发布免费价格' }),
    ).toEqual({})
    expect(validateBillingPriceForm({ unitCredits: 1.5, enabled: true, reason: '' })).toMatchObject({
      reason: expect.any(String),
      unitCredits: expect.any(String),
    })
  })
})

function makeItem(overrides: Partial<AdminBillingCatalogItem> = {}): AdminBillingCatalogItem {
  return {
    sku: 'test:item',
    displayName: 'Test item',
    description: '',
    enabled: true,
    currentPriceVersionId: 10,
    currentPrice: {
      priceVersionId: 10,
      version: 1,
      unitCredits: 25,
      enabled: true,
      createdBy: 1,
      createdAt: '2026-07-12T08:00:00Z',
      updatedBy: 1,
      updatedAt: '2026-07-12T08:00:00Z',
    },
    createdAt: '2026-07-12T08:00:00Z',
    updatedAt: '2026-07-12T08:00:00Z',
    ...overrides,
  }
}
