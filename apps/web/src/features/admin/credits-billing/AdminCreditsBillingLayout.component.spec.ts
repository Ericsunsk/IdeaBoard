import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import {
  ADMIN_CREDIT_PRICING_PATH,
  ADMIN_REWARD_CODES_PATH,
} from '../../../shared/constants/routePaths'
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
      http.get('/api/admin/credits/health', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            status: 'critical',
            open_findings: { total: 1, warning: 0, error: 0, critical: 1, high_risk: 1 },
            restricted_wallet_count: 1,
          },
        }),
      ),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(AdminCreditsBillingLayout, {
      global: {
        plugins: [
          [PrimeVue, primeVueTestOptions],
          [VueQueryPlugin, { queryClient }],
        ],
        stubs: { RouterView: true },
      },
    })

    expect(await screen.findByText('异常')).toBeVisible()
    expect(screen.getByText('账本健康')).toBeVisible()
  })

  it('exposes only the canonical reward code navigation entry', async () => {
    useMswHandlers(
      http.get('/api/admin/credits/health', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            status: 'healthy',
            open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
            restricted_wallet_count: 0,
          },
        }),
      ),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminCreditsBillingLayout, {
      global: {
        plugins: [
          [PrimeVue, primeVueTestOptions],
          [VueQueryPlugin, { queryClient }],
        ],
        stubs: { RouterView: true },
      },
    })

    expect(screen.getByText('兑换码')).toBeVisible()
    expect(screen.queryByText('用户Idea账户')).not.toBeInTheDocument()

    await user.click(screen.getByText('兑换码'))

    expect(routerMocks.push).toHaveBeenCalledWith(ADMIN_REWARD_CODES_PATH)
  })
})
