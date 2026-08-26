import { describe, expect, it, vi } from 'vitest'

vi.mock('better-auth/vue', () => ({
  createAuthClient: () => ({}),
}))

vi.mock('better-auth/client/plugins', () => ({
  adminClient: () => ({}),
  emailOTPClient: () => ({}),
}))

import { resolveBetterAuthErrorMessage } from './client'

describe('resolveBetterAuthErrorMessage', () => {
  it('resolves Better Auth stable error codes to User copy', () => {
    expect(resolveBetterAuthErrorMessage({ code: 'INVALID_EMAIL_OR_PASSWORD' }, 'Fallback')).toBe(
      'The email or password is incorrect.',
    )
    expect(resolveBetterAuthErrorMessage({ data: { code: 'TOO_MANY_REQUESTS' } }, 'Fallback')).toBe(
      'Too many attempts. Please wait and try again.',
    )
  })

  it('never exposes raw Better Auth prose', () => {
    expect(resolveBetterAuthErrorMessage({ message: 'provider detail' }, 'Fallback')).toBe(
      'Fallback',
    )
    expect(resolveBetterAuthErrorMessage(null, 'Fallback')).toBe('Fallback')
  })
})
