import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen } from '../../../../test/render'
import AdminBillingPricingPage from './AdminBillingPricingPage.vue'

describe('AdminBillingPricingPage', () => {
  it('preserves the grid surface while the Catalog is loading', () => {
    useMswHandlers(
      http.get('/api/admin/billing/catalog', async () => {
        await new Promise(() => {})
        return HttpResponse.json({ code: 0, msg: 'ok', data: { items: [] } })
      }),
    )

    renderPricingPage()

    expect(screen.getByRole('progressbar')).toBeVisible()
  })

  it('renders the four product-oriented Catalog states in the shared Masonry grid', async () => {
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
    )

    const { container } = renderPricingPage()

    expect(await screen.findByText('局部重绘')).toBeVisible()
    expect(screen.getByText('抠图')).toBeVisible()
    expect(screen.getByText('HD')).toBeVisible()
    expect(screen.getByText('已移除功能')).toBeVisible()
    expect(screen.getByText('未定价')).toBeVisible()
    expect(screen.getByText('生效中')).toBeVisible()
    expect(screen.getByText('已停用')).toBeVisible()
    expect(screen.getByText('已归档')).toBeVisible()
    expect(screen.getAllByText('1 Idea')).toHaveLength(3)
    expect(screen.queryByText('1 Ideas')).not.toBeInTheDocument()
    expect(container.querySelector('.columns-1.md\\:columns-2.xl\\:columns-3')).toBeTruthy()
    expect(screen.queryByRole('button', { name: '新建 SKU' })).not.toBeInTheDocument()
  })

  it('shows an empty state when no products are registered', async () => {
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: [] } }),
      ),
    )

    renderPricingPage()

    expect(await screen.findByText('暂无可定价功能')).toBeVisible()
  })

  it('keeps load failures visible and retryable', async () => {
    let requests = 0
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () => {
        requests += 1
        if (requests === 1) {
          return HttpResponse.json({ code: 500, msg: 'catalog unavailable' }, { status: 500 })
        }
        return HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } })
      }),
    )

    const { user } = renderPricingPage()

    expect(await screen.findByText('功能定价加载失败')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重新加载' }))
    expect(await screen.findByText('局部重绘')).toBeVisible()
  })
})

function renderPricingPage() {
  return renderWithUser(AdminBillingPricingPage, {
    global: {
      plugins: [[PrimeVue, primeVueTestOptions]],
      stubs: { Refresh: true },
    },
  })
}

function makeCatalogItems() {
  const price = {
    price_version_id: 1,
    version: 1,
    unit_credits: 1,
    enabled: true,
    created_by: 7,
    created_at: '2026-07-12T08:00:00Z',
    updated_by: 7,
    updated_at: '2026-07-12T08:00:00Z',
  }
  return [
    {
      sku: 'ai-image:partial-edit',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '局部重绘',
      description: '仅重绘图片选定区域。',
      sort_order: 10,
      status: 'unpriced',
    },
    {
      sku: 'ai-image:remove-bg',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '抠图',
      description: '识别主体并移除图片背景。',
      sort_order: 20,
      status: 'active',
      current_price: price,
    },
    {
      sku: 'ai-image:hd-upscale',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: 'HD',
      description: '提升图片分辨率与细节。',
      sort_order: 30,
      status: 'disabled',
      current_price: price,
    },
    {
      sku: 'legacy:removed',
      namespace: 'legacy',
      product_kind: 'legacy',
      display_name: '已移除功能',
      description: '已从 Registry 移除。',
      sort_order: 0,
      status: 'archived',
      current_price: price,
    },
  ]
}
