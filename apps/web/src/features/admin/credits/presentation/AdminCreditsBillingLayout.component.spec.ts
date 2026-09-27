import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import {
  createCreditHealthHandlers,
  createCreditHealthMswState,
  makeCreditHealth,
} from '../health/testing'
import {
  ADMIN_CREDIT_PRICING_PATH,
  ADMIN_PAYMENT_ORDERS_PATH,
  ADMIN_REWARD_CODES_PATH,
} from '../../../../shared/constants/routePaths'
import AdminCreditsBillingLayout from './AdminCreditsBillingLayout.vue'

const routerMocks = vi.hoisted(() => ({
  push: vi.fn(),
  route: { path: '/admin/credits/pricing' },
}))

vi.mock('vue-router', async (importOriginal) => {
  const original = await importOriginal<typeof import('vue-router')>()
  return {
    ...original,
    useRoute: () => routerMocks.route,
    useRouter: () => ({ push: routerMocks.push }),
  }
})

describe('AdminCreditsBillingLayout', () => {
  beforeEach(() => {
    routerMocks.push.mockReset()
    routerMocks.route.path = ADMIN_CREDIT_PRICING_PATH
  })

  it('shows a non-healthy Badge on the health navigation entry', async () => {
    useMswHandlers(
      ...createCreditHealthHandlers(
        createCreditHealthMswState({
          health: makeCreditHealth({
            status: 'critical',
            open_findings: { total: 1, warning: 0, error: 0, critical: 1, high_risk: 1 },
            restricted_wallet_count: 1,
          }),
        }),
      ),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(AdminCreditsBillingLayout, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        stubs: { RouterView: true },
      },
    })

    expect(await screen.findByText('异常')).toBeVisible()
    expect(screen.getByText('账本健康')).toBeVisible()
  })

  it('exposes only the canonical reward code navigation entry', async () => {
    useMswHandlers(...createCreditHealthHandlers(createCreditHealthMswState()))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminCreditsBillingLayout, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        stubs: { RouterView: true },
      },
    })

    expect(screen.getByText('兑换码')).toBeVisible()
    expect(screen.queryByText('用户Idea账户')).not.toBeInTheDocument()

    await user.click(screen.getByText('兑换码'))

    expect(routerMocks.push).toHaveBeenCalledWith(ADMIN_REWARD_CODES_PATH)
  })

  it('exposes the payment order navigation entry and routes through the Credits layout', async () => {
    useMswHandlers(...createCreditHealthHandlers(createCreditHealthMswState()))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminCreditsBillingLayout, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        stubs: { RouterView: true },
      },
    })

    await user.click(screen.getByText('支付订单'))

    expect(routerMocks.push).toHaveBeenCalledWith(ADMIN_PAYMENT_ORDERS_PATH)
  })

  it('opens Stripe settings from the gear left of the billing navigation', async () => {
    useMswHandlers(
      ...createCreditHealthHandlers(createCreditHealthMswState()),
      http.get('/api/control/v1/payments/stripe-config', () =>
        HttpResponse.json({
          data: {
            configured: false,
            api_key_configured: false,
            webhook_secret_configured: false,
            charges_enabled: false,
            product_tax_code: '',
            product_ready: false,
            webhook_ready: false,
            purchases_enabled: false,
          },
        }),
      ),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminCreditsBillingLayout, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        stubs: { RouterView: true, transition: false },
      },
    })

    await user.click(screen.getByRole('button', { name: 'Stripe 设置' }))

    expect(await screen.findByRole('dialog', { name: 'Stripe 设置' })).toBeVisible()
  })
})
