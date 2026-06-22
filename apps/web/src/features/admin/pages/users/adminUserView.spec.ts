import { describe, expect, it } from 'vitest'
import type { User } from '../../../../shared/types'
import { getUserActionItems, getUserInitial, isActiveUser, isAdminUser } from './adminUserView'

describe('admin user presentation helpers', () => {
  it('marks admin and active users from generated role/status values', () => {
    const user = makeUser({ role: 'admin', status: 1 })

    expect(isAdminUser(user)).toBe(true)
    expect(isActiveUser(user)).toBe(true)
  })

  it('builds action items with destructive actions disabled for admins', () => {
    const items = getUserActionItems(makeUser({ role: 'admin', auth_user_id: 'auth-1' }))

    expect(items.find((item) => item.key === 'status')).toMatchObject({ disabled: true })
    expect(items.find((item) => item.key === 'delete')).toMatchObject({ disabled: true })
    expect(items.find((item) => item.key === 'role')).not.toHaveProperty('disabled')
  })

  it('disables auth-bound actions when auth identity is missing', () => {
    const items = getUserActionItems(makeUser({ auth_user_id: '' }))

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
    ...overrides,
  } as User
}
