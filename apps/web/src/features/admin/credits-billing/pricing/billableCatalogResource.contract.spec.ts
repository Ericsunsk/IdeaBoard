import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { createAdminBillableCatalogResource } from './billableCatalogResource'

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
})
