import { describe, expect, it } from 'vitest'
import type { AdminUserListItem } from '../../../shared/api/generated-query/model'
import {
  createAdminUserRowView,
  getUserActionItems,
  getUserInitial,
  isActiveUser,
  isAdminUser,
  userRoleFilterOptions,
  userStatusFilterOptions,
} from './adminUserView'

describe('admin user view model', () => {
  it('marks admin and active users from role/status values', () => {
    const user = makeUser({ role: 'admin', status: 1 })

    expect(isAdminUser(user)).toBe(true)
    expect(isActiveUser(user)).toBe(true)
  })

  it('builds business role/status filter options without generated-query enums', () => {
    expect(userRoleFilterOptions.map((option) => option.value)).toEqual(['all', 'admin', 'user'])
    expect(userStatusFilterOptions.map((option) => option.value)).toEqual(['all', 1, 0])
  })

  it('maps backend user fields into row presentation fields', () => {
    const row = createAdminUserRowView(
      makeUser({
        available_credits: 125,
        role: 'admin',
        status: 0,
        public_id: '00000000-0000-0000-0000-000000000001',
        display_name: 'Alice',
        avatar: '/avatar.png',
        created_at: '2026-06-29T10:20:30Z',
      }),
    )

    expect(row).toMatchObject({
      publicId: '00000000-0000-0000-0000-000000000001',
      email: 'user@example.com',
      availableCredits: 125,
      availableCreditsLabel: '125 Ideas',
      avatarUrl: '/avatar.png',
      initial: 'A',
      isAdmin: true,
      isActive: false,
      roleLabel: 'Admin',
      roleSeverity: 'danger',
      statusLabel: '封禁',
      statusSeverity: 'danger',
    })
    expect(row.createdDateLabel).toBe('2026-06-29')
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

function makeUser(overrides: Partial<AdminUserListItem> = {}): AdminUserListItem {
  return {
    public_id: '00000000-0000-0000-0000-000000000001',
    email: 'user@example.com',
    display_name: 'user',
    role: 'user',
    status: 1,
    available_credits: 0,
    avatar: '',
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

function makeCommandTarget(overrides: Partial<ReturnType<typeof createAdminUserRowView>> = {}) {
  return {
    ...createAdminUserRowView(makeUser()),
    ...overrides,
  }
}
