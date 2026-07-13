import { beforeEach, describe, expect, it, vi } from 'vitest'
import PrimeVue from 'primevue/config'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminBillingPricingPage from './AdminBillingPricingPage.vue'

const confirmRequestMock = vi.hoisted(() => vi.fn())

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ request: confirmRequestMock }),
}))

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

describe('AdminBillingPricingPage', () => {
  let priceStateRequests: unknown[]

  beforeEach(() => {
    confirmRequestMock.mockReset()
    priceStateRequests = []
    useMswHandlers(
      http.get('/api/admin/billing/coverage', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: { items: makeCoverageItems() } }),
      ),
      http.get('/api/admin/billing/skus', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            items: [
              {
                sku: 'ai-image:sketch-to-image.77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18',
                display_name: '草图成图 / GPT Image 1',
                description: 'Canonical workflow price',
                enabled: true,
                current_price_version_id: 10,
                current_price: makePrice(),
                created_at: '2026-07-12T08:00:00Z',
                updated_at: '2026-07-12T08:00:00Z',
              },
            ],
          },
        }),
      ),
      http.get('/api/admin/billing/skus/:sku/prices', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { prices: [makePrice()] },
        }),
      ),
      http.patch('/api/admin/billing/skus/:sku/prices/:version', async ({ request }) => {
        priceStateRequests.push(await request.json())
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makePrice({ enabled: false }),
        })
      }),
    )
  })

  it('renders the real Catalog, canonical workflow identity, and immutable create warning', async () => {
    const { user } = renderPricingPage()

    expect((await screen.findAllByText('草图成图 / GPT Image 1'))[0]).toBeVisible()
    expect(screen.getByText('AI 生图 · 草图成图')).toBeVisible()
    expect(screen.getByRole('heading', { name: 'AI Workflow 计费覆盖' })).toBeVisible()
    expect(screen.queryByText(/当前 API 未暴露 workflow × model 映射/)).not.toBeInTheDocument()
    expect(screen.getAllByText('25 Ideas')[0]).toBeVisible()

    await user.click(screen.getByRole('button', { name: '新建 SKU' }))

    expect(screen.getByText(/SKU 是全局 immutable identity/)).toBeVisible()
    expect(screen.getByRole('textbox', { name: /SKU/ })).toBeEnabled()
  })

  it('renders every feature/model coverage state and prefills an unconfigured immutable SKU', async () => {
    const { user } = renderPricingPage()

    expect(await screen.findByText('gemini-ready')).toBeVisible()
    expect(screen.getByText('gemini-missing-sku')).toBeVisible()
    expect(screen.getByText('gemini-disabled-sku')).toBeVisible()
    expect(screen.getByText('gemini-no-price')).toBeVisible()
    expect(screen.getByText('gemini-disabled-price')).toBeVisible()
    expect(screen.getByText('SKU 未创建')).toBeVisible()
    expect(screen.getByText('SKU disabled')).toBeVisible()
    expect(screen.getByText('无 current price')).toBeVisible()
    expect(screen.getByText('current price disabled')).toBeVisible()
    expect(screen.getByText('12 Ideas')).toBeVisible()
    expect(screen.getByText('18 Ideas')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '为 gemini-missing-sku 配置 SKU' }))

    expect(screen.getByRole('textbox', { name: /SKU/ })).toHaveValue(makeCoverageItems()[1]?.sku)
    expect(screen.getByRole('textbox', { name: '展示名称' })).toHaveValue(
      '草图成图 / gemini-missing-sku',
    )
  })

  it('keeps coverage failures visible and recoverable without hiding the Catalog', async () => {
    useMswHandlers(
      http.get('/api/admin/billing/coverage', () =>
        HttpResponse.json({ code: 500, msg: 'coverage unavailable' }, { status: 500 }),
      ),
    )

    renderPricingPage()

    expect(await screen.findByText(/AI Workflow 计费覆盖加载失败/)).toBeVisible()
    expect(screen.getByRole('button', { name: '刷新 AI Workflow 计费覆盖' })).toBeEnabled()
    expect((await screen.findAllByText('草图成图 / GPT Image 1'))[0]).toBeVisible()
  })

  it('requires confirmation before publishing a new immutable price version', async () => {
    const { user } = renderPricingPage()

    await screen.findAllByText('草图成图 / GPT Image 1')
    await user.click(screen.getByRole('button', { name: '发布新价格' }))
    const creditsInput = screen.getByRole('spinbutton', { name: /单价/ })
    await user.clear(creditsInput)
    await user.type(creditsInput, '30')
    await user.click(screen.getByRole('button', { name: '确认发布' }))

    expect(confirmRequestMock).not.toHaveBeenCalled()
    expect(screen.getByText('请输入操作原因')).toBeVisible()

    await user.type(screen.getByRole('textbox', { name: /^操作原因/ }), '  七月调价  ')
    await user.click(screen.getByRole('button', { name: '确认发布' }))

    expect(confirmRequestMock).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认发布新价格版本',
        message: expect.stringContaining('30 Ideas'),
      }),
    )
  })

  it('requires an explicit reason dialog before changing price state and trims the request', async () => {
    const { user } = renderPricingPage()

    await user.click(await screen.findByRole('switch', { name: '停用价格版本 v2' }))

    expect(screen.getByRole('dialog', { name: '确认停用价格版本' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: '确认停用' }))
    expect(screen.getByText('请输入操作原因')).toBeVisible()
    expect(priceStateRequests).toEqual([])

    await user.type(screen.getByRole('textbox', { name: /^操作原因/ }), '  价格配置有误  ')
    await user.click(screen.getByRole('button', { name: '确认停用' }))

    await waitFor(() => {
      expect(priceStateRequests).toEqual([{ enabled: false, reason: '价格配置有误' }])
    })
    expect(confirmRequestMock).not.toHaveBeenCalled()
  })
})

function renderPricingPage() {
  return renderWithUser(AdminBillingPricingPage, {
    global: {
      plugins: [[PrimeVue, { unstyled: true }]],
      stubs: {
        NavArrowLeft: true,
        NavArrowRight: true,
        Plus: true,
        Refresh: true,
        Search: true,
        Xmark: true,
      },
    },
  })
}

function makePrice(overrides: Record<string, unknown> = {}) {
  return {
    price_version_id: 10,
    version: 2,
    unit_credits: 25,
    enabled: true,
    created_by: 7,
    created_at: '2026-07-12T08:00:00Z',
    updated_by: 7,
    updated_at: '2026-07-12T08:00:00Z',
    ...overrides,
  }
}

function makeCoverageItems() {
  const fingerprint = (seed: string) => seed.repeat(64).slice(0, 64)
  return [
    {
      feature: 'sketch-to-image',
      model: 'gemini-ready',
      sku: `ai-image:sketch-to-image.${fingerprint('a')}`,
      sku_configured: true,
      sku_enabled: true,
      price_configured: true,
      price_enabled: true,
      unit_credits: 25,
    },
    {
      feature: 'sketch-to-image',
      model: 'gemini-missing-sku',
      sku: `ai-image:sketch-to-image.${fingerprint('b')}`,
      sku_configured: false,
      sku_enabled: false,
      price_configured: false,
      price_enabled: false,
    },
    {
      feature: 'partial-edit',
      model: 'gemini-disabled-sku',
      sku: `ai-image:partial-edit.${fingerprint('c')}`,
      sku_configured: true,
      sku_enabled: false,
      price_configured: true,
      price_enabled: true,
      unit_credits: 12,
    },
    {
      feature: 'pattern-gen',
      model: 'gemini-no-price',
      sku: `ai-image:pattern-gen.${fingerprint('d')}`,
      sku_configured: true,
      sku_enabled: true,
      price_configured: false,
      price_enabled: false,
    },
    {
      feature: 'image-extend',
      model: 'gemini-disabled-price',
      sku: `ai-image:image-extend.${fingerprint('e')}`,
      sku_configured: true,
      sku_enabled: true,
      price_configured: true,
      price_enabled: false,
      unit_credits: 18,
    },
  ]
}
