import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import {
  createAdminBillableCatalogResource,
  publishBillableItemPrice,
  setBillableItemAvailability,
} from './billableCatalogResource'

describe('Admin Billable Catalog resource', () => {
  it('owns generated API normalization for the pricing feature', async () => {
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            items: [
              {
                sku: 'ai-image:partial-edit',
                namespace: 'ai-image',
                product_kind: 'workflow-feature',
                display_name: '局部重绘',
                description: '仅重绘图片选定区域。',
                sort_order: 30,
                status: 'unpriced',
              },
            ],
          },
        }),
      ),
    )

    await expect(createAdminBillableCatalogResource().list()).resolves.toEqual([
      {
        sku: 'ai-image:partial-edit',
        namespace: 'ai-image',
        productKind: 'workflow-feature',
        displayName: '局部重绘',
        description: '仅重绘图片选定区域。',
        sortOrder: 30,
        status: 'unpriced',
        currentPrice: undefined,
        updatedAt: undefined,
      },
    ])
  })

  it('publishes a feature price without legacy state or reason fields', async () => {
    let requestBody: unknown
    useMswHandlers(
      http.post('/api/admin/billing/catalog/:sku/prices', async ({ request, params }) => {
        requestBody = await request.json()
        expect(params.sku).toBe('ai-image:partial-edit')
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            price_version_id: 9,
            version: 1,
            unit_credits: 0,
            created_by: 7,
            created_at: '2026-07-16T08:00:00Z',
          },
        })
      }),
    )

    await expect(publishBillableItemPrice('ai-image:partial-edit', 0)).resolves.toEqual({
      priceVersionId: 9,
      version: 1,
      unitCredits: 0,
      createdBy: 7,
      createdAt: '2026-07-16T08:00:00Z',
    })
    expect(requestBody).toEqual({ unit_credits: 0 })
  })

  it('sets feature availability without a manual reason', async () => {
    let requestBody: unknown
    useMswHandlers(
      http.put('/api/admin/billing/catalog/:sku/availability', async ({ request, params }) => {
        requestBody = await request.json()
        expect(params.sku).toBe('ai-image:partial-edit')
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            sku: 'ai-image:partial-edit',
            status: 'disabled',
            updated_at: '2026-07-16T10:00:00Z',
          },
        })
      }),
    )

    await expect(setBillableItemAvailability('ai-image:partial-edit', false)).resolves.toEqual({
      sku: 'ai-image:partial-edit',
      status: 'disabled',
      updatedAt: '2026-07-16T10:00:00Z',
    })
    expect(requestBody).toEqual({ enabled: false })
  })
})
