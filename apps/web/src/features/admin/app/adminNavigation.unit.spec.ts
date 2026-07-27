import { describe, expect, it } from 'vitest'
import { resolveAdminPageTitle } from './adminNavigation'
import { adminRoutes } from './adminRoutes'
import { adminNavigationRouteDefs } from './adminNavigationMeta'

describe('resolveAdminPageTitle', () => {
  it('uses the leaf route title for the pricing page', () => {
    expect(resolveAdminPageTitle('/admin/credits/pricing', '功能定价')).toBe('功能定价')
  })

  it('falls back to the parent navigation title for a remaining credits route', () => {
    expect(resolveAdminPageTitle('/admin/credits/pricing', undefined)).toBe('积分运营')
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
    const rewardCodeRoute = creditsRoute?.children?.find(
      (route) => route.path === 'reward-codes',
    )

    expect(creditsRoute?.redirect).toBeUndefined()
    expect(creditsRoute?.children?.[0]).toMatchObject({
      path: '',
      redirect: '/admin/credits/pricing',
    })
    expect(childPaths).toEqual(['', 'pricing', 'health', 'reward-codes'])
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
