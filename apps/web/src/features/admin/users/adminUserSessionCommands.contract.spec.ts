import { beforeEach, describe, expect, it, vi } from 'vitest'

const events: string[] = []
const clearAuth = vi.fn(() => events.push('clearAuth'))
const clearAuthScopedQueryCache = vi.fn(() => events.push('clearQueryCache'))
const impersonateUser = vi.fn()

vi.mock('../../auth/session/authSession', () => ({
  useUserStore: () => ({
    clearAuth,
  }),
  startImpersonation: async (authUserId: string) => {
    const result = await impersonateUser({ userId: authUserId })
    if (result?.error) throw result.error
  },
}))

vi.mock('../../../shared/query/queryClient', () => ({
  clearAuthScopedQueryCache,
}))

describe('admin user session commands', () => {
  beforeEach(() => {
    events.length = 0
    vi.clearAllMocks()
  })

  it('impersonates through Better Auth, clears local session state, then schedules redirect', async () => {
    impersonateUser.mockResolvedValueOnce({ error: null })
    const { createAdminUserSessionCommands } = await import('./adminUserSessionCommands')
    const redirect = {
      schedule: vi.fn(() => events.push('redirect')),
    }

    await createAdminUserSessionCommands(redirect).startImpersonation('auth-1')

    expect(impersonateUser).toHaveBeenCalledWith({ userId: 'auth-1' })
    expect(clearAuth).toHaveBeenCalledTimes(1)
    expect(clearAuthScopedQueryCache).toHaveBeenCalledTimes(1)
    expect(redirect.schedule).toHaveBeenCalledWith('/', 1000)
    expect(events).toEqual(['clearAuth', 'clearQueryCache', 'redirect'])
  })

  it('surfaces Better Auth errors without clearing session state', async () => {
    impersonateUser.mockResolvedValueOnce({ error: new Error('not allowed') })
    const { createAdminUserSessionCommands } = await import('./adminUserSessionCommands')

    await expect(
      createAdminUserSessionCommands({ schedule: vi.fn() }).startImpersonation('auth-2'),
    ).rejects.toThrow('not allowed')

    expect(clearAuth).not.toHaveBeenCalled()
    expect(clearAuthScopedQueryCache).not.toHaveBeenCalled()
  })
})
