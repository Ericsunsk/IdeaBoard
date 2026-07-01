import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from './user'
import { queryClient } from '../../shared/query/queryClient'
import type { User } from '../../shared/api/generated-query/model'
import { getUserInfo } from '../../shared/api/generated-query/user/user'
import { authClient } from '../../shared/auth/client'

vi.mock('../../shared/api/generated-query/user/user', () => ({
  getUserInfo: vi.fn(),
}))

vi.mock('../../shared/auth/client', () => ({
  authClient: {
    signIn: {
      username: vi.fn(),
    },
    signUp: {
      email: vi.fn(),
    },
    signOut: vi.fn(),
    getSession: vi.fn(),
  },
  extractBetterAuthErrorMessage: (error: unknown, fallback: string) =>
    error && typeof error === 'object' && 'message' in error
      ? String((error as { message?: unknown }).message || fallback)
      : fallback,
}))

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
    vi.clearAllMocks()
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

  it('fails closed for fresh session probes when user info returns a server error', async () => {
    const store = useUserStore()
    store.setAuth(makeUser({ id: 1, role: 'admin' }))
    queryClient.setQueryData(['admin', 'users'], [{ id: 1 }])
    vi.mocked(authClient.getSession).mockResolvedValue({
      data: { session: { id: 'session-1' } },
    } as never)
    vi.mocked(getUserInfo).mockRejectedValue({ status: 500 })

    await expect(store.ensureSession({ fresh: true })).resolves.toBeNull()

    expect(store.user).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(queryClient.getQueryData(['admin', 'users'])).toBeUndefined()
  })

  it('registers and stores the created session user', async () => {
    const store = useUserStore()
    const registeredUser = makeUser({ id: 2, uid: 'USER02', username: 'alice' })
    vi.mocked(authClient.signUp.email).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { user: registeredUser },
    } as never)

    await store.register(' alice ', 'secret-password')

    expect(authClient.signUp.email).toHaveBeenCalledWith({
      email: 'alice@ideaboard.local',
      password: 'secret-password',
      name: 'alice',
      username: 'alice',
    })
    expect(authClient.signIn.username).not.toHaveBeenCalled()
    expect(store.user).toEqual(registeredUser)
  })

  it('falls back to username login when sign up does not restore user info', async () => {
    const store = useUserStore()
    const registeredUser = makeUser({ id: 3, uid: 'USER03', username: 'bob' })
    vi.mocked(authClient.signUp.email).mockResolvedValue({} as never)
    vi.mocked(authClient.signIn.username).mockResolvedValue({} as never)
    vi.mocked(getUserInfo)
      .mockResolvedValueOnce({ code: 0, msg: 'ok', data: {} } as never)
      .mockResolvedValueOnce({
        code: 0,
        msg: 'ok',
        data: { user: registeredUser },
      } as never)

    await store.register('bob', 'secret-password')

    expect(authClient.signIn.username).toHaveBeenCalledWith({
      username: 'bob',
      password: 'secret-password',
    })
    expect(store.user).toEqual(registeredUser)
  })
})
