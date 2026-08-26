import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../../../shared/api/userSession'

type TestUserOverrides = Partial<User> & {
  ordinal?: number
}

function makeUser(overrides: TestUserOverrides = {}): User {
  const ordinal = overrides.ordinal ?? 1
  return {
    public_id:
      overrides.public_id ?? `00000000-0000-4000-8000-${String(ordinal).padStart(12, '0')}`,
    display_name: overrides.display_name ?? `User ${ordinal}`,
    storage_namespace: overrides.storage_namespace ?? `USER${String(ordinal).padStart(4, '0')}`,
    avatar: overrides.avatar ?? '',
    role: overrides.role ?? 'user',
    status: overrides.status ?? 1,
    created_at: overrides.created_at ?? '2026-01-01T00:00:00.000Z',
    updated_at: overrides.updated_at ?? '2026-01-01T00:00:00.000Z',
  }
}

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
    import('./authSession'),
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
    const storedUser = makeUser({ ordinal: 7, role: 'admin' })
    localStorage.setItem('user', JSON.stringify(storedUser))

    const { store } = await getFreshUserStore()

    expect(store.user).toEqual(storedUser)
    expect(store.isAdmin).toBe(true)
    expect(store.sessionValidatedAt).toBe(0)
  })

  it('logs in and stores the fetched session user', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    const loggedInUser = makeUser({ ordinal: 4, storage_namespace: 'USER04' })
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
    vi.mocked(getUserInfo).mockResolvedValue({ user: makeUser() } as never)

    await expect(store.login(' Alice@Example.com ', 'secret-password')).resolves.toBe(true)
    expect(authClient.signIn.email).toHaveBeenCalledWith({
      email: 'alice@example.com',
      password: 'secret-password',
    })
    expect(authClient.signIn).not.toHaveProperty('username')
  })

  it('fails closed after login when the business identity projection cannot refresh', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    store.setAuth(makeUser({ ordinal: 9, role: 'admin' }))
    vi.mocked(authClient.signIn.email).mockResolvedValue({} as never)
    vi.mocked(getUserInfo).mockRejectedValue({ status: 500 })

    await expect(store.login('alice@example.com', 'secret-password')).rejects.toThrow(
      'You are signed in, but we could not load your account.',
    )
    expect(store.user).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
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
    store.setAuth(makeUser({ ordinal: 1, public_id: '00000000-0000-4000-8000-000000000001' }))
    queryClient.setQueryData(['admin', 'users'], [{ count: 1 }])

    store.setAuth(makeUser({ ordinal: 2, public_id: '00000000-0000-4000-8000-000000000002' }))

    expect(queryClient.getQueryData(['admin', 'users'])).toBeUndefined()
  })

  it('keeps Vue Query cache when the same identity is refreshed', async () => {
    const { store, queryClient } = await getFreshUserStore()
    store.setAuth(makeUser({ ordinal: 1, public_id: '00000000-0000-4000-8000-000000000001' }))
    queryClient.setQueryData(['user', 'boards'], { owner: 1 })

    store.setAuth(
      makeUser({
        ordinal: 1,
        public_id: '00000000-0000-4000-8000-000000000001',
        avatar: '/avatar.png',
      }),
    )

    expect(queryClient.getQueryData(['user', 'boards'])).toEqual({ owner: 1 })
  })

  it('reuses a recently validated session without probing the server', async () => {
    const { store, getUserInfo, authClient } = await getFreshUserStore()
    const currentUser = makeUser({ ordinal: 5 })
    store.setAuth(currentUser)

    await expect(store.ensureSession()).resolves.toEqual(currentUser)

    expect(authClient.getSession).not.toHaveBeenCalled()
    expect(getUserInfo).not.toHaveBeenCalled()
  })

  it('fails closed for fresh session probes when user info returns a server error', async () => {
    const { store, queryClient, getUserInfo, authClient } = await getFreshUserStore()
    store.setAuth(makeUser({ ordinal: 1, role: 'admin' }))
    queryClient.setQueryData(['admin', 'users'], [{ count: 1 }])
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
    const { store, authClient } = await getFreshUserStore()
    const registeredUser = makeUser({ ordinal: 2, storage_namespace: 'USER02' })
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
