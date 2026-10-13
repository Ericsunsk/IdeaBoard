import { beforeEach, describe, expect, it, vi } from 'vitest'

const events: string[] = []
const clearAuth = vi.fn(() => events.push('clearAuth'))
const impersonateAdminUser = vi.fn()

vi.mock('../../../auth/session/userSession', () => ({
  useUserStore: () => ({
    clearAuth,
  }),
}))

vi.mock('./api', () => ({
  impersonateAdminUser,
}))

describe('admin user session commands', () => {
  beforeEach(() => {
    events.length = 0
    vi.clearAllMocks()
  })

  it('impersonates through the Go Admin bridge, clears local session state, then schedules redirect', async () => {
    impersonateAdminUser.mockResolvedValueOnce(undefined)
    const { createAdminUserSessionCommands } = await import('./sessionCommands')
    const redirect = {
      schedule: vi.fn(() => events.push('redirect')),
    }

    await createAdminUserSessionCommands(redirect).startImpersonation('public-1')

    expect(impersonateAdminUser).toHaveBeenCalledWith('public-1')
    expect(clearAuth).toHaveBeenCalledTimes(1)
    expect(redirect.schedule).toHaveBeenCalledWith('/', 1000)
    expect(events).toEqual(['clearAuth', 'redirect'])
  })

  it('surfaces bridge errors without clearing session state', async () => {
    impersonateAdminUser.mockRejectedValueOnce(new Error('not allowed'))
    const { createAdminUserSessionCommands } = await import('./sessionCommands')

    await expect(
      createAdminUserSessionCommands({ schedule: vi.fn() }).startImpersonation('public-2'),
    ).rejects.toThrow('not allowed')

    expect(clearAuth).not.toHaveBeenCalled()
  })
})
