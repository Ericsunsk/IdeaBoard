import { describe, expect, it, vi } from 'vitest'

vi.mock('better-auth/vue', () => ({
  createAuthClient: () => ({}),
}))

vi.mock('better-auth/client/plugins', () => ({
  adminClient: () => ({}),
  emailOTPClient: () => ({}),
  usernameClient: () => ({}),
}))

import { extractBetterAuthErrorMessage } from './client'

describe('extractBetterAuthErrorMessage', () => {
  it('extracts Better Auth messages in priority order', () => {
    expect(extractBetterAuthErrorMessage({ message: ' root message ' }, '默认错误')).toBe(
      'root message',
    )
    expect(extractBetterAuthErrorMessage({ statusText: ' status text ' }, '默认错误')).toBe(
      'status text',
    )
    expect(
      extractBetterAuthErrorMessage({ error: { message: ' nested message ' } }, '默认错误'),
    ).toBe('nested message')
    expect(extractBetterAuthErrorMessage({ data: { message: ' data message ' } }, '默认错误')).toBe(
      'data message',
    )
    expect(extractBetterAuthErrorMessage({ data: { msg: ' data msg ' } }, '默认错误')).toBe(
      'data msg',
    )
  })

  it('falls back for empty or unsupported payloads', () => {
    expect(extractBetterAuthErrorMessage(null, '默认错误')).toBe('默认错误')
    expect(extractBetterAuthErrorMessage({ message: '   ' }, '默认错误')).toBe('默认错误')
    expect(extractBetterAuthErrorMessage({ data: { msg: 403 } }, '默认错误')).toBe('默认错误')
  })
})
