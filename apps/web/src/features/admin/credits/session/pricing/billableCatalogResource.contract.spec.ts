import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import {
  createAdminBillableCatalogResource,
  deleteArchivedBillableItem,
  publishBillableItemPrices,
  setBillableItemAvailability,
} from './billableCatalogResource'
import { createBillingCatalogHandlers, createBillingCatalogMswState } from './testing'

const options = {}

describe('Admin Billable Catalog resource', () => {
  it('lists the catalog through the control plane with pricing normalization', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(
        createBillingCatalogMswState({
          items: [
            {
              sku: 'ai-image:inpaint',
              namespace: 'ai-image',
              product_kind: 'workflow-feature',
              display_name: '局部重绘',
              sort_order: 30,
              status: 'unpriced',
              pricing_variants: ['1k'],
            },
          ],
          wrapItems: (items) => ({ data: { items } }),
        }),
      ),
    )

    await expect(createAdminBillableCatalogResource().list()).resolves.toEqual([
      {
        sku: 'ai-image:inpaint',
        namespace: 'ai-image',
        productKind: 'workflow-feature',
        displayName: '局部重绘',
        sortOrder: 30,
        status: 'unpriced',
        pricingVariants: ['1k'],
        currentPrices: {},
        updatedAt: undefined,
      },
    ])
  })

  it('publishes the configured fixed price with idempotency headers', async () => {
    let requestBody: unknown
    let seenHeaders: Record<string, string | null> = {}
    useMswHandlers(
      http.post('/api/control/v1/billing/catalog/:sku/prices', async ({ request, params }) => {
        requestBody = await request.json()
        seenHeaders = {
          idempotencyKey: request.headers.get('Idempotency-Key'),
        }
        expect(params.sku).toBe('ai-image:inpaint')
        return HttpResponse.json({
          data: {
            prices: {
              '1k': {
                output_resolution: '1k',
                price_version_id: 9,
                version: 1,
                unit_credits: 0,
                created_at: '2026-07-16T08:00:00Z',
              },
            },
          },
        })
      }),
    )

    await expect(
      publishBillableItemPrices('ai-image:inpaint', { '1k': 0 }, options),
    ).resolves.toEqual({
      '1k': {
        priceVersionId: 9,
        version: 1,
        unitCredits: 0,
        createdAt: '2026-07-16T08:00:00Z',
      },
    })
    expect(requestBody).toEqual({ prices: { '1k': 0 } })
    expect(seenHeaders.idempotencyKey).toMatch(/.+/)
  })

  it('sets feature availability with an idempotency key', async () => {
    let requestBody: unknown
    let seenHeaders: Record<string, string | null> = {}
    useMswHandlers(
      http.put('/api/control/v1/billing/catalog/:sku/availability', async ({ request, params }) => {
        requestBody = await request.json()
        expect(params.sku).toBe('ai-image:inpaint')
        seenHeaders = {
          idempotencyKey: request.headers.get('Idempotency-Key'),
        }
        return HttpResponse.json({
          data: {
            sku: 'ai-image:inpaint',
            status: 'disabled',
            updated_at: '2026-07-16T10:00:00Z',
          },
        })
      }),
    )

    await expect(setBillableItemAvailability('ai-image:inpaint', false)).resolves.toEqual({
      sku: 'ai-image:inpaint',
      status: 'disabled',
      updatedAt: '2026-07-16T10:00:00Z',
    })
    expect(requestBody).toEqual({ enabled: false })
    expect(seenHeaders.idempotencyKey).toMatch(/.+/)
  })

  it('deletes an Archived item through the control plane', async () => {
    useMswHandlers(
      http.delete('/api/control/v1/billing/catalog/:sku', ({ params }) =>
        HttpResponse.json({ data: { sku: params.sku } }),
      ),
    )

    await expect(deleteArchivedBillableItem('legacy:removed', options)).resolves.toBe(
      'legacy:removed',
    )
  })
})
