import { describe, expect, it } from 'vitest'
import { resolveAdminPageTitle } from './adminNavigation'

describe('resolveAdminPageTitle', () => {
  it('uses the leaf route title for the pricing page', () => {
    expect(resolveAdminPageTitle('/admin/credits/pricing', '功能定价')).toBe('功能定价')
  })

  it('falls back to the parent navigation title', () => {
    expect(resolveAdminPageTitle('/admin/credits/wallets', undefined)).toBe('积分运营')
  })
})
