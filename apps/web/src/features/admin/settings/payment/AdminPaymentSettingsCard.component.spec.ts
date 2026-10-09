import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, within } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import AdminPaymentSettingsCard from './AdminPaymentSettingsCard.vue'

vi.mock('../../../../shared/composables/useAppToast')
vi.mock('../../payments/../shared/logos/brandLogoResource', () => ({
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
    expect(await screen.findByText('Product 已就绪')).toBeVisible()
    expect(await screen.findByText('Webhook 未就绪')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '配置' }))
    const dialog = await screen.findByRole('dialog', { name: 'Stripe 设置' })
    expect(within(dialog).getByLabelText('Stripe API Key')).toBeVisible()
  })

  it('renders a retryable load error', async () => {
    mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/payments/providers/stripe', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'stripe down' } }, { status: 500 }),
      ),
    )
    const { user } = renderCard()

    expect(await screen.findByText('加载 Stripe 设置失败，请重试')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('加载 Stripe 设置失败，请重试')).toBeVisible()
  })
})
