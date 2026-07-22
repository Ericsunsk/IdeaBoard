import { describe, expect, it } from 'vitest'
import { resolveAdminPageTitle } from './adminNavigation'
import { adminRoutes } from './adminRoutes'

describe('resolveAdminPageTitle', () => {
  it('uses the leaf route title for the pricing page', () => {
    expect(resolveAdminPageTitle('/admin/credits/pricing', '功能定价')).toBe('功能定价')
  })

  it('falls back to the parent navigation title', () => {
    expect(resolveAdminPageTitle('/admin/credits/wallets', undefined)).toBe('积分运营')
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
  it('publishes only the canonical reward code route', () => {
    const creditsRoute = adminRoutes[0]?.children?.find((route) => route.path === 'credits')
    const childPaths = creditsRoute?.children?.map((route) => route.path)
    const rewardCodeRoute = creditsRoute?.children?.find(
      (route) => route.path === 'reward-codes',
    )

    expect(childPaths).toEqual(['', 'wallets', 'pricing', 'health', 'reward-codes'])
    expect(rewardCodeRoute).toMatchObject({
      path: 'reward-codes',
      name: 'AdminRewardCodes',
    })
    expect(rewardCodeRoute?.component).toEqual(expect.any(Function))
    expect(rewardCodeRoute?.redirect).toBeUndefined()
  })
})
