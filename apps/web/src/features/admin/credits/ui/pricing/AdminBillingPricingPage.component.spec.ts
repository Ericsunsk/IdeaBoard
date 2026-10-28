import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { renderWithUser, screen, waitFor } from '@/test/render'
import { expectDomOrder } from '@/test/domOrder'
import AdminBillingPricingPage from './AdminBillingPricingPage.vue'
import {
  createBillingCatalogHandlers,
  createBillingCatalogMswState,
  makeCatalogItems,
} from '../../session/pricing/testing'

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('../../../shared/session/feedback/useAdminConfirm', () => ({
  useAdminConfirm: () => ({
    confirm: confirmMock,
  }),
}))

vi.mock('../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

describe('AdminBillingPricingPage', () => {
  beforeEach(() => {
    confirmMock.mockReset()
    confirmMock.mockResolvedValue(true)
    toast.success.mockReset()
    toast.warning.mockReset()
    toast.error.mockReset()
    useMswHandlers(
      http.get('/api/control/v1/platform/ai-gateway/catalog', () =>
        HttpResponse.json({ data: [] }),
      ),
      // The pool summary column loads one pool per ai-image SKU; default
      // to empty pools so pricing-only tests stay focused.
      http.get('/api/control/v1/billing/catalog/:sku/pool', ({ params }) => {
        const sku = String(params.sku)
        return HttpResponse.json({
          data: {
            service: sku.startsWith('ai-image:') ? sku.slice('ai-image:'.length) : sku,
            sku,
            version: 0,
            entries: [],
          },
        })
      }),
    )
  })
  it('preserves the grid surface while the Catalog is loading', () => {
    useMswHandlers(...createBillingCatalogHandlers(createBillingCatalogMswState({ hang: true })))

    renderPricingPage()

    expect(screen.getByRole('progressbar')).toBeVisible()
  })

  it('renders namespace tables with node subtitles in Service cells', async () => {
    useMswHandlers(...createBillingCatalogHandlers(createBillingCatalogMswState()))

    renderPricingPage()

    expect(await screen.findByText('inpaint')).toBeVisible()
    expect(screen.getByText('cutout')).toBeVisible()
    expect(screen.getByText('upscale')).toBeVisible()
    // Service 列与任务监控页同源：图标 + slug（displayName 降级保留第二行）
    expect(screen.getByText('inpaint')).toBeVisible()
    expect(screen.getByText('cutout')).toBeVisible()
    expect(screen.getByText('upscale')).toBeVisible()
    expect(screen.getByText('legacy:removed')).toBeVisible()
    expect(screen.getByText('Unpriced')).toBeVisible()
    expect(screen.getByText('Active')).toBeVisible()
    expect(screen.getByText('Disabled')).toBeVisible()
    expect(screen.getByText('Archived')).toBeVisible()
    expect(screen.getAllByText('1')).toHaveLength(3)
    expect(screen.getByRole('heading', { name: 'Image' })).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Other' })).toBeVisible()
    expect(screen.queryByText(/共 \d+ 个计费功能/)).not.toBeInTheDocument()
    expect(screen.queryByText(/共 \d+ 个功能/)).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'ai-search' })).not.toBeInTheDocument()
    // Service 次标题为对应节点（不带“节点”后缀）；表头同名故共 4 处
    expect(screen.getAllByText('Image')).toHaveLength(4)
    expect(screen.queryByRole('button', { name: '新建 SKU' })).not.toBeInTheDocument()
  })

  it('uses the shared service order across catalog states and input order', async () => {
    const items = [
      ...makeCatalogItems().reverse(),
      ...['fusion', 'generation', 'outpaint', 'render', 'unknown'].map((slug) => ({
        sku: `ai-image:${slug}`,
        namespace: 'ai-image',
        product_kind: 'workflow-feature',
        display_name: slug,
        sort_order: 0,
        status: 'unpriced',
        pricing_variants: ['1k'],
      })),
      {
        sku: 'ai-image:variations',
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
    useMswHandlers(...createBillingCatalogHandlers(createBillingCatalogMswState({ items })))

    renderPricingPage()

    expect(await screen.findByText('variations')).toBeVisible()
    expectTextOrder([
      'render',
      'variations',
      'inpaint',
      'outpaint',
      'upscale',
      'cutout',
      'generation',
      'fusion',
      'unknown',
      'legacy:removed',
    ])
  })

  it('shows an empty state when no products are registered', async () => {
    useMswHandlers(...createBillingCatalogHandlers(createBillingCatalogMswState({ items: [] })))

    renderPricingPage()

    expect(await screen.findByText('暂无可定价功能')).toBeVisible()
  })

  it('routes load failures to toast and recovers through the toolbar refresh', async () => {
    const state = createBillingCatalogMswState({ failFirst: 1 })
    useMswHandlers(...createBillingCatalogHandlers(state))

    const { user } = renderPricingPage()

    // 整页行内锚点已删除：失败只走 toast，工具栏刷新即重试入口。
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('功能定价加载失败'))
    expect(screen.queryByText('功能定价加载失败')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '刷新功能定价' }))
    expect(await screen.findByText('inpaint')).toBeVisible()
  })

  it('publishes an explicit zero price from the card dialog with idempotency headers', async () => {
    let publishRequests = 0
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      http.post('/api/control/v1/billing/catalog/:sku/prices', async ({ request, params }) => {
        publishRequests += 1
        expect(params.sku).toBe('ai-image:inpaint')
        await expect(request.json()).resolves.toEqual({
          prices: { '1k': 0 },
        })
        return HttpResponse.json({
          data: {
            prices: {
              '1k': {
                output_resolution: '1k',
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
    await user.click(await screen.findByRole('button', { name: '设置' }))
    expectTextOrder(['inpaint', 'upscale', 'cutout'])
    const priceInput = screen.getByRole('spinbutton', { name: '1K' })
    await user.type(priceInput, '0')
    expect(priceInput).toHaveValue('0')
    priceInput.blur()
    await waitFor(() => expect(priceInput).toHaveValue('0 Ideas'))
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('0')).toBeVisible()
    expect(screen.getAllByText('Active')).toHaveLength(2)
    expectTextOrder(['inpaint', 'upscale', 'cutout'])
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(publishRequests).toBe(1)
  })

  it('disables and re-enables a priced feature from its card', async () => {
    const requests: boolean[] = []
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      http.put('/api/control/v1/billing/catalog/:sku/availability', async ({ request, params }) => {
        expect(params.sku).toBe('ai-image:cutout')
        const body = (await request.json()) as { enabled: boolean }
        requests.push(body.enabled)
        return HttpResponse.json({
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
    expectTextOrder(['inpaint', 'upscale', 'cutout'])
    expect(switchControl).toBeChecked()
    await user.click(switchControl)
    await waitFor(() => expect(switchControl).not.toBeChecked())
    expectTextOrder(['inpaint', 'upscale', 'cutout'])
    await user.click(switchControl)
    await waitFor(() => expect(switchControl).toBeChecked())
    expect(requests).toEqual([false, true])
    expect(confirmMock).not.toHaveBeenCalled()

    // 未定价行开关常驻但锁死：入口只有设置按钮
    expect(screen.getByRole('switch', { name: '局部重绘功能状态' })).toBeDisabled()
  })

  it('keeps publish failures in the dialog and allows a single retry', async () => {
    let requests = 0
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      http.post('/api/control/v1/billing/catalog/:sku/prices', () => {
        requests += 1
        if (requests === 1) {
          return HttpResponse.json(
            { error: { code: 'service_unavailable', message: 'publish unavailable' } },
            { status: 500 },
          )
        }
        return HttpResponse.json({
          data: {
            prices: {
              '1k': {
                output_resolution: '1k',
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
    await user.click(await screen.findByRole('button', { name: '设置' }))
    await user.type(screen.getByRole('spinbutton', { name: '1K' }), '3')
    await user.click(screen.getByRole('button', { name: '保存' }))

    // 发布失败只走 toast（对话框保持打开可重试），不再写行内。
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('publish unavailable'))
    expect(screen.queryByText('publish unavailable')).not.toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(await screen.findByText('3')).toBeVisible()
    expect(requests).toBe(2)
  })

  it('shows price publish errors from the unified settings dialog', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      http.post('/api/control/v1/billing/catalog/:sku/prices', () =>
        HttpResponse.json(
          { error: { code: 'service_unavailable', message: 'publish unavailable' } },
          { status: 503 },
        ),
      ),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '设置' }))
    await user.type(screen.getByRole('spinbutton', { name: '1K' }), '3')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('publish unavailable'))
    expect(screen.queryByText('publish unavailable')).not.toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeVisible()
  })

  it('validates the price and blocks duplicate publishes while pending', async () => {
    let publishRequests = 0
    let releasePublish!: () => void
    const publishGate = new Promise<void>((resolve) => {
      releasePublish = resolve
    })
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      http.post('/api/control/v1/billing/catalog/:sku/prices', async () => {
        publishRequests += 1
        await publishGate
        return HttpResponse.json({
          data: {
            prices: {
              '1k': {
                output_resolution: '1k',
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
    await user.click(await screen.findByRole('button', { name: '设置' }))
    const publishButton = screen.getByRole('button', { name: '保存' })
    await user.click(publishButton)
    expect(screen.getByText('请输入价格')).toBeVisible()
    expect(publishRequests).toBe(0)

    await user.type(screen.getByRole('spinbutton', { name: /1K/ }), '5')
    await user.click(publishButton)
    expect(publishButton).toBeDisabled()
    await user.click(publishButton)
    expect(publishRequests).toBe(1)

    releasePublish()
    expect(await screen.findByText('5')).toBeVisible()
  })

  it('confirms and removes an unreferenced Archived card', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      http.delete('/api/control/v1/billing/catalog/:sku', ({ params }) =>
        HttpResponse.json({ data: { sku: params.sku } }),
      ),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '删除' }))
    expect(confirmMock).toHaveBeenCalledOnce()

    await waitFor(() => {
      expect(screen.queryByText('legacy:removed')).not.toBeInTheDocument()
    })
    expect(toast.success).toHaveBeenCalledWith('已归档功能已删除')
  })

  it('keeps the Archived card when durable history blocks deletion', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      http.delete('/api/control/v1/billing/catalog/:sku', () =>
        HttpResponse.json(
          { error: { code: 'conflict', message: '功能已有历史计费记录，无法删除' } },
          { status: 409 },
        ),
      ),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '删除' }))
    expect(confirmMock).toHaveBeenCalledOnce()

    await waitFor(() => {
      expect(screen.getByText('legacy:removed')).toBeVisible()
    })
    expect(toast.error).toHaveBeenCalledWith('功能已有历史计费记录，无法删除')
  })
})

function renderPricingPage() {
  // Fresh client per render: sharing one client across tests would serve
  // stale lists and hide refetch behavior.
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return renderWithUser(AdminBillingPricingPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: { Refresh: true, 'router-link': { template: '<a v-bind="$attrs"><slot /></a>' } },
    },
  })
}

function expectTextOrder(labels: string[]) {
  const elements = labels.map((label) => screen.getByText(label))
  expectDomOrder(elements)
}
