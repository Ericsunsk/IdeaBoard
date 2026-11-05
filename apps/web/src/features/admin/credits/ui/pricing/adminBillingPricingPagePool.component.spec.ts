import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { renderWithUser, screen, waitFor, within } from '@/test/render'
import AdminBillingPricingPage from './AdminBillingPricingPage.vue'
import {
  createBillingCatalogHandlers,
  createBillingCatalogMswState,
} from '../../session/pricing/testing'

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))
vi.mock('@/core/composables/useConfirmDialog', () => ({
  useConfirm: () => ({
    confirm: confirmMock,
  }),
}))

vi.mock('../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
}))

function poolHandlers(failSkus: string[] = []) {
  return [
    http.get('/api/control/v1/billing/catalog/:sku/pool', ({ params }) => {
      const sku = String(params.sku)
      if (failSkus.includes(sku)) {
        return HttpResponse.json(
          { error: { code: 'service_unavailable', message: 'down', request_id: 'req-1' } },
          { status: 503 },
        )
      }
      return HttpResponse.json({
        data: {
          service: sku.slice('ai-image:'.length),
          sku,
          version: 2,
          entries: [
            { provider: 'a', model_id: 'first', enabled: true, resolutions: ['1k', '2k', '4k'] },
            { provider: 'b', model_id: 'second', enabled: true, resolutions: ['2k', '4k'] },
          ],
        },
      })
    }),
    // The editor's add-from-catalog list reuses the existing model catalog
    // (full GatewayModelCatalogItem shape; minimal id/name would trip
    // `toAdminModelItem` on `capabilities` and surface a spurious catalog error).
    http.get('/api/control/v1/platform/ai-gateway/catalog', () =>
      HttpResponse.json({
        data: [
          {
            id: 'third',
            name: 'Third',
            provider: 'c',
            description: '',
            context_or_resolution: '',
            capabilities: [],
            hosting: 'test',
            terms_url: '',
            docs_url: '',
          },
        ],
      }),
    ),
  ]
}

describe('AdminBillingPricingPage model pools', () => {
  beforeEach(() => {
    confirmMock.mockReset()
    confirmMock.mockResolvedValue(true)
    toast.success.mockReset()
    toast.warning.mockReset()
    toast.error.mockReset()
  })

  it('renders the compact pool summary per Service row', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
    )
    renderPricingPage()

    expect((await screen.findAllByText('a/first')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('b/second').length).toBeGreaterThan(0)
    // 1K appears in the pool summary (a/first [1K][2K][4K]).
    expect(screen.getAllByText('1K').length).toBeGreaterThan(0)
    // Model pool edit button was removed from the table cells.
    expect(screen.queryByRole('button', { name: '编辑模型池' })).not.toBeInTheDocument()
  })

  it('keeps a failed pool load visible per row without failing the page', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(['ai-image:cutout']),
    )
    renderPricingPage()

    expect((await screen.findAllByText('a/first')).length).toBeGreaterThan(0)
    expect(screen.getByText('池加载失败')).toBeVisible()
    expect(screen.queryByText('功能定价加载失败')).not.toBeInTheDocument()
  })

  it('opens the pool editor directly in the unified settings dialog', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
    )
    const { user } = renderPricingPage()

    // 抠图是 active 状态，其操作列按钮为“调整”
    await user.click(await findCutoutSettingsButton())
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: '编辑 · 抠图' })).toBeVisible()
    })
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).queryByText(/版本/)).not.toBeInTheDocument()
    expect(within(dialog).getByText('a/first')).toBeVisible()
    expect(within(dialog).getByRole('button', { name: 'a/first1K分辨率已支持' })).toBeVisible()
  })

  it('saves pool changes from the settings dialog', async () => {
    let replaceRequests = 0
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
      http.put('/api/control/v1/billing/catalog/:sku/pool', async ({ request, params }) => {
        replaceRequests += 1
        expect(params.sku).toBe('ai-image:cutout')
        const body = (await request.json()) as {
          expected_version: number
          entries: unknown[]
        }
        expect(body.expected_version).toBe(2)
        return HttpResponse.json({
          data: {
            service: 'cutout',
            sku: 'ai-image:cutout',
            version: 3,
            entries: body.entries,
          },
        })
      }),
    )
    const { user } = renderPricingPage()

    await user.click(await findCutoutSettingsButton())
    expect(await screen.findByRole('heading', { name: '编辑 · 抠图' })).toBeVisible()
    const toggleButton = screen.getByRole('button', { name: 'a/first4K分辨率已支持' })
    await user.click(toggleButton)
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(replaceRequests).toBe(1)
    })
    expect(confirmMock).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('模型池已保存')
  })

  it('shows model pool save errors in the settings dialog', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
      http.put('/api/control/v1/billing/catalog/:sku/pool', () =>
        HttpResponse.json(
          { error: { code: 'service_unavailable', message: 'pool unavailable' } },
          { status: 503 },
        ),
      ),
    )
    const { user } = renderPricingPage()

    await user.click(await findCutoutSettingsButton())
    await user.click(screen.getByRole('button', { name: 'a/first4K分辨率已支持' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    // 池保存失败只走 toast（对话框保持打开可重试），不再写行内。
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('pool unavailable'))
    expect(screen.queryByText('pool unavailable')).not.toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeVisible()
  })

  it('saves both price and pool changes with a single confirmation when both are modified', async () => {
    let publishRequests = 0
    let replaceRequests = 0
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
      http.post('/api/control/v1/billing/catalog/:sku/prices', async ({ request, params }) => {
        publishRequests += 1
        expect(params.sku).toBe('ai-image:cutout')
        const body = (await request.json()) as { prices: Record<string, number> }
        expect(body.prices['1k']).toBe(5)
        return HttpResponse.json({
          data: {
            prices: {
              '1k': {
                output_resolution: '1k',
                price_version_id: 11,
                version: 2,
                unit_credits: 5,
                created_at: '2026-07-16T12:00:00Z',
              },
            },
          },
        })
      }),
      http.put('/api/control/v1/billing/catalog/:sku/pool', async ({ request, params }) => {
        replaceRequests += 1
        expect(params.sku).toBe('ai-image:cutout')
        const body = (await request.json()) as {
          expected_version: number
          entries: unknown[]
        }
        expect(body.expected_version).toBe(2)
        return HttpResponse.json({
          data: {
            service: 'cutout',
            sku: 'ai-image:cutout',
            version: 3,
            entries: body.entries,
          },
        })
      }),
    )
    const { user } = renderPricingPage()

    await user.click(await findCutoutSettingsButton())
    expect(await screen.findByRole('heading', { name: '编辑 · 抠图' })).toBeVisible()

    // 1. 修改价格
    const priceInput = screen.getByRole('spinbutton', { name: '1K' })
    await user.clear(priceInput)
    await user.type(priceInput, '5')

    // 2. 修改模型池
    const toggleButton = screen.getByRole('button', { name: 'a/first4K分辨率已支持' })
    await user.click(toggleButton)

    // 3. 点击保存
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(publishRequests).toBe(1)
      expect(replaceRequests).toBe(1)
    })
    expect(toast.success).toHaveBeenCalledWith('设置已保存')
  })

  it('does not repeat a successful price publish when the pool save is retried', async () => {
    let publishRequests = 0
    let replaceRequests = 0
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
      http.post('/api/control/v1/billing/catalog/:sku/prices', async () => {
        publishRequests += 1
        return HttpResponse.json({
          data: {
            prices: {
              '1k': {
                output_resolution: '1k',
                price_version_id: 11,
                version: 2,
                unit_credits: 5,
                created_at: '2026-07-16T12:00:00Z',
              },
            },
          },
        })
      }),
      http.put('/api/control/v1/billing/catalog/:sku/pool', async ({ request, params }) => {
        replaceRequests += 1
        if (replaceRequests === 1) {
          return HttpResponse.json(
            { error: { code: 'service_unavailable', message: 'pool unavailable' } },
            { status: 503 },
          )
        }
        const body = (await request.json()) as { expected_version: number; entries: unknown[] }
        expect(params.sku).toBe('ai-image:cutout')
        expect(body.expected_version).toBe(2)
        return HttpResponse.json({
          data: {
            service: 'cutout',
            sku: 'ai-image:cutout',
            version: 3,
            entries: body.entries,
          },
        })
      }),
    )
    const { user } = renderPricingPage()

    await user.click(await findCutoutSettingsButton())
    expect(await screen.findByRole('heading', { name: '编辑 · 抠图' })).toBeVisible()
    await user.clear(screen.getByRole('spinbutton', { name: '1K' }))
    await user.type(screen.getByRole('spinbutton', { name: '1K' }), '5')
    await user.click(screen.getByRole('button', { name: 'a/first4K分辨率已支持' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    // 组合保存部分失败仅 toast 命名失败部分，不写 inline，避免双通知重复。
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
    const combinedToast = String(toast.error.mock.calls[0]?.[0] ?? '')
    expect(combinedToast).toContain('部分设置保存失败，请重试')
    expect(combinedToast).not.toContain('请根据提示')
    expect(combinedToast).toContain('Model')
    expect(combinedToast).toContain('pool unavailable')
    expect(screen.queryByText('pool unavailable')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重新加载' })).not.toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeVisible()
    expect(publishRequests).toBe(1)
    expect(replaceRequests).toBe(1)

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(replaceRequests).toBe(2))
    expect(publishRequests).toBe(1)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(toast.success).toHaveBeenCalledWith('模型池已保存')
  })

  it('names the pricing part in the toast without inline duplication when combined pricing fails', async () => {
    let publishRequests = 0
    let replaceRequests = 0
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
      http.post('/api/control/v1/billing/catalog/:sku/prices', () => {
        publishRequests += 1
        return HttpResponse.json(
          { error: { code: 'service_unavailable', message: 'pricing unavailable' } },
          { status: 503 },
        )
      }),
      http.put('/api/control/v1/billing/catalog/:sku/pool', async ({ request }) => {
        replaceRequests += 1
        const body = (await request.json()) as { entries: unknown[] }
        return HttpResponse.json({
          data: {
            service: 'cutout',
            sku: 'ai-image:cutout',
            version: 3,
            entries: body.entries,
          },
        })
      }),
    )
    const { user } = renderPricingPage()

    await user.click(await findCutoutSettingsButton())
    expect(await screen.findByRole('heading', { name: '编辑 · 抠图' })).toBeVisible()
    await user.clear(screen.getByRole('spinbutton', { name: '1K' }))
    await user.type(screen.getByRole('spinbutton', { name: '1K' }), '5')
    await user.click(screen.getByRole('button', { name: 'a/first4K分辨率已支持' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
    const combinedToast = String(toast.error.mock.calls[0]?.[0] ?? '')
    expect(combinedToast).toContain('部分设置保存失败，请重试')
    expect(combinedToast).not.toContain('请根据提示')
    expect(combinedToast).toContain('定价设置')
    expect(combinedToast).toContain('pricing unavailable')
    expect(screen.queryByText('pricing unavailable')).not.toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeVisible()
    expect(publishRequests).toBe(1)
    expect(replaceRequests).toBe(1)
  })

  it('offers a reload entry when the combined pool save hits a 409 conflict', async () => {
    let publishRequests = 0
    let replaceRequests = 0
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
      http.post('/api/control/v1/billing/catalog/:sku/prices', async () => {
        publishRequests += 1
        return HttpResponse.json({
          data: {
            prices: {
              '1k': {
                output_resolution: '1k',
                price_version_id: 11,
                version: 2,
                unit_credits: 5,
                created_at: '2026-07-16T12:00:00Z',
              },
            },
          },
        })
      }),
      http.put('/api/control/v1/billing/catalog/:sku/pool', () => {
        replaceRequests += 1
        return HttpResponse.json(
          { error: { code: 'conflict', message: 'stale', request_id: 'req-1' } },
          { status: 409 },
        )
      }),
    )
    const { user } = renderPricingPage()

    await user.click(await findCutoutSettingsButton())
    expect(await screen.findByRole('heading', { name: '编辑 · 抠图' })).toBeVisible()
    await user.clear(screen.getByRole('spinbutton', { name: '1K' }))
    await user.type(screen.getByRole('spinbutton', { name: '1K' }), '5')
    await user.click(screen.getByRole('button', { name: 'a/first4K分辨率已支持' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
    const combinedToast = String(toast.error.mock.calls[0]?.[0] ?? '')
    expect(combinedToast).toContain('部分设置保存失败，请重试')
    expect(combinedToast).not.toContain('请根据提示')
    expect(combinedToast).toContain('Model')
    expect(combinedToast).toContain('模型池已被更新，请刷新后重试')
    // 409 时对话框内保留“重新加载”入口（不写 inline 明细，避免双通知）。
    expect(await screen.findByRole('button', { name: '重新加载' })).toBeVisible()
    expect(screen.getByRole('dialog')).toBeVisible()
    expect(publishRequests).toBe(1)
    expect(replaceRequests).toBe(1)
  })
})

async function findCutoutSettingsButton() {
  const row = (await screen.findByText('cutout')).closest('tr')!
  return within(row).getByRole('button', { name: '调整' })
}

function renderPricingPage() {
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
