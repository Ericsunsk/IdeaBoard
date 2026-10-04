import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminBillingPricingPage from './AdminBillingPricingPage.vue'
import { createBillingCatalogHandlers, createBillingCatalogMswState } from './testing'

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('pool review'))
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
            { model_id: 'a/first', enabled: true, resolutions: ['1k', '2k', '4k'] },
            { model_id: 'b/second', enabled: true, resolutions: ['2k', '4k'] },
          ],
        },
      })
    }),
    // The editor's add-from-catalog list reuses the existing model catalog.
    http.get('/api/control/v1/platform/ai-gateway/catalog', () =>
      HttpResponse.json({ data: [{ id: 'a/first', name: 'First' }] }),
    ),
  ]
}

describe('AdminBillingPricingPage model pools', () => {
  beforeEach(() => {
    confirmMock.mockReset()
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReset()
    readReasonMock.mockReturnValue('pool review')
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

  it('opens the pool editor from the action column settings dialog', async () => {
    useMswHandlers(
      ...createBillingCatalogHandlers(createBillingCatalogMswState()),
      ...poolHandlers(),
    )
    const { user } = renderPricingPage()

    // 抠图是 active 状态，其操作列按钮为“调整”
    await user.click((await screen.findAllByRole('button', { name: '调整' }))[0]!)
    const modelTab = await screen.findByRole('tab', { name: 'Model' })
    await user.click(modelTab)
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: '模型池 · 抠图' })).toBeVisible()
    })
    expect(screen.getByText('版本 v2')).toBeVisible()
  })
})

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
