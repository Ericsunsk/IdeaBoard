import { describe, expect, it } from 'vitest'
import { resolveAdminPageTitle } from './adminNavigation'
import { adminRoutes } from './adminRoutes'
import { adminNavigationRouteDefs } from './adminNavigationMeta'

describe('resolveAdminPageTitle', () => {
  it('keeps the Credits page title stable across child navigation', () => {
    for (const path of [
      '/admin/credits/pricing',
      '/admin/credits/health',
      '/admin/credits/reward-codes',
      '/admin/credits/payment-orders',
    ]) {
      expect(resolveAdminPageTitle(path)).toBe('积分运营')
    }
  })

  it('resolves the canonical title for other top-level pages', () => {
    expect(resolveAdminPageTitle('/admin/users')).toBe('用户管理')
    expect(resolveAdminPageTitle('/admin/task-monitor/detail')).toBe('任务监控')
  })

  it('falls back for unknown paths', () => {
    expect(resolveAdminPageTitle('/admin/unknown')).toBe('管理后台')
  })
})

describe('Admin system prompt route', () => {
  it('publishes the system prompt page at the canonical feature-prompts path', () => {
    const navigation = adminNavigationRouteDefs.find((item) => item.key === 'feature-prompts')
    const route = adminRoutes[0]?.children?.find((item) => item.path === 'feature-prompts')

    expect(navigation).toMatchObject({
      path: '/admin/feature-prompts',
      childPath: 'feature-prompts',
      label: '系统提示词',
      title: '系统提示词',
      routeName: 'AdminFeaturePrompts',
    })
    expect(route?.name).toBe('AdminFeaturePrompts')
  })
})

describe('Admin Credit Health routes', () => {
  it('publishes health as the only ledger integrity route', () => {
    const creditsRoute = adminRoutes[0]?.children?.find((route) => route.path === 'credits')
    const childPaths = creditsRoute?.children?.map((route) => route.path)

    expect(childPaths).toContain('health')
    expect(childPaths).not.toContain('reconciliation')
  })
})

describe('Admin Reward Code routes', () => {
  it('publishes only the remaining Credits routes', () => {
    const creditsRoute = adminRoutes[0]?.children?.find((route) => route.path === 'credits')
    const childPaths = creditsRoute?.children?.map((route) => route.path)
    const rewardCodeRoute = creditsRoute?.children?.find((route) => route.path === 'reward-codes')

    expect(creditsRoute?.redirect).toBeUndefined()
    expect(creditsRoute?.children?.[0]).toMatchObject({
      path: '',
      redirect: '/admin/credits/pricing',
    })
    expect(childPaths).toEqual(['', 'pricing', 'health', 'reward-codes', 'payment-orders'])
    expect(childPaths).not.toContain('wallets')
    expect(creditsRoute?.children?.some((route) => route.name === 'AdminCreditWallets')).toBe(false)
    expect(rewardCodeRoute).toMatchObject({
      path: 'reward-codes',
      name: 'AdminRewardCodes',
    })
    expect(rewardCodeRoute?.component).toEqual(expect.any(Function))
    expect(rewardCodeRoute?.redirect).toBeUndefined()
  })
})

describe('Admin Payment Order routes', () => {
  it('publishes the Payments feature route under Credits operations', () => {
    const creditsRoute = adminRoutes[0]?.children?.find((route) => route.path === 'credits')
    const paymentOrdersRoute = creditsRoute?.children?.find(
      (route) => route.path === 'payment-orders',
    )

    expect(paymentOrdersRoute).toMatchObject({
      path: 'payment-orders',
      name: 'AdminPaymentOrders',
      meta: { requiresAuth: true, title: '支付订单' },
    })
    expect(paymentOrdersRoute?.component).toEqual(expect.any(Function))
  })
})
