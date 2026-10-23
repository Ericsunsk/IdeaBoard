import { describe, expect, it } from 'vitest'
import { makeAdminUserListItem } from '@/test/factories'
import {
  createAdminUserRowView,
  getUserActionItems,
  getCountryFlag,
  getCountryLabel,
  getUserInitial,
  isActiveUser,
  isAdminUser,
  userRoleFilterOptions,
  userStatusFilterOptions,
} from './view'

describe('admin user view model', () => {
  it('marks admin and active users from role/status values', () => {
    const user = makeAdminUserListItem({ role: 'admin', status: 1 })

    expect(isAdminUser(user)).toBe(true)
    expect(isActiveUser(user)).toBe(true)
  })

  it('builds business role/status filter options without generated-query enums', () => {
    expect(userRoleFilterOptions.map((option) => option.value)).toEqual(['all', 'admin', 'user'])
    expect(userStatusFilterOptions.map((option) => option.value)).toEqual(['all', 1, 0])
  })

  it('maps backend user fields into row presentation fields', () => {
    const row = createAdminUserRowView(
      makeAdminUserListItem({
        available_credits: 125,
        role: 'admin',
        status: 0,
        public_id: '00000000-0000-0000-0000-000000000001',
        display_name: 'Alice',
        avatar: '/avatar.png',
        providers: ['google', 'discord'],
        created_at: '2026-06-29T10:20:30Z',
        last_login_ip: '203.0.113.42',
        last_login_country: 'us',
      }),
    )

    expect(row).toMatchObject({
      publicId: '00000000-0000-0000-0000-000000000001',
      email: 'user@example.com',
      lastLoginIP: '203.0.113.42',
      lastLoginCountry: 'us',
      providers: ['google', 'discord'],
      availableCredits: 125,
      availableCreditsLabel: '125 Ideas',
      avatarUrl: '/avatar.png',
      initial: 'A',
      isAdmin: true,
      isActive: false,
      roleLabel: 'Admin',
      roleTone: 'danger',
      statusLabel: '封禁',
      statusTone: 'danger',
    })
    expect(row.createdDateLabel).toBe('2026-06-29')
  })

  it('renders a flag for a valid country and a globe fallback for unknown country data', () => {
    expect(getCountryFlag('us')).toBe('🇺🇸')
    expect(getCountryFlag('XX')).toBe('')
    expect(getCountryFlag('')).toBe('')
    expect(getCountryLabel('cn')).toBe('登录国家 CN')
    expect(getCountryLabel('')).toBe('登录国家未知')
  })

  it('builds action items with destructive actions disabled for admins', () => {
    const items = getUserActionItems(makeCommandTarget({ role: 'admin' }))

    expect(items.find((item) => item.key === 'status')).toMatchObject({ disabled: true })
    expect(items.find((item) => item.key === 'delete')).toMatchObject({ disabled: true })
    expect(items.find((item) => item.key === 'role')).not.toHaveProperty('disabled')
  })

  it('keeps auth-bound actions available because the server resolves auth identity', () => {
    const items = getUserActionItems(makeCommandTarget())

    expect(items.find((item) => item.key === 'revoke')).not.toHaveProperty('disabled')
    expect(items.find((item) => item.key === 'impersonate')).not.toHaveProperty('disabled')
  })

  it('uses display name, public id, then fallback for initials', () => {
    expect(getUserInitial({ displayName: 'alice', publicId: 'u1' })).toBe('A')
    expect(getUserInitial({ displayName: '', publicId: 'b-user' })).toBe('B')
    expect(getUserInitial({ displayName: '', publicId: '' })).toBe('?')
  })
})

function makeCommandTarget(overrides: Partial<ReturnType<typeof createAdminUserRowView>> = {}) {
  return {
    ...createAdminUserRowView(makeAdminUserListItem()),
    ...overrides,
  }
}
