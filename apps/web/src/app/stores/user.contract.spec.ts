import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../../shared/api/userSession'

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

async function getFreshUserStore() {
  vi.resetModules()
  const getUserInfo = vi.fn()
  const registerUsername = vi.fn()
  const authClient = {
    signIn: {
      username: vi.fn(),
    },
    signOut: vi.fn(),
    getSession: vi.fn(),
  }

  vi.doMock('../../shared/api/userSession', () => ({
    fetchCurrentUserInfo: getUserInfo,
  }))
  vi.doMock('../../shared/auth/client', () => ({
    authClient,
    registerUsername,
    extractBetterAuthErrorMessage: (error: unknown, fallback: string) =>
      error && typeof error === 'object' && 'message' in error
        ? String((error as { message?: unknown }).message || fallback)
        : fallback,
  }))

  const [{ useUserStore }, { queryClient }] = await Promise.all([
    import('./user'),
    import('../../shared/query/queryClient'),
  ])
  return {
    store: useUserStore(),
    queryClient,
    getUserInfo,
    authClient,
    registerUsername,
  }
}

describe('useUserStore auth cache handling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('restores a stored session user on module creation', async () => {
    const storedUser = makeUser({ id: 7, role: 'admin' })
    localStorage.setItem('user', JSON.stringify(storedUser))

    const { store } = await getFreshUserStore()

    expect(store.user).toEqual(storedUser)
    expect(store.isAdmin).toBe(true)
    expect(store.sessionValidatedAt).toBe(0)
  })

  it('logs in and stores the fetched session user', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    const loggedInUser = makeUser({ id: 4, uid: 'USER04' })
    vi.mocked(authClient.signIn.username).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { user: loggedInUser },
    } as never)

    await expect(store.login('alice', 'secret-password')).resolves.toBe(true)

    expect(authClient.signIn.username).toHaveBeenCalledWith({
      username: 'alice',
      password: 'secret-password',
    })
    expect(store.user).toEqual(loggedInUser)
    expect(localStorage.getItem('user')).toBe(JSON.stringify(loggedInUser))
    expect(store.sessionValidatedAt).toBeGreaterThan(0)
  })

  it('clears auth state and Vue Query cache after logout succeeds', async () => {
    const { store, queryClient, authClient } = await getFreshUserStore()
    store.setAuth(makeUser())
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })
    vi.mocked(authClient.signOut).mockResolvedValue({} as never)

    await expect(store.logout()).resolves.toBe(true)

    expect(store.user).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(queryClient.getQueryData(['user', 'boards'])).toBeUndefined()
  })

  it('clears Vue Query cache when auth is reset', async () => {
    const { store, queryClient } = await getFreshUserStore()
    store.setAuth(makeUser())
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })

    store.clearAuth()

    expect(queryClient.getQueryData(['user', 'boards'])).toBeUndefined()
  })

  it('clears Vue Query cache when the active identity changes', async () => {
    const { store, queryClient } = await getFreshUserStore()
    store.setAuth(makeUser({ id: 1, uid: 'USER01', auth_user_id: 'auth-1' }))
    queryClient.setQueryData(['admin', 'users'], [{ id: 1 }])

    store.setAuth(makeUser({ id: 2, uid: 'USER02', auth_user_id: 'auth-2' }))

    expect(queryClient.getQueryData(['admin', 'users'])).toBeUndefined()
  })

  it('keeps Vue Query cache when the same identity is refreshed', async () => {
    const { store, queryClient } = await getFreshUserStore()
    store.setAuth(makeUser({ id: 1, uid: 'USER01', auth_user_id: 'auth-1' }))
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })

    store.setAuth(makeUser({ id: 1, uid: 'USER01', auth_user_id: 'auth-1', avatar: '/avatar.png' }))

    expect(queryClient.getQueryData(['user', 'boards'])).toEqual({ owner: 1 })
  })

  it('reuses a recently validated session without probing the server', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    const currentUser = makeUser({ id: 5 })
    store.setAuth(currentUser)

    await expect(store.ensureSession()).resolves.toEqual(currentUser)

    expect(authClient.getSession).not.toHaveBeenCalled()
    expect(getUserInfo).not.toHaveBeenCalled()
  })

  it('fails closed for fresh session probes when user info returns a server error', async () => {
    const { store, queryClient, getUserInfo, authClient } = await getFreshUserStore()
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
    const { store, getUserInfo, authClient, registerUsername } = await getFreshUserStore()
    const registeredUser = makeUser({ id: 2, uid: 'USER02', username: 'alice' })
    registerUsername.mockResolvedValue('alice')
    vi.mocked(authClient.signIn.username).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { user: registeredUser },
    } as never)

    await store.register(' alice ', 'secret-password')

    expect(registerUsername).toHaveBeenCalledWith('alice', 'secret-password')
    expect(authClient.signIn.username).toHaveBeenCalledWith({
      username: 'alice',
      password: 'secret-password',
    })
    expect(store.user).toEqual(registeredUser)
  })

  it('fails when username registration login cannot restore user info', async () => {
    const { store, getUserInfo, authClient, registerUsername } = await getFreshUserStore()
    registerUsername.mockResolvedValue('bob')
    vi.mocked(authClient.signIn.username).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockResolvedValue({ code: 0, msg: 'ok', data: {} } as never)

    await expect(store.register('bob', 'secret-password')).rejects.toThrow('登录状态恢复失败')

    expect(authClient.signIn.username).toHaveBeenCalledWith({
      username: 'bob',
      password: 'secret-password',
    })
    expect(store.user).toBeNull()
  })
})
