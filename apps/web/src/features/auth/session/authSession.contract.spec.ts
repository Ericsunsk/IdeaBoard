import { beforeEach, describe, expect, it, vi } from 'vitest'
import { makeSessionUser } from '../../../test/factories'

async function getFreshUserStore() {
  vi.resetModules()
  const getUserInfo = vi.fn()
  const authClient = {
    signIn: {
      email: vi.fn(),
      social: vi.fn(),
    },
    signUp: {
      email: vi.fn(),
    },
    signOut: vi.fn(),
    getSession: vi.fn(),
  }

  vi.doMock('../../../shared/api/userSession', () => ({
    fetchCurrentUserInfo: getUserInfo,
  }))
  vi.doMock('../../../shared/auth/client', () => ({
    authClient,
    normalizeCanonicalEmail: (email: string) => email.trim().toLowerCase(),
    resolveBetterAuthErrorMessage: (_error: unknown, fallback: string) => fallback,
  }))

  const [{ useUserStore }, { queryClient }] = await Promise.all([
    import('./userSession'),
    import('../../../shared/query/queryClient'),
  ])
  return {
    store: useUserStore(),
    queryClient,
    getUserInfo,
    authClient,
  }
}

describe('useUserStore auth cache handling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('restores a stored session user on module creation', async () => {
    const storedUser = makeSessionUser({ ordinal: 7, role: 'admin' })
    localStorage.setItem('user', JSON.stringify(storedUser))

    const { store } = await getFreshUserStore()

    expect(store.user).toEqual(storedUser)
    expect(store.isAdmin).toBe(true)
    expect(store.sessionValidatedAt).toBe(0)
  })

  it('logs in and stores the fetched session user', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    const loggedInUser = makeSessionUser({ ordinal: 4, storage_namespace: 'USER04' })
    vi.mocked(authClient.signIn.email).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockResolvedValue({ user: loggedInUser } as never)

    await expect(store.login(' Alice@Example.com ', 'secret-password')).resolves.toBe(true)

    expect(authClient.signIn.email).toHaveBeenCalledWith({
      email: 'alice@example.com',
      password: 'secret-password',
    })
    expect(store.user).toEqual(loggedInUser)
    expect(localStorage.getItem('user')).toBe(JSON.stringify(loggedInUser))
    expect(store.sessionValidatedAt).toBeGreaterThan(0)
  })

  it('uses Email as the only password login identity', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    vi.mocked(authClient.signIn.email).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockResolvedValue({ user: makeSessionUser() } as never)

    await expect(store.login(' Alice@Example.com ', 'secret-password')).resolves.toBe(true)
    expect(authClient.signIn.email).toHaveBeenCalledWith({
      email: 'alice@example.com',
      password: 'secret-password',
    })
    expect(authClient.signIn).not.toHaveProperty('username')
  })

  it('clears the stale session when login projection remains transiently unavailable', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    const staleUser = makeSessionUser({ ordinal: 9, role: 'admin' })
    store.setAuth(staleUser)
    vi.mocked(authClient.signIn.email).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockRejectedValue({ status: 500 })

    await expect(store.login('alice@example.com', 'secret-password')).rejects.toThrow(
      'You are signed in, but we could not load your account.',
    )

    expect(vi.mocked(getUserInfo)).toHaveBeenCalledTimes(2)
    expect(store.user).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
  })

  it('throws sessionRestoreFailed after the login retry is exhausted with no cached user', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    vi.mocked(authClient.signIn.email).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockRejectedValue({ status: 500 })

    await expect(store.login('alice@example.com', 'secret-password')).rejects.toThrow(
      'You are signed in, but we could not load your account.',
    )

    expect(vi.mocked(getUserInfo)).toHaveBeenCalledTimes(2)
    expect(store.user).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
  })

  it('throws sessionRestoreFailed without retry when login projection is unauthorized', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    vi.mocked(authClient.signIn.email).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockRejectedValue({ status: 401 })

    await expect(store.login('alice@example.com', 'secret-password')).rejects.toThrow(
      'You are signed in, but we could not load your account.',
    )

    expect(vi.mocked(getUserInfo)).toHaveBeenCalledTimes(1)
    expect(store.user).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
  })

  it('clears auth state and Vue Query cache after logout succeeds', async () => {
    const { store, queryClient, authClient } = await getFreshUserStore()
    store.setAuth(makeSessionUser())
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })
    vi.mocked(authClient.signOut).mockResolvedValue({} as never)

    await expect(store.logout()).resolves.toBe(true)

    expect(store.user).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(queryClient.getQueryData(['user', 'boards'])).toBeUndefined()
  })

  it('clears Vue Query cache when auth is reset', async () => {
    const { store, queryClient } = await getFreshUserStore()
    store.setAuth(makeSessionUser())
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })

    store.clearAuth()

    expect(queryClient.getQueryData(['user', 'boards'])).toBeUndefined()
  })

  it('clears Vue Query cache when the active identity changes', async () => {
    const { store, queryClient } = await getFreshUserStore()
    store.setAuth(
      makeSessionUser({ ordinal: 1, public_id: '00000000-0000-4000-8000-000000000001' }),
    )
    queryClient.setQueryData(['admin', 'users'], [{ count: 1 }])

    store.setAuth(
      makeSessionUser({ ordinal: 2, public_id: '00000000-0000-4000-8000-000000000002' }),
    )

    expect(queryClient.getQueryData(['admin', 'users'])).toBeUndefined()
  })

  it('keeps Vue Query cache when the same identity is refreshed', async () => {
    const { store, queryClient } = await getFreshUserStore()
    store.setAuth(
      makeSessionUser({ ordinal: 1, public_id: '00000000-0000-4000-8000-000000000001' }),
    )
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })

    store.setAuth(
      makeSessionUser({
        ordinal: 1,
        public_id: '00000000-0000-4000-8000-000000000001',
        avatar: '/avatar.png',
      }),
    )

    expect(queryClient.getQueryData(['user', 'boards'])).toEqual({ owner: 1 })
  })

  it('reuses a recently validated session without probing the server', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    const currentUser = makeSessionUser({ ordinal: 5 })
    store.setAuth(currentUser)

    await expect(store.ensureSession()).resolves.toEqual(currentUser)

    expect(authClient.getSession).not.toHaveBeenCalled()
    expect(getUserInfo).not.toHaveBeenCalled()
  })

  it('keeps the stale session for fresh probes on server error (fail open, retry next nav)', async () => {
    const { store, queryClient, getUserInfo, authClient } = await getFreshUserStore()
    const staleUser = makeSessionUser({ ordinal: 1, role: 'admin' })
    store.setAuth(staleUser)
    queryClient.setQueryData(['admin', 'users'], [{ count: 1 }])
    vi.mocked(authClient.getSession).mockResolvedValue({
      data: { session: { id: 'session-1' } },
    } as never)
    vi.mocked(getUserInfo).mockRejectedValue({ status: 500 })

    await expect(store.ensureSession({ fresh: true })).resolves.toEqual(staleUser)

    expect(store.user).toEqual(staleUser)
    expect(localStorage.getItem('user')).toBe(JSON.stringify(staleUser))
    expect(queryClient.getQueryData(['admin', 'users'])).toEqual([{ count: 1 }])
  })

  it('registers and stores the created session user', async () => {
    const { store, authClient } = await getFreshUserStore()
    const registeredUser = makeSessionUser({ ordinal: 2, storage_namespace: 'USER02' })
    vi.mocked(authClient.signUp.email).mockResolvedValue({
      data: { user: registeredUser },
    } as never)

    await store.register(' Alice@Example.com ', 'alice', 'secret-password')

    expect(authClient.signUp.email).toHaveBeenCalledWith({
      email: 'alice@example.com',
      name: 'alice',
      password: 'secret-password',
    })
    expect(store.user).toBeNull()
  })

  it('does not create a session before Email verification', async () => {
    const { store, authClient } = await getFreshUserStore()
    vi.mocked(authClient.signUp.email).mockResolvedValue({
      data: { user: { id: 'auth-bob' } },
    } as never)

    await expect(store.register('bob@example.com', 'bob', 'secret-password')).resolves.toBe(true)
    expect(authClient.signIn.email).not.toHaveBeenCalled()
  })
})
