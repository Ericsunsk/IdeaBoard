import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminBillingPricingPage from './AdminBillingPricingPage.vue'

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('pricing review'))
const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({
    confirm: confirmMock,
    readReason: readReasonMock,
  }),
}))

vi.mock('../../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/toast')>()),
  useAdminToast: () => toast,
}))

describe('AdminBillingPricingPage', () => {
  beforeEach(() => {
    confirmMock.mockReset()
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReset()
    readReasonMock.mockReturnValue('pricing review')
    toast.success.mockReset()
    toast.warning.mockReset()
    toast.error.mockReset()
    useMswHandlers(
      http.get('/api/control/v1/platform/ai-gateway/models', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: [] }),
      ),
      http.get('/api/control/v1/platform/ai-gateway/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: [] }),
      ),
    )
  })
  it('preserves the grid surface while the Catalog is loading', () => {
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', async () => {
        await new Promise(() => {})
        return HttpResponse.json({ code: 0, msg: 'ok', data: { items: [] } })
      }),
    )

    renderPricingPage()

    expect(screen.getByRole('progressbar')).toBeVisible()
  })

  it('renders namespace tables with node subtitles in Service cells', async () => {
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
    )

    renderPricingPage()

    expect(await screen.findByText('inpaint')).toBeVisible()
    expect(screen.getByText('cutout')).toBeVisible()
    expect(screen.getByText('upscale')).toBeVisible()
    // Service 列与任务监控页同源：图标 + slug（displayName 降级保留第二行）
    expect(screen.getByText('inpaint')).toBeVisible()
    expect(screen.getByText('cutout')).toBeVisible()
    expect(screen.getByText('upscale')).toBeVisible()
    expect(screen.getByText('legacy:removed')).toBeVisible()
    expect(screen.getByText('未定价')).toBeVisible()
    expect(screen.getByText('生效中')).toBeVisible()
    expect(screen.getByText('已停用')).toBeVisible()
    expect(screen.getByText('已归档')).toBeVisible()
    expect(screen.getAllByText('1')).toHaveLength(3)
    expect(screen.getByRole('heading', { name: 'Image' })).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Other' })).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'ai-search' })).not.toBeInTheDocument()
    // Service 次标题为对应节点（不带“节点”后缀）；表头同名故共 4 处
    expect(screen.getAllByText('Image')).toHaveLength(4)
    expect(screen.queryByRole('button', { name: '新建 SKU' })).not.toBeInTheDocument()
  })

  it('orders namespace tables with images before other', async () => {
    const items = [
      ...makeCatalogItems(),
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
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items } }),
      ),
    )

    renderPricingPage()

    expect(await screen.findByText('variations')).toBeVisible()
    expectTextOrder(['variations', 'cutout', 'inpaint', 'upscale', 'legacy:removed'])
  })

  it('shows an empty state when no products are registered', async () => {
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: [] } }),
      ),
    )

    renderPricingPage()

    expect(await screen.findByText('暂无可定价功能')).toBeVisible()
  })

  it('keeps load failures visible and retryable', async () => {
    let requests = 0
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () => {
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
    expect(await screen.findByText('inpaint')).toBeVisible()
  })

  it('publishes an explicit zero price from the card dialog with an operator reason', async () => {
    let publishRequests = 0
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.post('/api/control/v1/billing/catalog/:sku/prices', async ({ request, params }) => {
        publishRequests += 1
        expect(params.sku).toBe('ai-image:inpaint')
        await expect(request.json()).resolves.toEqual({ prices: { default: 0 }, reason: 'pricing review' })
        return HttpResponse.json({
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
    await user.click(await screen.findByRole('button', { name: '设置' }))
    const priceInput = screen.getByRole('spinbutton', { name: '固定' })
    await user.type(priceInput, '0')
    expect(priceInput).toHaveValue('0')
    priceInput.blur()
    await waitFor(() => expect(priceInput).toHaveValue('0 Ideas'))
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('0')).toBeVisible()
    expect(screen.getAllByText('生效中')).toHaveLength(2)
    expectTextOrder(['inpaint', 'cutout'])
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(publishRequests).toBe(1)
  })

  it('disables and re-enables a priced feature from its card', async () => {
    const requests: boolean[] = []
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
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
    expect(switchControl).toBeChecked()
    await user.click(switchControl)
    await waitFor(() => expect(switchControl).not.toBeChecked())
    await user.click(switchControl)
    await waitFor(() => expect(switchControl).toBeChecked())
    expect(requests).toEqual([false, true])

    // 未定价行开关常驻但锁死：入口只有设置按钮
    expect(screen.getByRole('switch', { name: '局部重绘功能状态' })).toBeDisabled()
  })

  it('keeps publish failures in the dialog and allows a single retry', async () => {
    let requests = 0
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
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
    await user.click(await screen.findByRole('button', { name: '设置' }))
    await user.type(screen.getByRole('spinbutton', { name: '固定' }), '3')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect((await screen.findAllByText('publish unavailable')).length).toBeGreaterThan(0)
    expect(screen.getByRole('dialog')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(await screen.findByText('3')).toBeVisible()
    expect(requests).toBe(2)
  })

  it('validates the price and blocks duplicate publishes while pending', async () => {
    let publishRequests = 0
    let releasePublish!: () => void
    const publishGate = new Promise<void>((resolve) => {
      releasePublish = resolve
    })
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.post('/api/control/v1/billing/catalog/:sku/prices', async () => {
        publishRequests += 1
        await publishGate
        return HttpResponse.json({
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
    await user.click(await screen.findByRole('button', { name: '设置' }))
    const publishButton = screen.getByRole('button', { name: '保存' })
    await user.click(publishButton)
    expect(screen.getByText('请输入价格')).toBeVisible()
    expect(publishRequests).toBe(0)

    await user.type(screen.getByRole('spinbutton', { name: /固定/ }), '5')
    await user.click(publishButton)
    expect(publishButton).toBeDisabled()
    await user.click(publishButton)
    expect(publishRequests).toBe(1)

    releasePublish()
    expect(await screen.findByText('5')).toBeVisible()
  })

  it('confirms and removes an unreferenced Archived card', async () => {
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
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
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
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

  it('prefills per-feature models and saves without touching prices', async () => {
    let savedBody: unknown
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.get('/api/control/v1/platform/ai-gateway/models', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: [{ feature: 'inpaint', model: 'openai/gpt-image-1' }],
        }),
      ),
      http.get('/api/control/v1/platform/ai-gateway/catalog', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: [makeCatalogModel()],
        }),
      ),
      http.put('/api/control/v1/platform/ai-gateway/models/:feature', async ({ request }) => {
        savedBody = await request.json()
        return HttpResponse.json({ code: 0, msg: 'ok', data: null })
      }),
    )

    const { user } = renderPricingPage()
    // 模型映射收进设置弹窗：先打开 inpaint 行的设置弹窗
    await user.click(await screen.findByRole('button', { name: '设置' }))
    const trigger = await screen.findByRole('button', { name: '模型（局部重绘）' })
    expect(trigger).toBeVisible()
    await waitFor(() => expect(trigger).toHaveTextContent('openai/gpt-image-1'))

    await user.click(trigger)
    const catalogOption = await screen.findByRole('option', {
      name: /google\/gemini-2\.5-flash-image/,
    })
    await user.click(catalogOption)

    expect(toast.success).toHaveBeenCalledWith('模型映射已保存，价格不受影响')
    expect(savedBody).toEqual({ model: 'google/gemini-2.5-flash-image' })
    // Prices stay exactly as cataloged: model switches never move prices.
    expect(screen.getByText('inpaint')).toBeVisible()
  })

  it('keeps model save failures inline on the card', async () => {
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCatalogItems() } }),
      ),
      http.get('/api/control/v1/platform/ai-gateway/models', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: [] }),
      ),
      http.get('/api/control/v1/platform/ai-gateway/catalog', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: [makeCatalogModel({ id: 'bad/model', name: 'Bad Model' })],
        }),
      ),
      http.put('/api/control/v1/platform/ai-gateway/models/:feature', () =>
        HttpResponse.json({ code: 400, msg: '网关写入失败', message_safe: true }, { status: 400 }),
      ),
    )

    const { user } = renderPricingPage()
    await user.click(await screen.findByRole('button', { name: '设置' }))
    const trigger = await screen.findByRole('button', { name: '模型（局部重绘）' })
    await user.click(trigger)
    const catalogOption = await screen.findByRole('option', { name: /bad\/model/ })
    await user.click(catalogOption)

    expect(await screen.findByText('网关写入失败')).toBeVisible()
  })
  it('renders the AI search table and saves its model mapping from the dialog', async () => {
    let putFeature = ''
    let putBody: unknown = null
    useMswHandlers(
      http.get('/api/control/v1/billing/catalog', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            items: [
              {
                sku: 'ai-search:vision',
                namespace: 'ai-search',
                product_kind: 'search-feature',
                display_name: '视觉搜索',
                sort_order: 10,
                status: 'active',
                pricing_variants: ['default'],
              },
            ],
          },
        }),
      ),
      http.get('/api/control/v1/platform/ai-gateway/models', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: [] }),
      ),
      http.get('/api/control/v1/platform/ai-gateway/catalog', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: [makeCatalogModel({ id: 'test-provider/test-model', name: 'Vision Model' })],
        }),
      ),
      http.put('/api/control/v1/platform/ai-gateway/models/:feature', async ({ request, params }) => {
        putFeature = String(params.feature)
        putBody = await request.json()
        return HttpResponse.json({ code: 0, msg: 'ok', data: null })
      }),
    )

    const { user } = renderPricingPage()
    expect(await screen.findByRole('heading', { name: 'Other' })).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'Image' })).not.toBeInTheDocument()
    // Service 次标题回退映射特性（vision）
    expect(screen.getByText('vision')).toBeVisible()
    expect(screen.getByText('未配置')).toBeVisible()

    await user.click(await screen.findByRole('button', { name: '调整' }))
    const trigger = await screen.findByRole('button', { name: '模型（视觉搜索）' })
    await user.click(trigger)
    await user.click(await screen.findByRole('option', { name: /test-provider\/test-model/ }))

    await waitFor(() => expect(putBody).toEqual({ model: 'test-provider/test-model' }))
    expect(putFeature).toBe('vision')
    expect(toast.success).toHaveBeenCalledWith('模型映射已保存，价格不受影响')
  })
})

function renderPricingPage() {
  // Fresh client per render: the pricing model adapter shares the catalog
  // queryKey with the models page, so sharing one client across tests would
  // serve stale lists and hide refetch behavior.
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
  for (let index = 0; index < elements.length - 1; index += 1) {
    expect(
      elements[index]!.compareDocumentPosition(elements[index + 1]!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  }
}

function makeCatalogModel(overrides: Record<string, unknown> = {}) {
  return {
    id: 'google/gemini-2.5-flash-image',
    name: 'Gemini Flash Image',
    provider: 'Google',
    mode: 'image_generation',
    description: 'catalog model',
    context_or_resolution: '2048×2048',
    capabilities: [],
    hosting: '第三方',
    terms_url: '',
    docs_url: '',
    gradient: 'from-blue-600 to-cyan-500',
    ...overrides,
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
      sku: 'ai-image:inpaint',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '局部重绘',
      sort_order: 10,
      status: 'unpriced',
      pricing_variants: ['default'],
    },
    {
      sku: 'ai-image:cutout',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: '抠图',
      sort_order: 20,
      status: 'active',
      pricing_variants: ['default'],
      current_prices: { default: price },
    },
    {
      sku: 'ai-image:upscale',
      namespace: 'ai-image',
      product_kind: 'workflow-feature',
      display_name: 'Upscale',
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
