import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
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
    expect(screen.getAllByText('Image 节点')).toHaveLength(3)
    expect(screen.getByText('已移除功能')).toBeVisible()
    expect(screen.getByText('未定价')).toBeVisible()
    expect(screen.getByText('生效中')).toBeVisible()
    expect(screen.getByText('已停用')).toBeVisible()
    expect(screen.getByText('已归档')).toBeVisible()
    expect(screen.getAllByText('1 Idea')).toHaveLength(3)
    expect(screen.queryByText('1 Ideas')).not.toBeInTheDocument()
    expect(container.querySelector('.columns-1.md\\:columns-2.xl\\:columns-3')).toBeTruthy()
    expect(container.querySelector('.break-inside-avoid')).toBeTruthy()
    expect(container.querySelector('.grid.grid-cols-1.md\\:grid-cols-2.xl\\:grid-cols-3')).toBeNull()
    expect(screen.queryByRole('button', { name: '新建 SKU' })).not.toBeInTheDocument()
  })

  it('orders active priced cards before single-price and archived cards', async () => {
    const items = [
      ...makeCatalogItems(),
      {
        sku: 'ai-image:style-variation',
        namespace: 'ai-image',
        product_kind: 'workflow-feature',
        display_name: '风格延展',
        sort_order: 5,
        status: 'active',
        pricing_variants: ['1k', '2k', '4k'],
        current_prices: {
          '1k': {
            output_resolution: '1k',
            price_version_id: 2,
            version: 1,
            unit_credits: 2,
            created_at: '2026-07-12T08:00:00Z',
          },
        },
      },
    ]
    useMswHandlers(
      http.get('/api/admin/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items } }),
      ),
    )

    renderPricingPage()

    expect(await screen.findByText('风格延展')).toBeVisible()
    expectTextOrder(['风格延展', '抠图', '局部重绘', 'HD', '已移除功能'])
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
        await expect(request.json()).resolves.toEqual({ prices: { default: 0 } })
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            prices: {
              default: {
                output_resolution: 'default',
                price_version_id: 8,
                version: 1,
                unit_credits: 0,
                created_at: '2026-07-16T09:00:00Z',
              },
            },
          },
        })
      }),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '设置价格' }))
    expect(screen.getByText('创建新版本，历史版本不变')).toBeVisible()
    const priceInput = screen.getByRole('spinbutton', { name: '固定' })
    await user.type(priceInput, '0')
    expect(priceInput).toHaveValue('0 Ideas')
    await user.click(screen.getByRole('button', { name: '发布价格' }))

    expect(await screen.findByText('0 Ideas')).toBeVisible()
    expect(screen.getAllByText('生效中')).toHaveLength(2)
    expectTextOrder(['局部重绘', '抠图'])
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(publishRequests).toBe(1)
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
            prices: {
              default: {
                output_resolution: 'default',
                price_version_id: 9,
                version: 1,
                unit_credits: 3,
                created_at: '2026-07-16T09:00:00Z',
              },
            },
          },
        })
      }),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '设置价格' }))
    await user.type(screen.getByRole('spinbutton', { name: '固定' }), '3')
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
            prices: {
              default: {
                output_resolution: 'default',
                price_version_id: 10,
                version: 1,
                unit_credits: 5,
                created_at: '2026-07-16T09:00:00Z',
              },
            },
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

    await user.type(screen.getByRole('spinbutton', { name: /固定/ }), '5')
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

function expectTextOrder(labels: string[]) {
  const elements = labels.map((label) => screen.getByText(label))
  for (let index = 0; index < elements.length - 1; index += 1) {
    expect(
      elements[index]!.compareDocumentPosition(elements[index + 1]!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  }
}

function makeCatalogItems() {
  const price = {
    output_resolution: 'default' as const,
    price_version_id: 1,
    version: 1,
    unit_credits: 1,
    created_at: '2026-07-12T08:00:00Z',
  }
  const legacyPrice = { ...price, output_resolution: '1k' as const }
  return [
    {
      sku: 'ai-image:partial-edit',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '局部重绘',
      sort_order: 10,
      status: 'unpriced',
      pricing_variants: ['default'],
    },
    {
      sku: 'ai-image:remove-bg',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '抠图',
      sort_order: 20,
      status: 'active',
      pricing_variants: ['default'],
      current_prices: { default: price },
    },
    {
      sku: 'ai-image:hd-upscale',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: 'HD',
      sort_order: 30,
      status: 'disabled',
      pricing_variants: ['default'],
      current_prices: { default: price },
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
