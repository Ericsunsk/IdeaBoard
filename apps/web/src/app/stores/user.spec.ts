import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from './user'
import { queryClient } from '../../shared/query/queryClient'
import type { User } from '../../shared/types'

function makeUser(overrides: Partial<User> = {}): User {
  const id = overrides.id ?? 1
  return {
    id,
    uid: overrides.uid ?? `UID${id}`,
    username: overrides.username ?? `user-${id}`,
    auth_user_id: overrides.auth_user_id ?? `auth-${id}`,
    avatar: overrides.avatar,
    role: overrides.role ?? 'user',
    status: overrides.status ?? 1,
    created_at: overrides.created_at ?? '2026-01-01T00:00:00.000Z',
    updated_at: overrides.updated_at ?? '2026-01-01T00:00:00.000Z',
  }
}

describe('useUserStore auth cache handling', () => {
  beforeEach(() => {
    queryClient.clear()
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('clears Vue Query cache when auth is reset', () => {
    const store = useUserStore()
    store.setAuth(makeUser())
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })

    store.clearAuth()

    expect(queryClient.getQueryData(['user', 'boards'])).toBeUndefined()
  })

  it('clears Vue Query cache when the active identity changes', () => {
    const store = useUserStore()
    store.setAuth(makeUser({ id: 1, uid: 'USER01', auth_user_id: 'auth-1' }))
    queryClient.setQueryData(['admin', 'users'], [{ id: 1 }])

    store.setAuth(makeUser({ id: 2, uid: 'USER02', auth_user_id: 'auth-2' }))

    expect(queryClient.getQueryData(['admin', 'users'])).toBeUndefined()
  })

  it('keeps Vue Query cache when the same identity is refreshed', () => {
    const store = useUserStore()
    store.setAuth(makeUser({ id: 1, uid: 'USER01', auth_user_id: 'auth-1' }))
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })

    store.setAuth(makeUser({ id: 1, uid: 'USER01', auth_user_id: 'auth-1', avatar: '/avatar.png' }))

    expect(queryClient.getQueryData(['user', 'boards'])).toEqual({ owner: 1 })
  })
})
