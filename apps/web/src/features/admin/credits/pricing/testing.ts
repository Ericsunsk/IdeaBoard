import type { HttpHandler } from 'msw'
import { http, HttpResponse } from '../../../../test/msw'

// Shared fixtures + MSW builder for the billable catalog surface
// (`/api/control/v1/billing/catalog`). The builder covers the duplicated GET
// collection (code/msg/data envelope) with hang + fail-first injection for the
// loading/retry tests; per-test price/availability/delete mutations keep their
// bespoke assertions in the specs.
export type BillingCatalogMswState = {
  items: unknown[]
  getItems: () => unknown[]
  wrapItems: (items: unknown[]) => Record<string, unknown>
  hang: boolean
  failFirst: number
  catalogRequests: number
}

export function createBillingCatalogMswState(
  overrides: Partial<BillingCatalogMswState> & { items?: unknown[] } = {},
): BillingCatalogMswState {
  const { items, ...rest } = overrides
  const state: BillingCatalogMswState = {
    items: items ?? makeCatalogItems(),
    getItems: () => state.items,
    wrapItems: (wrapped) => ({ code: 0, msg: 'ok', data: { items: wrapped } }),
    hang: false,
    failFirst: 0,
    catalogRequests: 0,
    ...rest,
  }
  return state
}

export function createBillingCatalogHandlers(state: BillingCatalogMswState): HttpHandler[] {
  return [
    http.get('/api/control/v1/billing/catalog', async () => {
      state.catalogRequests += 1
      if (state.hang) {
        await new Promise(() => {})
        return HttpResponse.json(state.wrapItems([]))
      }
      if (state.catalogRequests <= state.failFirst) {
        return HttpResponse.json({ code: 500, msg: 'catalog unavailable' }, { status: 500 })
      }
      return HttpResponse.json(state.wrapItems(state.getItems()))
    }),
  ]
}

export function makeCatalogItems() {
  const price = {
    output_resolution: '1k' as const,
    price_version_id: 1,
    version: 1,
    unit_credits: 1,
    created_at: '2026-07-12T08:00:00Z',
  }
  const legacyPrice = { ...price, output_resolution: '1k' as const }
  return [
    {
      sku: 'ai-image:inpaint',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '局部重绘',
      sort_order: 10,
      status: 'unpriced',
      pricing_variants: ['1k'],
    },
    {
      sku: 'ai-image:cutout',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '抠图',
      sort_order: 20,
      status: 'active',
      pricing_variants: ['1k'],
      current_prices: { '1k': price },
    },
    {
      sku: 'ai-image:upscale',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: 'Upscale',
      sort_order: 30,
      status: 'disabled',
      pricing_variants: ['1k'],
      current_prices: { '1k': price },
    },
    {
      sku: 'legacy:removed',
      namespace: 'legacy',
      product_kind: 'legacy',
      display_name: '已移除功能',
      sort_order: 0,
      status: 'archived',
      current_prices: { '1k': legacyPrice },
    },
  ]
}
