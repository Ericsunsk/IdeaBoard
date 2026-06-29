import { describe, expect, it } from 'vitest'
import type { User } from '../../../shared/types'
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
        role: 'admin',
        status: 0,
        auth_user_id: 'auth-1',
        avatar: '/avatar.png',
        created_at: '2026-06-29T10:20:30Z',
      }),
    )

    expect(row).toMatchObject({
      authUserId: 'auth-1',
      authUserIdLabel: 'auth-1',
      avatarUrl: '/avatar.png',
      initial: 'U',
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
    const items = getUserActionItems(makeCommandTarget({ role: 'admin', authUserId: 'auth-1' }))

    expect(items.find((item) => item.key === 'status')).toMatchObject({ disabled: true })
    expect(items.find((item) => item.key === 'delete')).toMatchObject({ disabled: true })
    expect(items.find((item) => item.key === 'role')).not.toHaveProperty('disabled')
  })

  it('disables auth-bound actions when auth identity is missing', () => {
    const items = getUserActionItems(makeCommandTarget({ authUserId: '' }))

    expect(items.find((item) => item.key === 'revoke')).toMatchObject({ disabled: true })
    expect(items.find((item) => item.key === 'impersonate')).toMatchObject({ disabled: true })
  })

  it('uses username, uid, then fallback for initials', () => {
    expect(getUserInitial(makeUser({ username: 'alice', uid: 'u1' }))).toBe('A')
    expect(getUserInitial(makeUser({ username: '', uid: 'b-user' }))).toBe('B')
    expect(getUserInitial(makeUser({ username: '', uid: '' }))).toBe('?')
  })
})

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 1,
    uid: 'user-1',
    username: 'user',
    role: 'user',
    status: 1,
    avatar: '',
    auth_user_id: '',
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
