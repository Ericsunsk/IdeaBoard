import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../../../api/sessionUserType'

const mocks = vi.hoisted(() => ({
  clearAuthScopedQueryCache: vi.fn(),
  clearPersistedSession: vi.fn(),
  persistSessionUser: vi.fn(),
  readStoredSessionUser: vi.fn(),
}))

vi.mock('../../../core/query/queryClient', () => ({
  clearAuthScopedQueryCache: mocks.clearAuthScopedQueryCache,
}))
vi.mock('./sessionPersistence', () => ({
  clearPersistedSession: mocks.clearPersistedSession,
  persistSessionUser: mocks.persistSessionUser,
  readStoredSessionUser: mocks.readStoredSessionUser,
}))

import { createBrowserSessionTransition } from './browserSessionTransition'
import { SESSION_TRUST_WINDOW_MS } from './freshness'
import { makeSessionUser } from '../../../test/factories'

function createHarness(
  overrides: Partial<{
    user: User | null
    isLoaded: boolean
    validatedAt: number
  }> = {},
  nowSource: number | (() => number) = 1_001,
) {
  let user = overrides.user ?? null
  let loaded = overrides.isLoaded ?? false
  let validatedAt = overrides.validatedAt ?? 0
  const getNow = typeof nowSource === 'function' ? nowSource : () => nowSource
  const transport = {
    hasSession: vi.fn().mockResolvedValue(true),
    logout: vi.fn().mockResolvedValue(true),
    stopImpersonating: vi.fn().mockResolvedValue(undefined),
  }
  const loadUser = vi.fn()
  const state = {
    getUser: () => user,
    isLoaded: () => loaded,
    getValidatedAt: () => validatedAt,
    commit: vi.fn((nextUser: User) => {
      user = nextUser
      loaded = true
      validatedAt = getNow()
    }),
    restore: vi.fn((storedUser: User) => {
      user = storedUser
    }),
    markUnloaded: vi.fn(() => {
      loaded = false
    }),
    clear: vi.fn(() => {
      user = null
      loaded = false
      validatedAt = 0
    }),
  }
  const transition = createBrowserSessionTransition(state, {
    transport,
    loadUser,
    now: getNow,
  })
  return { transition, transport, loadUser, state, getUser: () => user }
}

describe('browser session transition', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.readStoredSessionUser.mockReturnValue(null)
  })

  it('commits a refreshed identity and clears auth-scoped cache only when it changes', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
      validatedAt: 0,
    })
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-2' }) })

    await expect(harness.transition.fetchUserInfo(true)).resolves.toEqual(
      makeSessionUser({ public_id: 'user-2' }),
    )

    expect(harness.state.commit).toHaveBeenCalledWith(makeSessionUser({ public_id: 'user-2' }))
    expect(mocks.persistSessionUser).toHaveBeenCalledWith(makeSessionUser({ public_id: 'user-2' }))
    expect(mocks.clearAuthScopedQueryCache).toHaveBeenCalledOnce()
  })

  it('discards a stale in-flight fetch that resolves after an identity switch', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-2' }),
      isLoaded: true,
    })
    let resolveStale!: (value: { user: User }) => void
    harness.loadUser.mockReturnValueOnce(
      new Promise<{ user: User }>((resolve) => {
        resolveStale = resolve
      }),
    )

    const pending = harness.transition.fetchUserInfo(true)
    harness.transition.setAuth(makeSessionUser({ public_id: 'user-3' }))
    resolveStale({ user: makeSessionUser({ public_id: 'user-2' }) })

    await expect(pending).resolves.toEqual(makeSessionUser({ public_id: 'user-3' }))
    expect(harness.getUser()?.public_id).toBe('user-3')
    expect(mocks.persistSessionUser).toHaveBeenCalledTimes(1)
    expect(mocks.persistSessionUser).toHaveBeenCalledWith(makeSessionUser({ public_id: 'user-3' }))
  })

  it('ignores a stale 401 from a previous identity epoch instead of clearing the new identity', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-2' }),
      isLoaded: true,
    })
    let rejectStale!: (reason: unknown) => void
    harness.loadUser.mockReturnValueOnce(
      new Promise<{ user: User }>((_resolve, reject) => {
        rejectStale = reject
      }),
    )

    const pending = harness.transition.fetchUserInfo(true)
    harness.transition.setAuth(makeSessionUser({ public_id: 'user-3' }))
    rejectStale({ status: 401 })

    await expect(pending).resolves.toEqual(makeSessionUser({ public_id: 'user-3' }))
    expect(harness.getUser()?.public_id).toBe('user-3')
    expect(harness.state.clear).not.toHaveBeenCalled()
    expect(mocks.clearPersistedSession).not.toHaveBeenCalled()
  })

  it('clears local state when a fresh user projection is rejected as unauthorized', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.loadUser.mockRejectedValue({ status: 401 })

    await expect(harness.transition.fetchUserInfo(true)).resolves.toBeNull()

    expect(harness.getUser()).toBeNull()
    expect(harness.state.clear).toHaveBeenCalledOnce()
    expect(mocks.clearPersistedSession).toHaveBeenCalledOnce()
    expect(mocks.clearAuthScopedQueryCache).toHaveBeenCalledOnce()
  })

  it('keeps the stale user and marks unloaded on 500 so the next nav retries', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.loadUser.mockRejectedValue({ status: 500 })

    await expect(harness.transition.fetchUserInfo(true)).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )

    expect(harness.getUser()).toEqual(makeSessionUser({ public_id: 'user-1' }))
    expect(harness.state.clear).not.toHaveBeenCalled()
    expect(harness.state.markUnloaded).toHaveBeenCalledOnce()
    expect(mocks.clearPersistedSession).not.toHaveBeenCalled()
    expect(mocks.clearAuthScopedQueryCache).not.toHaveBeenCalled()
  })

  it('keeps the stale user and marks unloaded on timeouts without a status', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.loadUser.mockRejectedValue(new Error('upstream unavailable'))

    await expect(harness.transition.fetchUserInfo(true)).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )

    expect(harness.getUser()).toEqual(makeSessionUser({ public_id: 'user-1' }))
    expect(harness.state.clear).not.toHaveBeenCalled()
    expect(harness.state.markUnloaded).toHaveBeenCalledOnce()
  })

  it('keeps the stale user for fresh ensureSession probes on server errors', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
      validatedAt: 0,
    })
    harness.loadUser.mockRejectedValue({ status: 503 })

    await expect(harness.transition.ensureSession({ fresh: true })).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )

    expect(harness.getUser()).toEqual(makeSessionUser({ public_id: 'user-1' }))
    expect(harness.state.clear).not.toHaveBeenCalled()
    expect(harness.state.markUnloaded).toHaveBeenCalledOnce()
  })

  it('retries a transient projection failure once when requested, then commits', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.loadUser
      .mockRejectedValueOnce({ status: 500 })
      .mockResolvedValueOnce({ user: makeSessionUser({ public_id: 'user-2' }) })

    await expect(harness.transition.fetchUserInfo(true, { retryTransient: true })).resolves.toEqual(
      makeSessionUser({ public_id: 'user-2' }),
    )

    expect(harness.loadUser).toHaveBeenCalledTimes(2)
    expect(harness.state.commit).toHaveBeenCalledWith(makeSessionUser({ public_id: 'user-2' }))
  })

  it('clears the stale identity after a post-auth retry is exhausted', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.loadUser.mockRejectedValue({ status: 500 })

    await expect(
      harness.transition.fetchUserInfo(true, { retryTransient: true }),
    ).resolves.toBeNull()

    expect(harness.loadUser).toHaveBeenCalledTimes(2)
    expect(harness.getUser()).toBeNull()
    expect(harness.state.clear).toHaveBeenCalledOnce()
    expect(harness.state.markUnloaded).not.toHaveBeenCalled()
  })

  it('does not retry when the projection is rejected as unauthorized', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.loadUser.mockRejectedValue({ status: 403 })

    await expect(
      harness.transition.fetchUserInfo(true, { retryTransient: true }),
    ).resolves.toBeNull()

    expect(harness.loadUser).toHaveBeenCalledTimes(1)
    expect(harness.getUser()).toBeNull()
  })

  it('normalizes unsafe OAuth redirects and clears state on callback failure', async () => {
    const harness = createHarness()
    const replace = vi.fn().mockResolvedValue(undefined)

    await expect(
      harness.transition.completeOAuthCallback({
        providerError: 'access_denied:raw-provider-detail',
        redirectPath: 'https://attacker.example/steal',
        replace,
      }),
    ).resolves.toBe(false)

    expect(harness.transport.logout).toHaveBeenCalledOnce()
    expect(replace).toHaveBeenCalledWith({ path: '/user/auth/callback', query: {} })
    expect(replace).not.toHaveBeenCalledWith(expect.stringContaining('attacker.example'))
  })

  it('falls back to User Boards after a successful callback with an unsafe redirect', async () => {
    const harness = createHarness()
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-1' }) })
    const replace = vi.fn().mockResolvedValue(undefined)

    await expect(
      harness.transition.completeOAuthCallback({
        redirectPath: 'https://attacker.example/steal',
        replace,
      }),
    ).resolves.toBe(true)

    expect(replace).toHaveBeenCalledWith('/user/boards')
  })

  it('does not clear local state when logout fails', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })
    harness.transport.logout.mockResolvedValue(false)

    await expect(harness.transition.logout()).resolves.toBe(false)

    expect(harness.state.clear).not.toHaveBeenCalled()
    expect(harness.getUser()).toEqual(makeSessionUser({ public_id: 'user-1' }))
  })

  it('reprobes when the session was validated exactly one trust window ago', async () => {
    const now = 1_000_000
    const harness = createHarness(
      {
        user: makeSessionUser({ public_id: 'user-1' }),
        isLoaded: true,
        validatedAt: now - SESSION_TRUST_WINDOW_MS,
      },
      now,
    )
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-1' }) })

    await expect(harness.transition.ensureSession()).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )

    expect(harness.transport.hasSession).toHaveBeenCalledOnce()
    expect(harness.loadUser).toHaveBeenCalledOnce()
  })

  it('reuses the cache one millisecond inside the trust window', async () => {
    const now = 1_000_000
    const harness = createHarness(
      {
        user: makeSessionUser({ public_id: 'user-1' }),
        isLoaded: true,
        validatedAt: now - SESSION_TRUST_WINDOW_MS + 1,
      },
      now,
    )

    await expect(harness.transition.ensureSession()).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )

    expect(harness.transport.hasSession).not.toHaveBeenCalled()
    expect(harness.loadUser).not.toHaveBeenCalled()
  })

  it('reprobes an unverified session instead of trusting it', async () => {
    const harness = createHarness(
      { user: makeSessionUser({ public_id: 'user-1' }), isLoaded: true, validatedAt: 0 },
      1_000_000,
    )
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-1' }) })

    await expect(harness.transition.ensureSession()).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )

    expect(harness.loadUser).toHaveBeenCalledOnce()
  })

  it('bypasses a fresh cache when fresh is requested', async () => {
    const now = 1_000_000
    const harness = createHarness(
      {
        user: makeSessionUser({ public_id: 'user-1' }),
        isLoaded: true,
        validatedAt: now - 60_000,
      },
      now,
    )
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-1' }) })

    await expect(harness.transition.ensureSession({ fresh: true })).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )

    expect(harness.loadUser).toHaveBeenCalledOnce()
  })

  it('reuses the cache across navigations until the trust window lapses', async () => {
    const clock = { now: 1_000_000 }
    const harness = createHarness(
      { user: makeSessionUser({ public_id: 'user-1' }), isLoaded: true, validatedAt: 0 },
      () => clock.now,
    )
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-1' }) })

    await expect(harness.transition.ensureSession()).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )
    expect(harness.loadUser).toHaveBeenCalledTimes(1)

    clock.now += 60_000
    await expect(harness.transition.ensureSession()).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )
    expect(harness.loadUser).toHaveBeenCalledTimes(1)

    clock.now += SESSION_TRUST_WINDOW_MS
    await expect(harness.transition.ensureSession()).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )
    expect(harness.loadUser).toHaveBeenCalledTimes(2)
  })

  it('clears stale avatars when the server no longer has one', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1', avatar: '/uploads/profile/U1/old.png' }),
      isLoaded: true,
    })
    mocks.readStoredSessionUser.mockReturnValue(
      makeSessionUser({ public_id: 'user-1', avatar: '/uploads/profile/U1/old.png' }),
    )
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-1' }) })

    await expect(harness.transition.fetchUserInfo(true)).resolves.toEqual(
      makeSessionUser({ public_id: 'user-1' }),
    )
    expect(mocks.persistSessionUser).toHaveBeenCalledWith(makeSessionUser({ public_id: 'user-1' }))
  })

  it('loads the server avatar on a cold start without a stored snapshot', async () => {
    const harness = createHarness()
    const serverUser = makeSessionUser({
      public_id: 'user-1',
      avatar: '/uploads/profile/U1/avatar.png',
    })
    harness.loadUser.mockResolvedValue({ user: serverUser })

    await expect(harness.transition.ensureSession()).resolves.toEqual(serverUser)
    expect(mocks.persistSessionUser).toHaveBeenCalledWith(serverUser)
  })

  it('recovers the server avatar after logout and login', async () => {
    const serverUser = makeSessionUser({
      public_id: 'user-1',
      avatar: '/uploads/profile/U1/avatar.png',
    })
    const harness = createHarness({ user: serverUser, isLoaded: true })
    await harness.transition.logout()
    expect(harness.getUser()).toBeNull()
    expect(mocks.clearPersistedSession).toHaveBeenCalled()
    harness.loadUser.mockResolvedValue({ user: serverUser })

    await expect(harness.transition.ensureSession()).resolves.toEqual(serverUser)
  })

  it('loads the server avatar after an OAuth callback without a client snapshot', async () => {
    const harness = createHarness()
    const serverUser = makeSessionUser({
      public_id: 'user-1',
      avatar: '/uploads/profile/U1/avatar.png',
    })
    harness.loadUser.mockResolvedValue({ user: serverUser })
    const replace = vi.fn().mockResolvedValue(undefined)

    await expect(harness.transition.completeOAuthCallback({ replace })).resolves.toBe(true)
    expect(harness.getUser()).toEqual(serverUser)
  })

  it('never carries an avatar across identities', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1', avatar: '/uploads/profile/U1/avatar.png' }),
      isLoaded: true,
    })
    mocks.readStoredSessionUser.mockReturnValue(
      makeSessionUser({ public_id: 'user-1', avatar: '/uploads/profile/U1/avatar.png' }),
    )
    harness.loadUser.mockResolvedValue({ user: makeSessionUser({ public_id: 'user-2' }) })

    await expect(harness.transition.fetchUserInfo(true)).resolves.toEqual(
      makeSessionUser({ public_id: 'user-2' }),
    )
  })

  it('keeps an explicitly committed avatar instead of merging', async () => {
    const harness = createHarness({
      user: makeSessionUser({ public_id: 'user-1' }),
      isLoaded: true,
    })

    harness.transition.setAuth(
      makeSessionUser({ public_id: 'user-1', avatar: '/uploads/profile/U1/avatar-new.png' }),
    )

    expect(harness.getUser()).toEqual(
      makeSessionUser({ public_id: 'user-1', avatar: '/uploads/profile/U1/avatar-new.png' }),
    )
    expect(mocks.persistSessionUser).toHaveBeenCalledWith(
      makeSessionUser({ public_id: 'user-1', avatar: '/uploads/profile/U1/avatar-new.png' }),
    )
  })
})
