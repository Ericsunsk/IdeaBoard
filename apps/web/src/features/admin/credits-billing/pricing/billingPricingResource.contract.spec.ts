import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { createAdminBillingPricingResource } from './billingPricingResource'

describe('admin billing pricing resource', () => {
  it('keeps generated Billing API calls behind the feature resource boundary', async () => {
    const requests: Array<{ method: string; path: string; search: string; body?: unknown }> = []
    useMswHandlers(
      http.get('/api/admin/billing/coverage', ({ request }) => {
        recordRequest(requests, request)
        return ok({
          items: [
            {
              feature: 'sketch-to-image',
              model: 'gpt-image-1',
              sku: 'ai-image:sketch-to-image.abc123',
              sku_configured: true,
              sku_enabled: true,
              price_configured: true,
              price_enabled: true,
              unit_credits: 25,
            },
          ],
        })
      }),
      http.get('/api/admin/billing/skus', ({ request }) => {
        recordRequest(requests, request)
        return ok({ items: [makeCatalogItem()], next_after_sku: 'test:next' })
      }),
      http.post('/api/admin/billing/skus', async ({ request }) => {
        recordRequest(requests, request, await readRequestJson(request))
        return ok(makeCatalogItem())
      }),
      http.patch('/api/admin/billing/skus/:sku', async ({ request }) => {
        recordRequest(requests, request, await readRequestJson(request))
        return ok(makeCatalogItem({ display_name: 'Edited' }))
      }),
      http.get('/api/admin/billing/skus/:sku/prices', ({ request }) => {
        recordRequest(requests, request)
        return ok({ prices: [makePrice()], next_before_version: 1 })
      }),
      http.post('/api/admin/billing/skus/:sku/prices', async ({ request }) => {
        recordRequest(requests, request, await readRequestJson(request))
        return ok(makePrice({ version: 3, unit_credits: 35 }))
      }),
      http.patch('/api/admin/billing/skus/:sku/prices/:version', async ({ request }) => {
        recordRequest(requests, request, await readRequestJson(request))
        return ok(makePrice({ enabled: false }))
      }),
    )

    const resource = createAdminBillingPricingResource()

    await expect(resource.listCoverage()).resolves.toEqual([
      {
        feature: 'sketch-to-image',
        model: 'gpt-image-1',
        sku: 'ai-image:sketch-to-image.abc123',
        skuConfigured: true,
        skuEnabled: true,
        priceConfigured: true,
        priceEnabled: true,
        unitCredits: 25,
      },
    ])
    await expect(resource.listSKUs({ afterSku: 'test:after', limit: 24 })).resolves.toMatchObject({
      items: [{ sku: 'test:item', displayName: 'Test item' }],
      nextAfterSku: 'test:next',
    })
    await expect(
      resource.createSKU({
        sku: ' Test:Item ',
        displayName: ' Test item ',
        description: ' Description ',
        enabled: true,
        reason: ' Create for launch ',
      }),
    ).resolves.toMatchObject({ sku: 'test:item' })
    await expect(
      resource.updateSKU('test:item', {
        displayName: ' Edited ',
        description: ' Description ',
        enabled: false,
        reason: ' Disable retired SKU ',
      }),
    ).resolves.toMatchObject({ displayName: 'Edited' })
    await expect(
      resource.listPrices('test:item', { beforeVersion: 2, limit: 20 }),
    ).resolves.toMatchObject({ prices: [{ version: 2, unitCredits: 25 }], nextBeforeVersion: 1 })
    await expect(
      resource.createPrice('test:item', {
        unitCredits: 35,
        enabled: true,
        reason: ' Publish July pricing ',
      }),
    ).resolves.toMatchObject({ version: 3, unitCredits: 35 })
    await expect(
      resource.setPriceEnabled('test:item', 2, {
        enabled: false,
        reason: ' Pause incorrect price ',
      }),
    ).resolves.toMatchObject({ version: 2, enabled: false })

    expect(requests).toEqual([
      {
        method: 'GET',
        path: '/api/admin/billing/coverage',
        search: '',
      },
      {
        method: 'GET',
        path: '/api/admin/billing/skus',
        search: '?after_sku=test%3Aafter&limit=24',
      },
      {
        method: 'POST',
        path: '/api/admin/billing/skus',
        search: '',
        body: {
          sku: 'test:item',
          display_name: 'Test item',
          description: 'Description',
          enabled: true,
          reason: 'Create for launch',
        },
      },
      {
        method: 'PATCH',
        path: '/api/admin/billing/skus/test%3Aitem',
        search: '',
        body: {
          display_name: 'Edited',
          description: 'Description',
          enabled: false,
          reason: 'Disable retired SKU',
        },
      },
      {
        method: 'GET',
        path: '/api/admin/billing/skus/test%3Aitem/prices',
        search: '?before_version=2&limit=20',
      },
      {
        method: 'POST',
        path: '/api/admin/billing/skus/test%3Aitem/prices',
        search: '',
        body: { unit_credits: 35, enabled: true, reason: 'Publish July pricing' },
      },
      {
        method: 'PATCH',
        path: '/api/admin/billing/skus/test%3Aitem/prices/2',
        search: '',
        body: { enabled: false, reason: 'Pause incorrect price' },
      },
    ])
  })
})

function ok(data: unknown) {
  return HttpResponse.json({ code: 0, msg: 'ok', data })
}

function makeCatalogItem(overrides: Record<string, unknown> = {}) {
  return {
    sku: 'test:item',
    display_name: 'Test item',
    description: 'Description',
    enabled: true,
    current_price_version_id: 10,
    current_price: makePrice(),
    created_at: '2026-07-12T08:00:00Z',
    updated_at: '2026-07-12T08:00:00Z',
    ...overrides,
  }
}

function makePrice(overrides: Record<string, unknown> = {}) {
  return {
    price_version_id: 10,
    version: 2,
    unit_credits: 25,
    enabled: true,
    created_by: 7,
    created_at: '2026-07-12T08:00:00Z',
    updated_by: 7,
    updated_at: '2026-07-12T08:00:00Z',
    ...overrides,
  }
}

function recordRequest(
  requests: Array<{ method: string; path: string; search: string; body?: unknown }>,
  request: Request,
  body?: unknown,
) {
  const url = new URL(request.url)
  requests.push({
    method: request.method,
    path: url.pathname,
    search: url.search,
    ...(body === undefined ? {} : { body }),
  })
}

async function readRequestJson(request: Request) {
  const text = await request.text()
  return text ? JSON.parse(text) : undefined
}
