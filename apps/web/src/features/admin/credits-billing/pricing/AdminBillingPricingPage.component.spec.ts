import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import AdminBillingPricingPage from './AdminBillingPricingPage.vue'

const confirmRequestMock = vi.hoisted(() => vi.fn())
const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ request: confirmRequestMock }),
}))

vi.mock('../../shared/lib/adminToast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/adminToast')>()),
  useAdminToast: () => toast,
}))

describe('AdminBillingPricingPage', () => {
  beforeEach(() => {
    confirmRequestMock.mockReset()
    toast.success.mockReset()
    toast.warning.mockReset()
    toast.error.mockReset()
  })
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

  it('publishes an explicit zero price from the card dialog without confirmation', async () => {
    let publishRequests = 0
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.post('/api/admin/billing/catalog/:sku/prices', async ({ request, params }) => {
        publishRequests += 1
        expect(params.sku).toBe('ai-image:partial-edit')
        await expect(request.json()).resolves.toEqual({ unit_credits: 0 })
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            price_version_id: 8,
            version: 1,
            unit_credits: 0,
            created_at: '2026-07-16T09:00:00Z',
          },
        })
      }),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '设置价格' }))
    expect(screen.getByText('创建新版本，历史版本不变')).toBeVisible()
    expect(screen.getByText('待输入')).toBeVisible()
    const priceInput = screen.getByRole('spinbutton', { name: '价格' })
    await user.type(priceInput, '0')
    expect(priceInput).toHaveValue('0 Ideas')
    await user.click(screen.getByRole('button', { name: '发布价格' }))

    expect(await screen.findByText('0 Ideas')).toBeVisible()
    expect(screen.getAllByText('生效中')).toHaveLength(2)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(publishRequests).toBe(1)
  })

  it('shows the current, next, delta, and immutable version impact', async () => {
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
    )

    const { user } = renderPricingPage()
    await user.click((await screen.findAllByRole('button', { name: '调整价格' }))[0])

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText('当前价格')).toBeVisible()
    expect(within(dialog).getByText('新价格')).toBeVisible()
    expect(within(dialog).getByText('差值')).toBeVisible()
    expect(within(dialog).getAllByText('1 Idea')).toHaveLength(2)
    expect(within(dialog).getByText('0 Ideas')).toBeVisible()
    expect(within(dialog).getByText('创建新版本，历史版本不变')).toBeVisible()
  })

  it('disables and re-enables a priced feature from its card', async () => {
    const requests: boolean[] = []
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.put('/api/admin/billing/catalog/:sku/availability', async ({ request, params }) => {
        expect(params.sku).toBe('ai-image:remove-bg')
        const body = (await request.json()) as { enabled: boolean }
        requests.push(body.enabled)
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            sku: params.sku,
            status: body.enabled ? 'active' : 'disabled',
            updated_at: '2026-07-16T10:00:00Z',
          },
        })
      }),
    )

    const { user } = renderPricingPage()
    const switchControl = await screen.findByRole('switch', { name: '抠图功能状态' })
    expect(switchControl).toBeChecked()
    await user.click(switchControl)
    await waitFor(() => expect(switchControl).not.toBeChecked())
    await user.click(switchControl)
    await waitFor(() => expect(switchControl).toBeChecked())
    expect(requests).toEqual([false, true])

    expect(screen.getByRole('switch', { name: '局部重绘功能状态' })).toBeDisabled()
  })

  it('keeps publish failures in the dialog and allows a single retry', async () => {
    let requests = 0
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.post('/api/admin/billing/catalog/:sku/prices', () => {
        requests += 1
        if (requests === 1) {
          return HttpResponse.json(
            { code: 500, msg: 'publish unavailable', message_safe: true },
            { status: 500 },
          )
        }
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            price_version_id: 9,
            version: 1,
            unit_credits: 3,
            created_at: '2026-07-16T09:00:00Z',
          },
        })
      }),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '设置价格' }))
    await user.type(screen.getByRole('spinbutton', { name: '价格' }), '3')
    await user.click(screen.getByRole('button', { name: '发布价格' }))

    expect((await screen.findAllByText('publish unavailable')).length).toBeGreaterThan(0)
    expect(screen.getByRole('dialog')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '发布价格' }))
    expect(await screen.findByText('3 Ideas')).toBeVisible()
    expect(requests).toBe(2)
  })

  it('validates the price and blocks duplicate publishes while pending', async () => {
    let publishRequests = 0
    let releasePublish!: () => void
    const publishGate = new Promise<void>((resolve) => {
      releasePublish = resolve
    })
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.post('/api/admin/billing/catalog/:sku/prices', async () => {
        publishRequests += 1
        await publishGate
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            price_version_id: 10,
            version: 1,
            unit_credits: 5,
            created_at: '2026-07-16T09:00:00Z',
          },
        })
      }),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '设置价格' }))
    const publishButton = screen.getByRole('button', { name: '发布价格' })
    await user.click(publishButton)
    expect(screen.getByText('请输入价格')).toBeVisible()
    expect(publishRequests).toBe(0)

    await user.type(screen.getByRole('spinbutton', { name: /价格/ }), '5')
    await user.click(publishButton)
    expect(publishButton).toBeDisabled()
    await user.click(publishButton)
    expect(publishRequests).toBe(1)

    releasePublish()
    expect(await screen.findByText('5 Ideas')).toBeVisible()
  })

  it('confirms and removes an unreferenced Archived card', async () => {
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.delete('/api/admin/billing/catalog/:sku', ({ params }) =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { sku: params.sku } }),
      ),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '删除' }))
    expect(confirmRequestMock).toHaveBeenCalledOnce()
    const request = confirmRequestMock.mock.calls[0]?.[0] as { onConfirm: () => Promise<void> }
    await request.onConfirm()

    expect(screen.queryByText('已移除功能')).not.toBeInTheDocument()
    expect(toast.success).toHaveBeenCalledWith('已归档功能已删除')
  })

  it('keeps the Archived card when durable history blocks deletion', async () => {
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.delete('/api/admin/billing/catalog/:sku', () =>
        HttpResponse.json(
          { code: 409, msg: '功能已有历史计费记录，无法删除', message_safe: true },
          { status: 409 },
        ),
      ),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '删除' }))
    const request = confirmRequestMock.mock.calls[0]?.[0] as { onConfirm: () => Promise<void> }
    await request.onConfirm()

    expect(screen.getByText('已移除功能')).toBeVisible()
    expect(toast.error).toHaveBeenCalledWith('功能已有历史计费记录，无法删除')
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
    created_at: '2026-07-12T08:00:00Z',
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
