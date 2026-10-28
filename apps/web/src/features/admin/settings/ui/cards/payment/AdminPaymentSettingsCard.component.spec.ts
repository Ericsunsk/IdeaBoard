import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { renderWithUser, screen, within } from '@/test/render'
import { mockAppToast, expectToastFeedback } from '@/test/toast'
import AdminPaymentSettingsCard from './AdminPaymentSettingsCard.vue'

vi.mock('../../../../../../core/composables/useAppToast')
vi.mock('../../../../shared/session/brand/brandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))

function stripeSummary(overrides: Record<string, unknown> = {}) {
  return {
    configured: true,
    api_key_configured: true,
    webhook_secret_configured: true,
    charges_enabled: true,
    product_tax_code: 'txcd_99999999',
    product_ready: true,
    webhook_ready: false,
    purchases_enabled: true,
    ...overrides,
  }
}

function renderCard() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithUser(AdminPaymentSettingsCard, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

describe('AdminPaymentSettingsCard', () => {
  it('renders readiness statuses and opens the settings dialog from configure', async () => {
    mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/payments/providers/stripe', () =>
        HttpResponse.json({ data: stripeSummary() }),
      ),
    )
    const { user } = renderCard()

    expect(await screen.findByText('支付')).toBeVisible()
    expect(await screen.findByText('收款配置')).toBeVisible()
    expect(await screen.findByText('Product Ready')).toBeVisible()
    expect(await screen.findByText('Webhook Not ready')).toBeVisible()
    // 方案 A：收款配置是配置态，走中性（不用 success 绿）；Product/Webhook 是健康态。
    expect(screen.getByText('Configured').parentElement).toHaveClass('bg-muted')
    expect(screen.getByText('Configured').parentElement).not.toHaveClass('bg-success-soft')
    expect(screen.getByText('Product Ready').parentElement).toHaveClass('bg-success-soft')
    expect(screen.getByText('Webhook Not ready').parentElement).toHaveClass('bg-muted')

    await user.click(screen.getByRole('button', { name: '配置' }))
    const dialog = await screen.findByRole('dialog', { name: 'Stripe 设置' })
    expect(within(dialog).getByLabelText('Stripe API Key')).toBeVisible()
  })

  it('routes load failure to toast error without an inline alert', async () => {
    const toast = mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/payments/providers/stripe', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'stripe down' } }, { status: 500 }),
      ),
    )
    renderCard()

    // 精简目标：底部 globalError AdminAlert 已删除，失败一律 toast；Status 列保留为真相源。
    await expectToastFeedback(toast, 'stripe down')
    expect(toast.success).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: '重试' })).not.toBeInTheDocument()
    expect(await screen.findByText('收款配置')).toBeVisible()
    expect(screen.getByText('Not configured')).toBeVisible()
    expect(screen.getByText('Product Not ready')).toBeVisible()
    expect(screen.getByText('Webhook Not ready')).toBeVisible()
  })
})
