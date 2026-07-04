import { beforeEach, describe, expect, it, vi } from 'vitest'

const router = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => router,
}))

import { useLogoutRedirect } from './useLogoutRedirect'

describe('useLogoutRedirect', () => {
  beforeEach(() => {
    router.push.mockReset()
    router.replace.mockReset()
  })

  it('logs out through the injected dependency before pushing the redirect', async () => {
    const logout = vi.fn(async () => true)
    const { logoutAndRedirect } = useLogoutRedirect({ logout })

    await expect(logoutAndRedirect('/user/home')).resolves.toBe(true)

    expect(logout).toHaveBeenCalledOnce()
    expect(router.push).toHaveBeenCalledWith('/user/home')
    expect(router.replace).not.toHaveBeenCalled()
  })

  it('uses replace navigation when requested', async () => {
    const logout = vi.fn(async () => true)
    const { logoutAndRedirect } = useLogoutRedirect({ logout })

    await expect(logoutAndRedirect('/admin/login', { replace: true })).resolves.toBe(true)

    expect(router.replace).toHaveBeenCalledWith('/admin/login')
    expect(router.push).not.toHaveBeenCalled()
  })

  it('does not navigate when logout fails', async () => {
    const logout = vi.fn(async () => false)
    const { logoutAndRedirect } = useLogoutRedirect({ logout })

    await expect(logoutAndRedirect('/user/home')).resolves.toBe(false)

    expect(router.push).not.toHaveBeenCalled()
    expect(router.replace).not.toHaveBeenCalled()
  })
})
