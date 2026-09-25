import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../../../shared/api/sessionUser'

const mocks = vi.hoisted(() => ({
  clearAuthScopedQueryCache: vi.fn(),
  clearPersistedSession: vi.fn(),
  persistSessionUser: vi.fn(),
  readStoredSessionUser: vi.fn(),
}))

vi.mock('../../../shared/query/queryClient', () => ({
  clearAuthScopedQueryCache: mocks.clearAuthScopedQueryCache,
}))
vi.mock('./sessionPersistence', () => ({
  clearPersistedSession: mocks.clearPersistedSession,
  persistSessionUser: mocks.persistSessionUser,
  readStoredSessionUser: mocks.readStoredSessionUser,
}))

import { createBrowserSessionTransition } from './browserSessionTransition'

function makeUser(publicId: string): User {
  return { public_id: publicId } as User
}

function createHarness(
  overrides: Partial<{
    user: User | null
    loaded: boolean
    validatedAt: number
  }> = {},
) {
  let user = overrides.user ?? null
  let loaded = overrides.loaded ?? false
  let validatedAt = overrides.validatedAt ?? 0
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
      validatedAt = 1_000
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
    now: () => 1_001,
  })
  return { transition, transport, loadUser, state, getUser: () => user }
}

describe('browser session transition', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.readStoredSessionUser.mockReturnValue(null)
  })

  it('commits a refreshed identity and clears auth-scoped cache only when it changes', async () => {
    const harness = createHarness({ user: makeUser('user-1'), loaded: true, validatedAt: 0 })
    harness.loadUser.mockResolvedValue({ user: makeUser('user-2') })

    await expect(harness.transition.fetchUserInfo(true)).resolves.toEqual(makeUser('user-2'))

    expect(harness.state.commit).toHaveBeenCalledWith(makeUser('user-2'))
    expect(mocks.persistSessionUser).toHaveBeenCalledWith(makeUser('user-2'))
    expect(mocks.clearAuthScopedQueryCache).toHaveBeenCalledOnce()
  })

  it('fails closed and clears local state when a fresh user projection fails', async () => {
    const harness = createHarness({ user: makeUser('user-1'), loaded: true })
    harness.loadUser.mockRejectedValue(new Error('upstream unavailable'))

    await expect(
      harness.transition.fetchUserInfo(true, { failClosedOnError: true }),
    ).resolves.toBeNull()

    expect(harness.getUser()).toBeNull()
    expect(harness.state.clear).toHaveBeenCalledOnce()
    expect(mocks.clearPersistedSession).toHaveBeenCalledOnce()
    expect(mocks.clearAuthScopedQueryCache).toHaveBeenCalledOnce()
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
    harness.loadUser.mockResolvedValue({ user: makeUser('user-1') })
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
    const harness = createHarness({ user: makeUser('user-1'), loaded: true })
    harness.transport.logout.mockResolvedValue(false)

    await expect(harness.transition.logout()).resolves.toBe(false)

    expect(harness.state.clear).not.toHaveBeenCalled()
    expect(harness.getUser()).toEqual(makeUser('user-1'))
  })
})
