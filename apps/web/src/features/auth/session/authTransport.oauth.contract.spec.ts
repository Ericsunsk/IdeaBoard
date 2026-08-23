import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  signInSocial: vi.fn(),
}))

vi.mock('../../../shared/auth/client', () => ({
  authClient: {
    signIn: {
      email: vi.fn(),
      social: mocks.signInSocial,
    },
    signUp: { email: vi.fn() },
    signOut: vi.fn(),
    getSession: vi.fn(),
    admin: {
      stopImpersonating: vi.fn(),
    },
    useSession: vi.fn(),
  },
  extractBetterAuthErrorMessage: (_error: unknown, fallback: string) => fallback,
  normalizeCanonicalEmail: (email: string) => email.trim().toLowerCase(),
}))

import { USER_AUTH_CALLBACK_PATH } from '../../../shared/constants/routePaths'
import { authTransport } from './authTransport'

describe('authTransport OAuth providers', () => {
  beforeEach(() => {
    mocks.signInSocial.mockReset().mockResolvedValue({})
  })

  it('uses Better Auth Google social login with a same-origin callback and safe redirect', async () => {
    window.history.replaceState(
      {},
      '',
      '/user/login?redirect=%2Fuser%2Fboards%3Fview%3Drecent%23owned',
    )

    await authTransport.signInWithOAuth('google')

    expect(mocks.signInSocial).toHaveBeenCalledOnce()
    const request = mocks.signInSocial.mock.calls[0]?.[0] as {
      provider: string
      callbackURL: string
      errorCallbackURL: string
    }
    const callbackURL = new URL(request.callbackURL)
    const errorCallbackURL = new URL(request.errorCallbackURL)
    expect(request.provider).toBe('google')
    expect(callbackURL.origin).toBe(window.location.origin)
    expect(callbackURL.pathname).toBe(USER_AUTH_CALLBACK_PATH)
    expect(callbackURL.searchParams.get('redirect')).toBe('/user/boards?view=recent#owned')
    expect(errorCallbackURL).toEqual(callbackURL)
  })

  it.each([
    'https://attacker.example/user/boards',
    '//attacker.example/user/boards',
    '/user/../admin/users',
  ])('does not forward an unsafe redirect into the callback URL: %s', async (redirect) => {
    window.history.replaceState({}, '', `/user/login?redirect=${encodeURIComponent(redirect)}`)

    await authTransport.signInWithOAuth('google')

    const request = mocks.signInSocial.mock.calls[0]?.[0] as { callbackURL: string }
    const callbackURL = new URL(request.callbackURL)
    expect(callbackURL.origin).toBe(window.location.origin)
    expect(callbackURL.pathname).toBe(USER_AUTH_CALLBACK_PATH)
    expect(callbackURL.searchParams.has('redirect')).toBe(false)
  })

  it('uses the Discord provider with the same callback contract', async () => {
    window.history.replaceState({}, '', '/user/login')

    await authTransport.signInWithOAuth('discord')

    expect(mocks.signInSocial).toHaveBeenCalledWith(
      expect.objectContaining({ provider: 'discord' }),
    )
  })

  it('accepts an explicit safe return path for an in-place auth overlay', async () => {
    window.history.replaceState({}, '', '/user/search')

    await authTransport.signInWithOAuth('google', '/user/search?q=architectural%20lighting')

    const request = mocks.signInSocial.mock.calls[0]?.[0] as { callbackURL: string }
    expect(new URL(request.callbackURL).searchParams.get('redirect')).toBe(
      '/user/search?q=architectural%20lighting',
    )
  })
})
